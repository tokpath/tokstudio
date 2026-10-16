package organization

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) ApplyChannelTakeoverOwnership(ctx context.Context, tx *gorm.DB, in TakeoverOwnershipInput) (out []Ownership, err error) {
	// A supplied transaction is mandatory: a standalone ownership change would
	// break the atomic transfer of all asset and future fulfilment obligations.
	if tx == nil || !validID(in.ChannelID) || !validID(in.BrandOwnerOperatorID) || strings.TrimSpace(in.EvidenceReference) == "" {
		return nil, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var channel operatorRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&channel, "id=?", in.ChannelID).Error; err != nil {
			return err
		}
		if channel.Kind != Channel || channel.ParentOperatorID == nil || *channel.ParentOperatorID != in.BrandOwnerOperatorID {
			return fmt.Errorf("%w: source %s kind=%s parent=%v target=%s", ErrInvalidRelationship, channel.ID, channel.Kind, channel.ParentOperatorID, in.BrandOwnerOperatorID)
		}
		brand, err := s.GetBrand(ctx, tx, channel.BrandID)
		if err != nil {
			return err
		}
		if brand.OwnerOperatorID != in.BrandOwnerOperatorID {
			return fmt.Errorf("%w: brand %s owner=%s target=%s", ErrInvalidRelationship, brand.ID, brand.OwnerOperatorID, in.BrandOwnerOperatorID)
		}
		var rows []ownershipRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND ended_at IS NULL", channel.ID).Order("customer_id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(in.ExpectedVersions) {
			return ErrStaleOwnership
		}
		for _, r := range rows {
			if in.ExpectedVersions[r.CustomerID] != r.Version {
				return ErrStaleOwnership
			}
		}
		// Ownership begins at the database registration instant. Use the same
		// clock when closing it; host clock skew must not invert its lifetime.
		var now time.Time
		if err = tx.Raw(`SELECT clock_timestamp()`).Scan(&now).Error; err != nil {
			return err
		}
		now = now.UTC()
		out = make([]Ownership, 0, len(rows))
		for _, old := range rows {
			if err = tx.Model(&ownershipRow{}).Where("id=? AND ended_at IS NULL", old.ID).Update("ended_at", now).Error; err != nil {
				return err
			}
			next := ownershipRow{ID: newID(), CustomerID: old.CustomerID, BrandID: old.BrandID, OperatorID: in.BrandOwnerOperatorID, Version: old.Version + 1, EffectiveAt: now, EvidenceReference: in.EvidenceReference}
			if err = tx.Create(&next).Error; err != nil {
				return err
			}
			out = append(out, Ownership(next))
		}
		// Existing settlement remains open; new channel business is stopped.
		if err = tx.Model(&businessRow{}).Where("operator_id=?", channel.ID).Updates(map[string]any{"registration": false, "new_purchase": false, "renewal": false, "model_usage": false, "activity": false, "version": gorm.Expr("version+1"), "updated_at": now}).Error; err != nil {
			return err
		}
		return s.event(tx, channel.ID, "", "channel.ownership_taken_over", in.EvidenceReference, map[string]any{"to_operator_id": in.BrandOwnerOperatorID, "customers": len(rows)})
	})
	return
}
