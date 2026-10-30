package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strings"
	"time"
)

type Notification struct {
	ID          string     `json:"id"`
	OperatorID  string     `json:"operator_id"`
	RecipientID string     `json:"recipient_id"`
	BusinessKey string     `json:"business_key"`
	Category    string     `json:"category"`
	Severity    string     `json:"severity"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Reference   string     `json:"reference"`
	Fingerprint string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}

func (Notification) TableName() string { return "operations_notifications" }

// SystemGenerated identifies notices emitted by trusted domain hooks. Operator
// announcements cannot use those namespaces, regardless of their content.
func (n Notification) SystemGenerated() bool {
	prefix := systemNotificationNamespace(n.BusinessKey)
	return prefix != "" && len(n.BusinessKey) > len(prefix)
}

func systemNotificationNamespace(key string) string {
	for _, prefix := range []string{"commerce:", "incentive:", "catalog:", "renewal-notice:", "replenishment:", "risk:", restrictionReviewKey, "remediation:", "price-check:"} {
		if strings.HasPrefix(key, prefix) {
			return prefix
		}
	}
	return ""
}

type Preference struct {
	OperatorID  string    `gorm:"primaryKey" json:"operator_id"`
	RecipientID string    `gorm:"primaryKey" json:"recipient_id"`
	Marketing   bool      `json:"marketing"`
	UsageEmail  bool      `json:"usage_email"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Preference) TableName() string { return "operations_preferences" }

// Emit is a trusted domain hook. User-facing announcements must use Announce.
// Sensitive request content and credentials must never be passed into a notice.
func (s *Service) Emit(ctx context.Context, tx *gorm.DB, in Notification) (Notification, error) {
	var out Notification
	if !validID(in.OperatorID) || !validID(in.RecipientID) || in.BusinessKey == "" || len(in.BusinessKey) > 200 || strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || len(in.Body) > 16000 || (in.Category != "mandatory" && in.Category != "usage" && in.Category != "marketing") || (in.Severity != "info" && in.Severity != "warning" && in.Severity != "critical") {
		return out, ErrInvalid
	}
	fp := hash([]string{in.Category, in.Severity, in.Title, in.Body, in.Reference})
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lock(t, in.OperatorID+in.RecipientID+in.BusinessKey); e != nil {
			return e
		}
		q := t.Where("operator_id=? AND recipient_id=? AND business_key=?", in.OperatorID, in.RecipientID, in.BusinessKey).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		recipient, e := s.self(ctx, t, in.RecipientID, in.OperatorID)
		if e != nil {
			return e
		}
		pref := Preference{Marketing: false, UsageEmail: true}
		if e = t.Where("operator_id=? AND recipient_id=?", in.OperatorID, in.RecipientID).Limit(1).Find(&pref).Error; e != nil {
			return e
		}
		in.ID = uuid.NewString()
		in.Fingerprint = fp
		in.CreatedAt = time.Now().UTC()
		in.ReadAt = nil
		out = in
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		state := "pending"
		code := ""
		cipher := ""
		if (in.Category == "marketing" && !pref.Marketing) || (in.Category == "usage" && !pref.UsageEmail) {
			state = "suppressed"
			code = "preference"
		} else if !recipient.EmailVerified || !mail.ValidAddress(recipient.Email) {
			state = "needs_attention"
			code = "verified_email_required"
		} else {
			cipher, e = s.seal(mail.Message{ID: out.ID, To: recipient.Email, Brand: recipient.Brand, BrandID: recipient.BrandID, Subject: in.Title, Text: in.Body})
			if e != nil {
				return e
			}
		}
		return t.Table("operations_deliveries").Create(map[string]any{"notification_id": out.ID, "message_ciphertext": cipher, "state": state, "next_attempt_at": in.CreatedAt, "error_code": code}).Error
	})
	return out, e
}
func (s *Service) Announce(ctx context.Context, actor string, in Notification) (Notification, error) {
	if systemNotificationNamespace(in.BusinessKey) != "" {
		return Notification{}, ErrInvalid
	}
	var out Notification
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, in.OperatorID, "operations.manage"); e != nil {
			return e
		}
		var e error
		out, e = s.Emit(ctx, t, in)
		return e
	})
	return out, e
}
func (s *Service) Preferences(ctx context.Context, actor, operator string) (Preference, error) {
	out := Preference{OperatorID: operator, RecipientID: actor, UsageEmail: true}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		return t.Where("operator_id=? AND recipient_id=?", operator, actor).Limit(1).Find(&out).Error
	})
	return out, e
}
func (s *Service) SetPreferences(ctx context.Context, actor, operator string, marketing, usageEmail bool) (Preference, error) {
	out := Preference{OperatorID: operator, RecipientID: actor, Marketing: marketing, UsageEmail: usageEmail, UpdatedAt: time.Now().UTC()}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		return t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "recipient_id"}}, DoUpdates: clause.AssignmentColumns([]string{"marketing", "usage_email", "updated_at"})}).Create(&out).Error
	})
	return out, e
}
func (s *Service) ListNotifications(ctx context.Context, actor, operator string, before *time.Time, limit int) ([]Notification, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	out := []Notification{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		q := t.Where("operator_id=? AND recipient_id=?", operator, actor)
		if before != nil {
			q = q.Where("created_at<?", before.UTC())
		}
		return q.Order("created_at DESC,id DESC").Limit(limit).Find(&out).Error
	})
	return out, e
}
func (s *Service) UnreadNotificationCount(ctx context.Context, actor, operator string) (int64, error) {
	var count int64
	err := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		return t.Model(&Notification{}).Where("operator_id=? AND recipient_id=? AND read_at IS NULL", operator, actor).Count(&count).Error
	})
	return count, err
}
func (s *Service) ReadNotification(ctx context.Context, actor, operator, id string) error {
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		r := t.Model(&Notification{}).Where("id=? AND operator_id=? AND recipient_id=?", id, operator, actor).Update("read_at", time.Now().UTC())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrForbidden
		}
		return nil
	})
}

