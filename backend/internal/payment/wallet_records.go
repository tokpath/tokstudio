package payment

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/pagecursor"
	"time"
)

type PersonalOrder struct {
	ID          string    `json:"id"`
	Purpose     string    `json:"purpose"`
	Currency    string    `json:"currency"`
	AmountMinor int64     `json:"amount_minor"`
	CreditMinor int64     `json:"credit_minor"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
type PersonalOrderPage struct {
	Items      []PersonalOrder `json:"items"`
	Total      int64           `json:"total"`
	NextCursor string          `json:"next_cursor"`
}

func (s *Service) PersonalOrders(ctx context.Context, userID, cursor string) (*PersonalOrderPage, error) {
	pos, err := pagecursor.Decode(cursor, userID, "orders")
	if err != nil {
		return nil, err
	}
	out := &PersonalOrderPage{Items: []PersonalOrder{}}
	q := s.db.WithContext(ctx).Model(&orderRow{}).Where("user_id=?", userID)
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	if pos != nil {
		q = q.Where("(created_at,id)<(?,?)", pos.At, pos.ID)
	}
	if err := q.Select("id,purpose,currency,amount_minor,credit_minor,status,created_at").Order("created_at DESC,id DESC").Limit(26).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		last := out.Items[24]
		out.NextCursor = pagecursor.Encode(last.CreatedAt, last.ID, userID, "orders")
	}
	return out, nil
}
