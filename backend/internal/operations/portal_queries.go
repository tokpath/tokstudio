package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

type notificationCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (s *Service) ListNotificationsPage(ctx context.Context, actor, operator, cursor, query string, limit int) ([]Notification, string, error) {
	if limit < 1 || limit > 100 || len(query) > 200 {
		return nil, "", ErrInvalid
	}
	var c notificationCursor
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.At.IsZero() || !validID(c.ID) {
			return nil, "", ErrInvalid
		}
	}
	rows := []Notification{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if _, e := s.self(ctx, tx, actor, operator); e != nil {
			return e
		}
		q := tx.Where("operator_id=? AND recipient_id=?", operator, actor)
		if cursor != "" {
			q = q.Where("(created_at,id)<(?,?)", c.At, c.ID)
		}
		if query != "" {
			pattern := "%" + strings.TrimSpace(query) + "%"
			q = q.Where("(title ILIKE ? OR body ILIKE ?)", pattern, pattern)
		}
		return q.Order("created_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error
	})
	if e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		b, _ := json.Marshal(notificationCursor{last.CreatedAt, last.ID})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	return rows, next, nil
}

// CustomerInvoice is a trusted authenticated self-query across service ownership
// changes. It resolves the immutable issuer without trusting a browser supplied ID.
func (s *Service) CustomerInvoice(ctx context.Context, customer, id string) (Invoice, error) {
	if !validID(customer) || !validID(id) {
		return Invoice{}, ErrInvalid
	}
	var out Invoice
	err := s.DB.WithContext(ctx).Where("id=? AND party_kind='customer' AND customer_id=?", id, customer).First(&out).Error
	return out, err
}