type Delivery struct {
	NotificationID string     `json:"notification_id"`
	State          string     `json:"state"`
	Attempts       int        `json:"attempts"`
	ErrorCode      string     `json:"error_code,omitempty"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
}

type ManagedNotification struct {
	Notification
	State         string     `json:"state"`
	Attempts      int        `json:"attempts"`
	ErrorCode     string     `json:"error_code,omitempty"`
	AcceptedAt    *time.Time `json:"accepted_at,omitempty"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
}

type NotificationNotice struct {
	ID              string    `json:"id"`
	RecipientID     string    `json:"recipient_id"`
	RecipientName   string    `json:"recipient_name"`
	RecipientEmail  string    `json:"recipient_email"`
	BusinessKey     string    `json:"business_key"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
	Category        string    `json:"category"`
	Reference       string    `json:"reference"`
	ReadStatus      string    `json:"read_status"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	SystemGenerated bool      `json:"system_generated"`
}
type NotificationGroup struct {
	Kind            string               `json:"record_kind"`
	Key             string               `json:"business_key"`
	Title           string               `json:"title"`
	Body            string               `json:"body"`
	Category        string               `json:"category"`
	CreatedAt       time.Time            `json:"created_at"`
	UnreadCount     int                  `json:"unread_count"`
	TotalCount      int                  `json:"total_count"`
	SystemGenerated bool                 `json:"system_generated"`
	Notices         []NotificationNotice `json:"notices"`
}

type notificationGroupCursor struct {
	At  time.Time `json:"at"`
	Key string    `json:"key"`
}

