package cooperation

import (
	"context"
	"gorm.io/gorm"
	"time"
)

func businessTime(ctx context.Context, tx *gorm.DB) (time.Time, error) {
	var now time.Time
	e := tx.WithContext(ctx).Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now.UTC(), e
}
