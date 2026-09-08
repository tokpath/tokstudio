package commission

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	MarketingFrozen         = "frozen"
	MarketingIssued         = "issued"
	MarketingKindCommission = "commission"
	MarketingKindCredit     = "credit"
)

type marketingRow struct {
	ID                string    `gorm:"column:id;primaryKey"`
	ChannelOrgID      string    `gorm:"column:channel_org_id"`
	Kind              string    `gorm:"column:kind"`
	Status            string    `gorm:"column:status"`
	AmountMinor       int64     `gorm:"column:amount_minor"`
	UsageEventID      *string   `gorm:"column:usage_event_id"`
	CommissionEntryID *string   `gorm:"column:commission_entry_id"`
	SourceType        string    `gorm:"column:source_type"`
	SourceID          string    `gorm:"column:source_id"`
	ReversalOf        *string   `gorm:"column:reversal_of"`
	IdempotencyKey    string    `gorm:"column:idempotency_key"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (marketingRow) TableName() string { return "commission_marketing_entries" }

func writeMarketing(tx *gorm.DB, channelID, kind, status string, amount int64, usageID, entryID, sourceType, sourceID, idem string, reversalOf *string) error {
	if channelID == "" || amount == 0 {
		return nil
	}
	row := marketingRow{
		ID: id.New("mkt"), ChannelOrgID: channelID, Kind: kind, Status: status,
		AmountMinor: amount, SourceType: sourceType, SourceID: sourceID,
		IdempotencyKey: idem, CreatedAt: time.Now().UTC(), ReversalOf: reversalOf,
	}
	if usageID != "" {
		row.UsageEventID = &usageID
	}
	if entryID != "" {
		row.CommissionEntryID = &entryID
	}
	return tx.Where("idempotency_key = ?", idem).FirstOrCreate(&row).Error
}

type MarketingTotals struct {
	FrozenMinor int64 `json:"frozen_minor"`
	IssuedMinor int64 `json:"issued_minor"`
}

func (s *Service) MarketingTotals(ctx context.Context, channelID string) (MarketingTotals, error) {
	var out MarketingTotals
	if channelID == "" {
		return out, nil
	}
	var rows []struct {
		Status string
		Total  int64
	}
	err := s.db.WithContext(ctx).Model(&marketingRow{}).
		Select("status, COALESCE(SUM(amount_minor),0) AS total").
		Where("channel_org_id = ?", channelID).
		Group("status").Scan(&rows).Error
	for _, row := range rows {
		switch row.Status {
		case MarketingFrozen:
			out.FrozenMinor = row.Total
		case MarketingIssued:
			out.IssuedMinor = row.Total
		}
	}
	return out, err
}