func (s *Service) ListNotificationGroups(ctx context.Context, actor, operator, cursor, search, state string, limit int) (NotificationGroup, []NotificationGroup, string, error) {
	if limit < 1 || limit > 100 || len(search) > 200 {
		return NotificationGroup{}, nil, "", ErrInvalid
	}
	var position notificationGroupCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &position) != nil || position.At.IsZero() || position.Key == "" {
			return NotificationGroup{}, nil, "", ErrInvalid
		}
	}
	var personal NotificationGroup
	var events []NotificationGroup
	next := ""
	err := s.within(ctx, nil, func(t *gorm.DB) error {
		if err := s.require(ctx, t, actor, operator, "operations.manage"); err != nil {
			return err
		}
		filter, args := notificationListFilter(search, state)
		personalNotices, err := s.notificationNotices(t, operator, "n.recipient_id=?"+filter, append([]any{actor}, args...))
		if err != nil {
			return err
		}
		sortNotificationNotices(personalNotices)
		personal = NotificationGroup{Kind: "personal", Key: "personal", Notices: personalNotices, TotalCount: len(personalNotices)}
		for _, notice := range personalNotices {
			if notice.ReadStatus == "unread" {
				personal.UnreadCount++
			}
		}
		type eventKey struct {
			BusinessKey, Title, Body, Category string
			CreatedAt                          time.Time
			TotalCount, UnreadCount            int
		}
		keys := []eventKey{}
		eventSQL := `SELECT business_key, title, body, category, created_at, total_count, unread_count FROM (
SELECT n.business_key, (array_agg(n.title ORDER BY n.created_at ASC))[1] AS title, (array_agg(n.body ORDER BY n.created_at ASC))[1] AS body, (array_agg(n.category ORDER BY n.created_at ASC))[1] AS category, MIN(n.created_at) AS created_at, COUNT(*) AS total_count, COUNT(*) FILTER (WHERE n.read_at IS NULL) AS unread_count
FROM operations_notifications n JOIN operations_deliveries d ON d.notification_id=n.id
WHERE n.operator_id=?` + filter + `
GROUP BY n.business_key
HAVING COUNT(*) FILTER (WHERE n.recipient_id<>?)>0
) groups`
		eventArgs := append([]any{operator}, args...)
		eventArgs = append(eventArgs, actor)
		if cursor != "" {
			eventSQL += ` WHERE (created_at, business_key) < (?, ?)`
			eventArgs = append(eventArgs, position.At, position.Key)
		}
		eventSQL += ` ORDER BY created_at DESC, business_key DESC LIMIT ?`
		eventArgs = append(eventArgs, limit+1)
		if err = t.Raw(eventSQL, eventArgs...).Scan(&keys).Error; err != nil {
			return err
		}
		if len(keys) > limit {
			keys = keys[:limit]
			last := keys[len(keys)-1]
			encoded, _ := json.Marshal(notificationGroupCursor{At: last.CreatedAt, Key: last.BusinessKey})
			next = base64.RawURLEncoding.EncodeToString(encoded)
		}
		if len(keys) == 0 {
			return nil
		}
		keyValues := make([]string, len(keys))
		for i, key := range keys {
			keyValues[i] = key.BusinessKey
		}
		notices, err := s.notificationNotices(t, operator, "n.business_key IN ?"+filter, append([]any{keyValues}, args...))
		if err != nil {
			return err
		}
		byKey := map[string][]NotificationNotice{}
		for _, notice := range notices {
			byKey[notice.BusinessKey] = append(byKey[notice.BusinessKey], notice)
		}
		for _, key := range keys {
			grouped := byKey[key.BusinessKey]
			sortNotificationNotices(grouped)
			events = append(events, NotificationGroup{Kind: "event", Key: key.BusinessKey, Title: key.Title, Body: key.Body, Category: key.Category, CreatedAt: key.CreatedAt, UnreadCount: key.UnreadCount, TotalCount: key.TotalCount, SystemGenerated: systemNotificationNamespace(key.BusinessKey) != "", Notices: grouped})
		}
		return nil
	})
	return personal, events, next, err
}
func notificationListFilter(search, state string) (string, []any) {
	clause := ""
	args := []any{}
	if state != "" {
		clause += " AND d.state=?"
		args = append(args, state)
	}
	if term := strings.ToLower(strings.TrimSpace(search)); term != "" {
		clause += " AND strpos(lower(n.title||' '||n.body||' '||n.reference),?)>0"
		args = append(args, term)
	}
	return clause, args
}
func (s *Service) notificationNotices(t *gorm.DB, operator, where string, args []any) ([]NotificationNotice, error) {
	rows := []struct {
		ID, RecipientID, RecipientName, RecipientEmail, BusinessKey, Title, Body, Category, Reference, State string
		CreatedAt                                                                                            time.Time
		ReadAt                                                                                               *time.Time
	}{}
	query := `SELECT n.id, n.recipient_id, COALESCE(NULLIF(st.display_name,''), NULLIF(cu.display_name,''), '') AS recipient_name, COALESCE(NULLIF(st.email,''), NULLIF(ident.email_normalized,''), '') AS recipient_email, n.business_key, n.title, n.body, n.category, n.reference, n.created_at, n.read_at, d.state
FROM operations_notifications n
JOIN operations_deliveries d ON d.notification_id=n.id
LEFT JOIN portal_staff st ON st.id=n.recipient_id
LEFT JOIN organization_customers cu ON cu.id=n.recipient_id
LEFT JOIN LATERAL (SELECT email_normalized FROM organization_customer_identities WHERE customer_id=n.recipient_id AND is_current=true LIMIT 1) ident ON true
WHERE n.operator_id=? AND ` + where
	if err := t.Raw(query, append([]any{operator}, args...)...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]NotificationNotice, len(rows))
	for i, row := range rows {
		status := "unread"
		if row.ReadAt != nil {
			status = "read"
		}
		out[i] = NotificationNotice{ID: row.ID, RecipientID: row.RecipientID, RecipientName: row.RecipientName, RecipientEmail: row.RecipientEmail, BusinessKey: row.BusinessKey, Title: row.Title, Body: row.Body, Category: row.Category, Reference: row.Reference, ReadStatus: status, State: row.State, CreatedAt: row.CreatedAt, SystemGenerated: systemNotificationNamespace(row.BusinessKey) != ""}
	}
	return out, nil
}
func sortNotificationNotices(notices []NotificationNotice) {
	sort.SliceStable(notices, func(i, j int) bool {
		if unread, other := notices[i].ReadStatus == "unread", notices[j].ReadStatus == "unread"; unread != other {
			return unread
		}
		return notices[i].CreatedAt.After(notices[j].CreatedAt)
	})
}
func (s *Service) ListOperatorNotifications(ctx context.Context, actor, operator, cursor, search, state string, limit int) ([]ManagedNotification, string, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	var position notificationCursor
	if cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(b, &position) != nil || position.At.IsZero() || !validID(position.ID) {
			return nil, "", ErrInvalid
		}
	}
	out := []ManagedNotification{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		q := t.Table("operations_notifications n").Select("n.*,d.state,d.attempts,d.error_code,d.accepted_at,d.next_attempt_at").Joins("JOIN operations_deliveries d ON d.notification_id=n.id").Where("n.operator_id=?", operator)
		if cursor != "" {
			q = q.Where("(n.created_at,n.id)<(?,?::uuid)", position.At, position.ID)
		}
		if state != "" {
			q = q.Where("d.state=?", state)
		}
		if term := strings.ToLower(strings.TrimSpace(search)); term != "" {
			q = q.Where("strpos(lower(n.title||' '||n.body||' '||n.reference),?)>0", term)
		}
		return q.Order("n.created_at DESC,n.id DESC").Limit(limit + 1).Scan(&out).Error
	})
	if e != nil {
		return nil, "", e
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		b, _ := json.Marshal(notificationCursor{At: last.CreatedAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	return out, next, nil
}

// RunNotifications uses at-least-once SMTP submission. A server can accept DATA
// before the worker crashes; stable Message-ID helps clients deduplicate, but
// accepted is deliberately never reported as delivered or read.
func (s *Service) RunNotifications(ctx context.Context, limit int) ([]Delivery, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var jobs []struct {
		NotificationID, MessageCiphertext string
		Attempts                          int
	}
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		now := time.Now().UTC()
		if e := t.Table("operations_deliveries").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("(state='pending' AND next_attempt_at<=?) OR (state='processing' AND lease_until<?)", now, now).Order("next_attempt_at").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for _, j := range jobs {
			if e := t.Table("operations_deliveries").Where("notification_id=?", j.NotificationID).Updates(map[string]any{"state": "processing", "lease_until": now.Add(45 * time.Second)}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := []Delivery{}
	for _, j := range jobs {
		var msg mail.Message
		callErr := s.open(j.MessageCiphertext, &msg)
		suppressed := false
		suppressionCode := "preference"
		if callErr == nil {
			// Preferences and the verified address can change while queued.
			var n Notification
			callErr = s.DB.WithContext(ctx).First(&n, "id=?", j.NotificationID).Error
			if callErr == nil && strings.HasPrefix(n.BusinessKey, restrictionReviewKey) {
				var current bool
				current, callErr = s.restrictionReviewCurrent(ctx, n)
				if callErr == nil && !current {
					suppressed = true
					suppressionCode = "review_resolved_or_access_changed"
				}
			}
			if callErr == nil && !suppressed {
				recipient, resolveErr := s.self(ctx, s.DB, n.RecipientID, n.OperatorID)
				callErr = resolveErr
				pref := Preference{UsageEmail: true}
				if callErr == nil {
					callErr = s.DB.WithContext(ctx).Where("operator_id=? AND recipient_id=?", n.OperatorID, n.RecipientID).Limit(1).Find(&pref).Error
				}
				suppressed = (n.Category == "marketing" && !pref.Marketing) || (n.Category == "usage" && !pref.UsageEmail)
				if callErr == nil && (!recipient.EmailVerified || !mail.ValidAddress(recipient.Email)) {
					callErr = ErrNotReady
				}
				msg.To = recipient.Email
				msg.Brand = recipient.Brand
				msg.BrandID = recipient.BrandID
			}
		}
		if callErr == nil && !suppressed {
			if !mail.AvailableForBrand(s.Mail, msg.BrandID) {
				callErr = ErrNotReady
			} else {
				work, cancel := context.WithTimeout(ctx, 25*time.Second)
				callErr = s.Mail.Send(work, msg)
				cancel()
			}
		}
		d := Delivery{NotificationID: j.NotificationID, State: "accepted", Attempts: j.Attempts + 1}
		updates := map[string]any{"attempts": d.Attempts, "lease_until": nil, "error_code": ""}
		if suppressed {
			d.State = "suppressed"
			updates["error_code"] = suppressionCode
		} else if callErr == nil {
			at := time.Now().UTC()
			d.AcceptedAt = &at
			updates["accepted_at"] = at
		} else {
			d.State = "pending"
			d.ErrorCode = "submission_failed"
			if callErr == ErrNotReady {
				d.ErrorCode = "smtp_unavailable"
			}
			if d.Attempts >= 12 {
				d.State = "needs_attention"
			}
			power := j.Attempts
			if power > 10 {
				power = 10
			}
			updates["next_attempt_at"] = time.Now().UTC().Add(time.Minute * time.Duration(1<<power))
			updates["error_code"] = d.ErrorCode
		}
		updates["state"] = d.State
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		dbErr := s.DB.WithContext(persist).Table("operations_deliveries").Where("notification_id=? AND state='processing'", j.NotificationID).Updates(updates).Error
		cancel()
		if dbErr != nil {
			return out, dbErr
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) RetryDelivery(ctx context.Context, actor, operator, id string) error {
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		var n Notification
		if e := t.First(&n, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		recipient, e := s.self(ctx, t, n.RecipientID, n.OperatorID)
		if e != nil {
			return e
		}
		if !recipient.EmailVerified || !mail.ValidAddress(recipient.Email) {
			return ErrNotReady
		}
		ciphertext, e := s.seal(mail.Message{ID: n.ID, To: recipient.Email, Brand: recipient.Brand, BrandID: recipient.BrandID, Subject: n.Title, Text: n.Body})
		if e != nil {
			return e
		}
		result := t.Table("operations_deliveries").Where("notification_id=? AND state='needs_attention'", id).Updates(map[string]any{"state": "pending", "attempts": 0, "next_attempt_at": time.Now().UTC(), "error_code": "", "message_ciphertext": ciphertext})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
}

// GetDelivery is a trusted worker read for durable acknowledgement workflows.
// SMTP acceptance is returned exactly as recorded; it is not mailbox delivery.
func (s *Service) GetDelivery(ctx context.Context, tx *gorm.DB, notificationID string) (Delivery, error) {
	var out Delivery
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Table("operations_deliveries").Select("notification_id,state,attempts,error_code,accepted_at,next_attempt_at,lease_until").Where("notification_id=?", notificationID).Take(&out).Error
	})
	return out, e
}
func (s *Service) GetNotificationRead(ctx context.Context, tx *gorm.DB, notificationID string) (*time.Time, error) {
	var out struct{ ReadAt *time.Time }
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Model(&Notification{}).Select("read_at").Where("id=?", notificationID).Take(&out).Error
	})
	return out.ReadAt, e
}
