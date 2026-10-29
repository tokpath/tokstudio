package mail

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

var ErrUnavailable = errors.New("SMTP is unavailable")
var ErrSettings = errors.New("invalid mail settings")

func Available(sender Sender) bool {
	if sender == nil {
		return false
	}
	if dynamic, ok := sender.(interface{ Available() bool }); ok {
		return dynamic.Available()
	}
	return true
}

func AvailableForBrand(sender Sender, brandID string) bool {
	if dynamic, ok := sender.(interface{ AvailableForBrand(string) bool }); ok {
		return dynamic.AvailableForBrand(brandID)
	}
	return Available(sender)
}

type settingsRow struct {
	ID                string
	Enabled           bool
	PayloadCiphertext string
	UpdatedAt         time.Time
}

func (settingsRow) TableName() string { return "mail_settings" }

// Service resolves brand settings on each delivery so API and worker see
// changes without a restart. Recovery emails and business emails share transport,
// while their private queues remain separate.
type Service struct {
	db              *gorm.DB
	key             string
	fallback        SMTPOptions
	platformBrandID string
	override        Sender // test injection only
}

func NewService(db *gorm.DB, key string, fallback SMTPOptions, platformBrandID string) *Service {
	return &Service{db: db, key: key, fallback: fallback, platformBrandID: platformBrandID}
}
func (s *Service) SetSender(sender Sender) { s.override = sender }
func Migrations() (string, fs.FS)          { sub, _ := fs.Sub(migrationFS, "migrate"); return "mail", sub }

type SettingsView struct {
	Mode        string     `json:"mode"`
	Inherited   bool       `json:"inherited"`
	Enabled     bool       `json:"enabled"`
	Available   bool       `json:"available"`
	Source      string     `json:"source"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    string     `json:"username"`
	From        string     `json:"from"`
	FromName    string     `json:"from_name"`
	Security    string     `json:"security"`
	PasswordSet bool       `json:"password_set"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}
