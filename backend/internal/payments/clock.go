package payments

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// databaseTime is the authority for business eligibility and durable deadlines.
// Protocol signatures still use the transport's wall clock as required by the
// external protocol; a client timestamp never decides order/refund eligibility.
func databaseTime(ctx context.Context, db *gorm.DB) (time.Time, error) {
	var now time.Time
	e := db.WithContext(ctx).Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now.UTC(), e
}
