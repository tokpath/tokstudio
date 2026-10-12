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

type settingsRow struct {
	ID                string
	Enabled           bool
	PayloadCiphertext string
	UpdatedAt         time.Time
}

func (settingsRow) TableName() string { return "mail_settings" }

// Service resolves platform settings on each delivery so API and worker see
// changes without a restart. Recovery emails and business emails share transport,
// while their private queues remain separate.
type Service struct {
	db       *gorm.DB
	key      string
	fallback SMTPOptions
	override Sender // test injection only
}

func NewService(db *gorm.DB, key string, fallback SMTPOptions) *Service {
	return &Service{db: db, key: key, fallback: fallback}
}
func (s *Service) SetSender(sender Sender) { s.override = sender }
func Migrations() (string, fs.FS)          { sub, _ := fs.Sub(migrationFS, "migrate"); return "mail", sub }

type SettingsView struct {
	Enabled     bool       `json:"enabled"`
	Available   bool       `json:"available"`
	Source      string     `json:"source"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    string     `json:"username"`
	From        string     `json:"from"`
	Security    string     `json:"security"`
	PasswordSet bool       `json:"password_set"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}
type SettingsInput struct {
	Enabled  bool    `json:"enabled"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username string  `json:"username"`
	Password *string `json:"password,omitempty"`
	From     string  `json:"from"`
	Security string  `json:"security"`
}

func (s *Service) options(db *gorm.DB) (SMTPOptions, bool, string, *time.Time, error) {
	var row settingsRow
	err := db.Where("id = ?", "platform").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.fallback, s.fallback.Host != "", "environment", nil, nil
	}
	if err != nil {
		return SMTPOptions{}, false, "database", nil, err
	}
	if s.key == "" {
		return SMTPOptions{}, false, "database", nil, ErrUnavailable
	}
	plain, err := crypto.Open(s.key, row.PayloadCiphertext)
	var options SMTPOptions
	if err != nil || json.Unmarshal([]byte(plain), &options) != nil {
		return options, false, "database", nil, ErrUnavailable
	}
	options.Production = s.fallback.Production
	return options, row.Enabled, "database", &row.UpdatedAt, nil
}
func (s *Service) Settings(ctx context.Context) (*SettingsView, error) {
	opt, enabled, source, updated, err := s.options(s.db.WithContext(ctx))
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
	return &SettingsView{enabled, enabled && valid == nil && s.key != "", source, opt.Host, port, opt.Username, opt.From, security, opt.Password != "", updated}, nil
}
func (s *Service) Update(ctx context.Context, in SettingsInput) (*SettingsView, error) {
	if strings.TrimSpace(s.key) == "" {
		return nil, ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "platform.mail.settings").Error; err != nil {
			return err
		}
		old, _, _, _, err := s.options(tx)
		if err != nil {
			return err
		}
		password := old.Password
		if in.Password != nil {
			password = *in.Password
		}
		opt := SMTPOptions{Host: strings.TrimSpace(in.Host), Port: in.Port, Username: strings.TrimSpace(in.Username), Password: password, From: strings.TrimSpace(in.From), Security: in.Security, Production: s.fallback.Production}
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
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&settingsRow{"platform", in.Enabled, cipher, time.Now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.Settings(ctx)
}
func (s *Service) Available() bool {
	if s.override != nil {
		return Available(s.override)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	opt, enabled, _, _, err := s.options(s.db.WithContext(ctx))
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
	opt, enabled, _, _, err := s.options(s.db.WithContext(ctx))
	if err != nil || !enabled || s.key == "" {
		return ErrUnavailable
	}
	sender, err := NewSMTP(opt)
	if err != nil {
		return ErrUnavailable
	}
	return sender.Send(ctx, message)
}
