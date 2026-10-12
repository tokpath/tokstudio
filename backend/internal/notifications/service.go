// Package notifications owns inbox records and business email delivery tasks.
// Authentication tokens always stay in identity's separate private mail queue.
package notifications

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

var ErrNotFound = errors.New("notification not found")
var ErrIneligible = errors.New("notification cannot be emailed or retried")

type Recipients interface {
	NotificationUser(context.Context, string) (*identity.NotificationRecipient, error)
	NotificationStaff(context.Context, string, string, []string) ([]identity.NotificationRecipient, error)
}
type Funds interface {
	NotificationFunds(context.Context, string) (int64, error)
	NotificationQuota(context.Context, string) (int64, error)
	NotificationQuotaBlocked(context.Context, string, string, int64) (bool, error)
}
type Subscriptions interface {
	GetSubscription(context.Context, string, string) (*plans.SubscriptionView, error)
}

type Alerts interface {
	NotificationAlertOpen(context.Context, string) (bool, error)
}

type Service struct {
	db                  *gorm.DB
	events              *outbox.Service
	recipients          Recipients
	funds               Funds
	subscriptions       Subscriptions
	sender              mail.Sender
	key, officialOrigin string
	alerts              Alerts
}

func New(db *gorm.DB, events *outbox.Service, recipients Recipients, funds Funds, subs Subscriptions, sender mail.Sender, key, origin string) *Service {
	return &Service{db: db, events: events, recipients: recipients, funds: funds, subscriptions: subs, sender: sender, key: key, officialOrigin: strings.TrimRight(origin, "/")}
}

func (s *Service) SetAlerts(source Alerts) { s.alerts = source }
func Migrations() (string, fs.FS) {
	sub, _ := fs.Sub(migrationFS, "migrate")
	return "notifications", sub
}

type inboxRow struct {
	Portal                                                     string
	ID, UserID, BrandID, EventID, DedupeKey, Kind, ReferenceID string
	SubjectUserID, CustomerChannelID                           string
	PeriodEnd                                                  *time.Time
	RequiredMinor                                              int64
	Title, Body, Href                                          string
	CreatedAt                                                  time.Time
	ReadAt                                                     *time.Time
}

func (inboxRow) TableName() string { return "notifications_inbox" }

type deliveryRow struct {
	ID, NotificationID, Status, PayloadCiphertext string
	Attempts                                      int
	AvailableAt, ExpiresAt, CreatedAt             time.Time
	LeaseID                                       *string
	LastError                                     string
	AcceptedAt                                    *time.Time
}

func (deliveryRow) TableName() string { return "notifications_email_deliveries" }

type preferenceRow struct {
	UserID       string `gorm:"primaryKey"`
	BrandID      string
	EmailEnabled bool
	UpdatedAt    time.Time
}

func (preferenceRow) TableName() string { return "notifications_preferences" }

type eventData struct {
	UserID            string     `json:"user_id"`
	ChannelOrgID      string     `json:"channel_org_id"`
	CustomerChannelID string     `json:"customer_channel_id"`
	SubscriptionID    string     `json:"subscription_id"`
	PeriodEnd         *time.Time `json:"period_end"`
	RequiredMinor     int64      `json:"required_minor"`
	Message           string     `json:"message"`
	Severity          string     `json:"severity"`
}

var eventTypes = []string{"billing.balance.insufficient", "billing.charge.committed", "billing.quota.insufficient", "billing.quota.low", "billing.quota.recovered",
	"plans.subscription.expiring_3d", "plans.subscription.expiring_1d", "plans.subscription.expired", "plans.subscription.renewal_failed", "plans.subscription.renewed", "ops.alert.opened", "ops.alert.resolved"}

