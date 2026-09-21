package commission

import (
	"bytes"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"time"
)

// RecalculateTx uses immutable original calculation inputs, never today's policy or attribution.
// Correct entries are a no-op, so retries do not extend freezing or debit cash again.
func (s *Service) RecalculateTx(tx *gorm.DB, usageID string, base int64) (*billing.CommissionView, error) {
	if err := lockSettlementLifecycle(tx); err != nil {
		return nil, err
	}
	rows := []entryRow{}
	if err := tx.Where("usage_event_id = ? AND status <> ?", usageID, StatusReversed).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	view := &billing.CommissionView{UsageEventID: usageID, Status: "no_active_commission"}
	if len(rows) == 0 {
		return view, nil
	}
	var policy PolicyView
	if len(rows[0].PolicySnapshot) == 0 || json.Unmarshal(rows[0].PolicySnapshot, &policy) != nil {
		return nil, ErrSnapshotUnavailable
	}
	in := AccrueInput{UsageEventID: usageID, WholesaleMinor: base, CanCommission: true}
	for _, row := range rows {
		if !bytes.Equal(row.PolicySnapshot, rows[0].PolicySnapshot) {
			return nil, ErrSnapshotUnavailable
		}
		if row.BeneficiaryRoleID == nil {
			return nil, ErrSnapshotUnavailable
		}
		switch row.Kind {
		case KindDirect:
			in.RoleID = *row.BeneficiaryRoleID
		case KindIndirect:
			in.ParentRoleID = *row.BeneficiaryRoleID
		default:
			return nil, ErrSnapshotUnavailable
		}
	}
	expected := map[string]split{}
	for _, part := range splitCommission(in, &policy) {
		expected[part.Kind] = part
	}
	if len(expected) != len(rows) {
		return nil, ErrSnapshotUnavailable
	}
	changed := false
	for _, row := range rows {
		part, ok := expected[row.Kind]
		if !ok {
			return nil, ErrSnapshotUnavailable
		}
		if row.AmountMinor != part.Amount || row.RawAmountMinor != part.Raw || row.BaseAmountMinor != base {
			changed = true
		}
		view.AmountMinor += part.Amount
	}
	view.ID = rows[0].ID
	view.Status = rows[0].Status
	view.PolicyVersion = policy.Version
	if !changed {
		return view, nil
	}
	// Released or paid amounts require an explicit financial correction, not a silent recalculation.
	for _, row := range rows {
		if row.Status != StatusFrozen {
			return nil, ErrConflict
		}
	}
	if err := s.ReverseTx(tx, usageID); err != nil {
		return nil, err
	}
	for _, original := range rows {
		part := expected[original.Kind]
		row := original
		row.ID = id.New("cme")
		row.CreatedAt = time.Now().UTC()
		row.AmountMinor = part.Amount
		row.RawAmountMinor = part.Raw
		row.BaseAmountMinor = base
		row.SourceEntryID = &original.ID
		row.IdempotencyKey = "cme-recalc:" + original.ID
		row.SettlementID = nil
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
		if row.ChannelOrgID != nil {
			if err := writeMarketing(tx, *row.ChannelOrgID, MarketingKindCommission, MarketingFrozen, -row.AmountMinor, usageID, row.ID, "recalculation", original.ID, "mkt-frz:"+row.ID, &original.ID); err != nil {
				return nil, err
			}
		}
		view.ID = row.ID
	}
	view.Changed = true
	return view, nil
}
