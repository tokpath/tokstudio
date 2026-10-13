package page

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

func Size(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// BeforeCreated applies a stable boundary within the caller's already-filtered
// query. Older clients may still send a last-row ID; resolve it in that scope.
func BeforeCreated(q *gorm.DB, raw string) (*gorm.DB, error) {
	if raw == "" {
		return q, nil
	}
	if len(raw) > 1024 {
		return nil, ErrCursor
	}
	at, key, err := Decode(raw)
	if err != nil {
		if len(raw) > 128 {
			return nil, ErrCursor
		}
		var anchor struct {
			ID        string
			CreatedAt time.Time
		}
		err = q.Session(&gorm.Session{}).Select("id, created_at").Where("id = ?", raw).Take(&anchor).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCursor
		}
		if err != nil {
			return nil, err
		}
		at, key = anchor.CreatedAt, anchor.ID
	}
	return q.Where("(created_at, id) < (?, ?)", at, key), nil
}
