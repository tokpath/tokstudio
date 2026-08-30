package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

var (
	ErrTOTPRequired = errors.New("totp required")
	ErrTOTPInvalid  = errors.New("totp invalid")
)

type totpRow struct {
	UserID           string     `gorm:"column:user_id;primaryKey"`
	SecretCiphertext string     `gorm:"column:secret_ciphertext"`
	Status           string     `gorm:"column:status"`
	EnabledAt        *time.Time `gorm:"column:enabled_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (totpRow) TableName() string { return "identity_admin_totp" }

type TOTPSetup struct {
	Secret    string `json:"secret"`
	OTPAuth   string `json:"otpauth_url"`
	Status    string `json:"status"`
	Enabled   bool   `json:"enabled"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
}

func (s *Service) TOTPStatus(ctx context.Context, userID string) (*TOTPSetup, error) {
	var row totpRow
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return &TOTPSetup{Status: "disabled", Algorithm: "SHA1", Digits: 6, Period: 30}, nil
	}
	if err != nil {
		return nil, err
	}
	return &TOTPSetup{Status: row.Status, Enabled: row.Status == "enabled", Algorithm: "SHA1", Digits: 6, Period: 30}, nil
}

func (s *Service) TOTPEnabled(ctx context.Context, userID string) bool {
	var row totpRow
	if err := s.db.WithContext(ctx).Where("user_id = ? AND status = ?", userID, "enabled").First(&row).Error; err != nil {
		return false
	}
	return true
}

func (s *Service) SetupTOTP(ctx context.Context, user Principal, encKey, issuer string) (*TOTPSetup, error) {
	if !user.HasRole("platform_admin", "finance_admin", "ops_admin", "tech_admin") {
		return nil, ErrChannelImmutable
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	sealed, err := crypto.Seal(encKey, secret)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := totpRow{UserID: user.UserID, SecretCiphertext: sealed, Status: "pending", CreatedAt: now}
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.UserID).Assign(map[string]any{
		"secret_ciphertext": sealed, "status": "pending", "enabled_at": nil,
	}).FirstOrCreate(&row).Error; err != nil {
		return nil, err
	}
	if issuer == "" {
		issuer = "TokenHub"
	}
	otpauth := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, user.Email, secret, issuer)
	return &TOTPSetup{Secret: secret, OTPAuth: otpauth, Status: "pending", Algorithm: "SHA1", Digits: 6, Period: 30}, nil
}

func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&totpRow{}).Error
}

func (s *Service) EnableTOTP(ctx context.Context, userID, code, encKey string) error {
	if !s.verifyStoredTOTP(ctx, userID, code, encKey) {
		return ErrTOTPInvalid
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&totpRow{}).Where("user_id = ?", userID).Updates(map[string]any{
		"status": "enabled", "enabled_at": now,
	}).Error
}

func (s *Service) VerifyTOTP(ctx context.Context, userID, code, encKey string) error {
	if !s.TOTPEnabled(ctx, userID) {
		return nil
	}
	if !s.verifyStoredTOTP(ctx, userID, code, encKey) {
		return ErrTOTPInvalid
	}
	return nil
}

func (s *Service) verifyStoredTOTP(ctx context.Context, userID, code, encKey string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	var row totpRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		return false
	}
	secret, err := crypto.Open(encKey, row.SecretCiphertext)
	if err != nil {
		return false
	}
	now := time.Now().Unix()
	for _, skew := range []int64{-1, 0, 1} {
		if totpAt(secret, now+skew*30) == code {
			return true
		}
	}
	return false
}

func totpAt(secret string, unix int64) string {
	padded := secret
	if rem := len(padded) % 8; rem != 0 {
		padded += strings.Repeat("=", 8-rem)
	}
	key, err := base32.StdEncoding.DecodeString(strings.ToUpper(padded))
	if err != nil {
		return ""
	}
	counter := uint64(unix / 30)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := int(sum[offset]&0x7f)<<24 | int(sum[offset+1])<<16 | int(sum[offset+2])<<8 | int(sum[offset+3])
	return fmt.Sprintf("%06d", bin%1000000)
}

// GenerateTOTP 给测试和演练用，不要在日志里打印。
func GenerateTOTP(secret string) string {
	return totpAt(secret, time.Now().Unix())
}