type SettingsInput struct {
	Mode     string  `json:"mode"`
	Enabled  bool    `json:"enabled"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username string  `json:"username"`
	Password *string `json:"password,omitempty"`
	From     string  `json:"from"`
	FromName string  `json:"from_name"`
	Security string  `json:"security"`
}

func (s *Service) settingsID(brandID string) string {
	if brandID == "" || brandID == s.platformBrandID {
		return "platform"
	}
	return "brand:" + brandID
}

func (s *Service) storedOptions(db *gorm.DB, id string) (SMTPOptions, bool, *time.Time, error) {
	var row settingsRow
	err := db.Where("id = ?", id).First(&row).Error
	if err != nil {
		return SMTPOptions{}, false, nil, err
	}
	if s.key == "" {
		return SMTPOptions{}, false, nil, ErrUnavailable
	}
	plain, err := crypto.Open(s.key, row.PayloadCiphertext)
	var options SMTPOptions
	if err != nil || json.Unmarshal([]byte(plain), &options) != nil {
		return options, false, nil, ErrUnavailable
	}
	options.Production = s.fallback.Production
	return options, row.Enabled, &row.UpdatedAt, nil
}

func (s *Service) options(db *gorm.DB, brandID string) (SMTPOptions, bool, string, *time.Time, error) {
	if id := s.settingsID(brandID); id != "platform" {
		opt, enabled, updated, err := s.storedOptions(db, id)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// A disabled or broken brand transport must not silently send through
			// another account. Only explicit inheritance uses the platform default.
			return opt, enabled, "brand", updated, err
		}
		opt, enabled, _, updated, err = s.options(db, "")
		opt.FromName = "" // inherited mail identifies the recipient's own brand
		return opt, enabled, "platform_default", updated, err
	}
	opt, enabled, updated, err := s.storedOptions(db, "platform")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.fallback, s.fallback.Host != "", "environment", nil, nil
	}
	return opt, enabled, "database", updated, err
}

func (s *Service) Settings(ctx context.Context) (*SettingsView, error) {
	return s.SettingsForBrand(ctx, "")
}
func (s *Service) SettingsForBrand(ctx context.Context, brandID string) (*SettingsView, error) {
	opt, enabled, source, updated, err := s.options(s.db.WithContext(ctx), brandID)
	if err != nil {
		return nil, err
	}
	_, valid := NewSMTP(opt)
	port, security := opt.Port, opt.Security
	if port == 0 {
		port = 587
	}
	if security == "" {
		security = "starttls"
	}
	view := &SettingsView{Mode: "custom", Inherited: source == "platform_default", Enabled: enabled, Available: enabled && valid == nil && s.key != "", Source: source, Host: opt.Host, Port: port, Username: opt.Username, From: opt.From, FromName: opt.FromName, Security: security, PasswordSet: opt.Password != "", UpdatedAt: updated}
	if view.Inherited {
		view.Mode = "platform"
		// Shared transport credentials and server settings belong to the platform.
		view.Host, view.Username, view.Security = "", "", ""
		view.Port, view.PasswordSet, view.UpdatedAt = 0, false, nil
	}
	return view, nil
}
func (s *Service) Update(ctx context.Context, in SettingsInput) (*SettingsView, error) {
	return s.UpdateForBrand(ctx, in, "")
}
func (s *Service) UpdateForBrand(ctx context.Context, in SettingsInput, brandID string) (*SettingsView, error) {
	if strings.TrimSpace(s.key) == "" {
		return nil, ErrUnavailable
	}
	id := s.settingsID(brandID)
	if in.Mode != "" && in.Mode != "custom" && (in.Mode != "platform" || id == "platform") {
		return nil, ErrSettings
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "mail.settings:"+id).Error; err != nil {
			return err
		}
		if in.Mode == "platform" {
			return tx.Delete(&settingsRow{}, "id=?", id).Error
		}
		old, _, _, err := s.storedOptions(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			old = SMTPOptions{}
			if id == "platform" {
				old = s.fallback
			}
		} else if err != nil {
			return err
		}
		password := old.Password
		if in.Password != nil {
			password = *in.Password
		}
		opt := SMTPOptions{Host: strings.TrimSpace(in.Host), Port: in.Port, Username: strings.TrimSpace(in.Username), Password: password, From: strings.TrimSpace(in.From), FromName: strings.TrimSpace(in.FromName), Security: in.Security, Production: s.fallback.Production}
		if opt.Security == "" {
			opt.Security = "starttls"
		}
		if opt.Port == 0 {
			opt.Port = 587
		}
		if in.Enabled {
			if _, err := NewSMTP(opt); err != nil {
				return ErrSettings
			}
		}
		body, err := json.Marshal(opt)
		if err != nil {
			return err
		}
		cipher, err := crypto.Seal(s.key, string(body))
		if err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&settingsRow{id, in.Enabled, cipher, time.Now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.SettingsForBrand(ctx, brandID)
}
func (s *Service) Available() bool {
	return s.AvailableForBrand("")
}
func (s *Service) AvailableForBrand(brandID string) bool {
	if s.override != nil {
		return AvailableForBrand(s.override, brandID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	opt, enabled, _, _, err := s.options(s.db.WithContext(ctx), brandID)
	if err != nil || !enabled || s.key == "" {
		return false
	}
	_, err = NewSMTP(opt)
	return err == nil
}
func (s *Service) Send(ctx context.Context, message Message) error {
	if s.override != nil {
		return s.override.Send(ctx, message)
	}
	opt, enabled, _, _, err := s.options(s.db.WithContext(ctx), message.BrandID)
	if err != nil || !enabled || s.key == "" {
		return ErrUnavailable
	}
	sender, err := NewSMTP(opt)
	if err != nil {
		return ErrUnavailable
	}
	return sender.Send(ctx, message)
}