func (s *Service) Consume(ctx context.Context) error {
	return s.events.Consume(ctx, "notifications-v1", eventTypes, func(tx *gorm.DB, event outbox.Event) error {
		var data eventData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return err
		}
		kind := strings.TrimPrefix(event.Type, "plans.subscription.")
		reference := event.AggregateID
		key := event.Type + ":" + reference
		var recipients []identity.NotificationRecipient
		switch {
		case strings.HasPrefix(event.Type, "ops.alert."):
			var err error
			recipients, err = s.recipients.NotificationStaff(ctx, "platform", "*", []string{"platform_admin", "ops_admin", "tech_admin"})
			if err != nil {
				return err
			}
			kind = "alert_" + strings.TrimPrefix(event.Type, "ops.alert.")
		case strings.HasPrefix(event.Type, "billing.quota."):
			var err error
			recipients, err = s.recipients.NotificationStaff(ctx, "channel", data.ChannelOrgID, []string{"channel_admin", "oem_ops", "oem_finance"})
			if err != nil {
				return err
			}
			kind = "quota_" + strings.TrimPrefix(event.Type, "billing.quota.")
			reference = data.ChannelOrgID
			key = event.Type + ":" + reference + ":" + event.ID
			if kind == "quota_insufficient" {
				key = event.Type + ":" + reference + ":" + event.CreatedAt.UTC().Truncate(6*time.Hour).Format(time.RFC3339)
			}
		default:
			recipient, err := s.recipients.NotificationUser(ctx, data.UserID)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			recipients = []identity.NotificationRecipient{*recipient}
			if data.SubscriptionID != "" {
				reference = data.SubscriptionID
				if data.PeriodEnd != nil {
					key = event.Type + ":" + reference + ":" + data.PeriodEnd.UTC().Format(time.RFC3339Nano)
				}
			}
			if event.Type == "billing.balance.insufficient" {
				kind = "balance_insufficient"
				key = kind + ":" + event.CreatedAt.UTC().Truncate(6*time.Hour).Format(time.RFC3339)
			}
			if event.Type == "billing.charge.committed" {
				balance, err := s.funds.NotificationFunds(ctx, data.UserID)
				if err != nil {
					return err
				}
				if balance >= LowBalanceMinor {
					return nil
				}
				kind = "balance_low"
				data.RequiredMinor = LowBalanceMinor
				key = kind + ":" + event.CreatedAt.UTC().Format("2006-01-02")
			}
		}
		for _, recipient := range recipients {
			title, body, href := noticeText(kind, recipient.Locale, data.Message)
			if href == "" {
				continue
			}
			row := inboxRow{ID: id.New("ntf"), UserID: recipient.UserID, BrandID: recipient.BrandID, EventID: event.ID, DedupeKey: key, Kind: kind,
				SubjectUserID: data.UserID, CustomerChannelID: data.CustomerChannelID,
				ReferenceID: reference, PeriodEnd: data.PeriodEnd, RequiredMinor: data.RequiredMinor, Title: title, Body: body, Href: href, CreatedAt: event.CreatedAt}
			if strings.HasPrefix(kind, "expiring_") {
				obsolete, err := s.obsolete(ctx, row)
				if err != nil {
					return err
				}
				if obsolete {
					continue
				}
			}
			if err := s.queueTx(tx, row, recipient); err != nil {
				return err
			}
		}
		return nil
	})
}

// Low-balance alerts include valid USD plan/gift credit and cash, not cash alone.
const LowBalanceMinor int64 = 1_000_000

func (s *Service) origin(recipient identity.NotificationRecipient) string {
	if recipient.BrandID == identity.OfficialBrandID {
		return s.officialOrigin
	}
	host := recipient.PrimaryDomain
	if recipient.Portal == "channel" && recipient.AdminDomain != "" {
		host = recipient.AdminDomain
	}
	// Domains are managed by identity, never copied from a request's Host header.
	parsed, err := url.Parse("https://" + host)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}
func (s *Service) message(row inboxRow, recipient identity.NotificationRecipient, deliveryID string) mail.Message {
	if row.Portal != "" {
		recipient.Portal = row.Portal
	}
	return mail.Message{ID: deliveryID, To: recipient.Email, Brand: recipient.BrandName, Subject: row.Title,
		Text: fmt.Sprintf("%s\n\n%s\n\n%s%s\n\n%s", recipient.BrandName, row.Body, s.origin(recipient), row.Href, recipient.BrandName)}
}
func (s *Service) queueTx(tx *gorm.DB, row inboxRow, recipient identity.NotificationRecipient) error {
	row.Portal = recipient.Portal
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "brand_id"}, {Name: "user_id"}, {Name: "dedupe_key"}}, DoNothing: true}).Create(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}
	delivery := deliveryRow{ID: id.New("ntd"), NotificationID: row.ID, Status: "pending", AvailableAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), ExpiresAt: row.CreatedAt.Add(72 * time.Hour)}
	if strings.HasPrefix(row.Kind, "expiring_") && row.PeriodEnd != nil && row.PeriodEnd.Before(delivery.ExpiresAt) {
		delivery.ExpiresAt = *row.PeriodEnd
	}
	enabled, err := s.emailEnabled(tx, row.UserID, row.BrandID)
	if err != nil {
		return err
	}
	if row.Kind == "smtp_test" {
		enabled = true
	}
	if !enabled {
		delivery.Status, delivery.LastError = "cancelled", "email_disabled"
	} else if !recipient.Verified {
		delivery.Status, delivery.LastError = "blocked_unverified", "email_unverified"
	} else if !mail.Available(s.sender) || s.key == "" {
		delivery.Status, delivery.LastError = "blocked_smtp", "smtp_unavailable"
	}
	if s.key != "" && delivery.Status != "cancelled" {
		body, err := json.Marshal(s.message(row, recipient, delivery.ID))
		if err != nil {
			return err
		}
		delivery.PayloadCiphertext, err = crypto.Seal(s.key, string(body))
		if err != nil {
			return err
		}
	}
	return tx.Create(&delivery).Error
}
func (s *Service) emailEnabled(tx *gorm.DB, userID, brandID string) (bool, error) {
	var row preferenceRow
	err := tx.Where("user_id = ? AND brand_id = ?", userID, brandID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	return row.EmailEnabled, err
}
func (s *Service) EmailEnabled(ctx context.Context, userID, brandID string) (bool, error) {
	return s.emailEnabled(s.db.WithContext(ctx), userID, brandID)
}
func (s *Service) SetEmailEnabled(ctx context.Context, userID, brandID string, enabled bool) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&preferenceRow{userID, brandID, enabled, time.Now().UTC()}).Error
}

