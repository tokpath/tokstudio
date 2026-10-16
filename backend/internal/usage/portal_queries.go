package usage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PortalCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func RequestCursor(r Request) string {
	b, _ := json.Marshal(PortalCursor{r.AcceptedAt, r.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

// ListRequestsPage is a customer self-query. It never returns request content,
// credentials, another customer's requests or a supplier's upstream evidence.
func (s *Service) ListRequestsPage(ctx context.Context, customer, cursor, query string, limit int) ([]Request, error) {
	if _, e := uuid.Parse(customer); e != nil || limit < 1 || limit > 100 {
		return nil, ErrKey
	}
	q := s.DB.WithContext(ctx).Where("customer_id=?", customer)
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil {
			return nil, ErrKey
		}
		var c PortalCursor
		if json.Unmarshal(raw, &c) != nil || c.At.IsZero() {
			return nil, ErrKey
		}
		if _, e = uuid.Parse(c.ID); e != nil {
			return nil, ErrKey
		}
		q = q.Where("(accepted_at,id)<(?,?)", c.At, c.ID)
	}
	if query != "" {
		if len(query) > 200 {
			return nil, ErrKey
		}
		pattern := "%" + query + "%"
		q = q.Where("(id::text ILIKE ? OR model_id ILIKE ? OR status ILIKE ?)", pattern, pattern, pattern)
	}
	rows := []Request{}
	e := q.Order("accepted_at DESC,id DESC").Limit(limit).Find(&rows).Error
	return rows, e
}