type Item struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Href        string     `json:"href"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at"`
	EmailStatus string     `json:"email_status"`
}
type Page struct {
	Items      []Item `json:"items"`
	Unread     int64  `json:"unread"`
	NextCursor string `json:"next_cursor"`
}

func (s *Service) List(ctx context.Context, userID, brandID, cursor string, unreadOnly bool) (*Page, error) {
	q := s.db.WithContext(ctx).Table("notifications_inbox n").Select("n.id,n.kind,n.title,n.body,n.href,n.created_at,n.read_at,d.status AS email_status").
		Joins("LEFT JOIN notifications_email_deliveries d ON d.notification_id=n.id").Where("n.user_id = ? AND n.brand_id = ?", userID, brandID)
	if unreadOnly {
		q = q.Where("n.read_at IS NULL")
	}
	if cursor != "" {
		var row inboxRow
		if err := s.db.WithContext(ctx).Where("id=? AND user_id=? AND brand_id=?", cursor, userID, brandID).First(&row).Error; err != nil {
			return nil, ErrNotFound
		}
		q = q.Where("(n.created_at,n.id) < (?,?)", row.CreatedAt, row.ID)
	}
	page := &Page{Items: []Item{}}
	if err := q.Order("n.created_at DESC,n.id DESC").Limit(31).Scan(&page.Items).Error; err != nil {
		return nil, err
	}
	if len(page.Items) > 30 {
		page.Items = page.Items[:30]
		page.NextCursor = page.Items[29].ID
	}
	if err := s.db.WithContext(ctx).Model(&inboxRow{}).Where("user_id=? AND brand_id=? AND read_at IS NULL", userID, brandID).Count(&page.Unread).Error; err != nil {
		return nil, err
	}
	return page, nil
}
func (s *Service) Read(ctx context.Context, userID, brandID, notificationID string) error {
	q := s.db.WithContext(ctx).Model(&inboxRow{}).Where("user_id=? AND brand_id=?", userID, brandID)
	if notificationID != "" {
		q = q.Where("id=?", notificationID)
	}
	res := q.Update("read_at", gorm.Expr("COALESCE(read_at, ?)", time.Now().UTC()))
	if res.Error != nil {
		return res.Error
	}
	if notificationID != "" && res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Unread(ctx context.Context, userID, brandID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&inboxRow{}).Where("user_id=? AND brand_id=? AND read_at IS NULL", userID, brandID).Count(&count).Error
	return count, err
}

func (s *Service) QueueTest(ctx context.Context, userID string) (string, error) {
	recipient, err := s.recipients.NotificationUser(ctx, userID)
	if err != nil {
		return "", err
	}
	if !recipient.Verified || !mail.Available(s.sender) {
		return "", ErrIneligible
	}
	title, body, href := noticeText("smtp_test", recipient.Locale, "")
	row := inboxRow{ID: id.New("ntf"), UserID: userID, BrandID: recipient.BrandID, EventID: id.New("test"), DedupeKey: id.New("test"), Kind: "smtp_test", Title: title, Body: body, Href: href, CreatedAt: time.Now().UTC()}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.queueTx(tx, row, *recipient) })
	return row.ID, err
}
