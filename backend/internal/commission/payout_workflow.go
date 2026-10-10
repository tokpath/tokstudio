package commission

import (
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PayoutInput struct {
	AmountMinor int64     `json:"amount_minor"`
	OperationID string    `json:"operation_id"`
	Method      string    `json:"method"`
	OccurredAt  time.Time `json:"occurred_at"`
	Confirmed   bool      `json:"confirmed"`
	Reference   string    `json:"reference"`
	Note        string    `json:"note"`
}

func (s *Service) RecordPayoutTx(tx *gorm.DB, scope WorkflowScope, settlementID string, in PayoutInput) (*SettlementView, bool, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.Note = strings.TrimSpace(in.Note)
	in.Method = strings.TrimSpace(in.Method)
	if in.Method == "" {
		in.Method = "manual"
	}
	in.OccurredAt = in.OccurredAt.UTC().Truncate(time.Microsecond)
	if in.AmountMinor <= 0 || !in.Confirmed || in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return nil, false, ErrInvalid
	}
	payload := struct {
		PayoutInput
		SettlementID string
	}{in, settlementID}
	var out SettlementView
	replayed, err := loadWorkflowOperationTx(tx, scope, in.OperationID, "payout", payload, &out)
	if err != nil || replayed {
		return &out, false, err
	}
	result, err := s.payoutRecordTx(tx, settlementID, in, scope.ActorUserID, scope.OwnerID, scope.Channels, scope.ReferenceChannels)
	if err != nil {
		return nil, false, err
	}
	if err := saveWorkflowOperationTx(tx, scope, in.OperationID, "payout", payload, result); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

// Cash entries, payout record, settlement lifecycle and the caller's audit share
// one transaction. No external transfer is initiated.
func (s *Service) payoutRecordTx(tx *gorm.DB, settlementID string, in PayoutInput, actor, scopeID string, channels, referenceChannels []string) (*SettlementView, error) {
	in.Method = strings.TrimSpace(in.Method)
	in.Reference = strings.TrimSpace(in.Reference)
	in.Note = strings.TrimSpace(in.Note)
	if in.Method != "manual" || len(in.Reference) > 200 || len(in.Note) > 1000 {
		return nil, ErrInvalid
	}
	if err := lockSettlementLifecycle(tx); err != nil {
		return nil, err
	}
	var row settleRow
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", settlementID)
	if channels != nil {
		q = q.Where("COALESCE(channel_org_id,'') IN ?", channels)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if row.Status == StatusPaid {
		var paid payoutRow
		if err := tx.Where("settlement_id=?", settlementID).First(&paid).Error; err != nil {
			return nil, err
		}
		// Only legacy trusted callers retain their previous receipt-based replay.
		// Public operations replay by immutable actor+brand+operation identity.
		if !strings.HasPrefix(scopeID, "legacy:") || paid.Method != in.Method || paid.Reference == nil || *paid.Reference != in.Reference {
			return nil, ErrConflict
		}
		view := settleView(row)
		applyPayoutView(view, paid)
		return view, nil
	}
	if row.Status != StatusSettled {
		return nil, ErrSettlementChanged
	}
	if in.AmountMinor != 0 && in.AmountMinor != row.AmountMinor {
		return nil, ErrSettlementChanged
	}
	if in.Reference != "" {
		var count int64
		if len(referenceChannels) == 0 {
			referenceChannels = channels
		}
		q := tx.Model(&payoutRow{}).Where("scope_id=?", scopeID)
		if len(referenceChannels) > 0 {
			originalIDs := tx.Model(&settleRow{}).Select("id").Where("COALESCE(channel_org_id,'') IN ?", referenceChannels)
			q = tx.Model(&payoutRow{}).Where("scope_id=? OR (scope_id LIKE 'legacy:%' AND settlement_id IN (?))", scopeID, originalIDs)
		}
		if err := q.Where("reference=?", in.Reference).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrConflict
		}
	}
	var entries []entryRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("settlement_id=?", settlementID).Order("id").Find(&entries).Error; err != nil {
		return nil, err
	}
	var total int64
	for _, entry := range entries {
		if entry.Status != StatusSettled {
			return nil, ErrSettlementChanged
		}
		total += entry.AmountMinor
	}
	if len(entries) == 0 || total <= 0 || total != row.AmountMinor {
		return nil, ErrSettlementChanged
	}
	if s.cash != nil {
		for _, entry := range entries {
			if err := s.cash.PayoutCommissionTx(tx, entry.ID, settlementID, entry.AmountMinor); err != nil {
				if errors.Is(err, billing.ErrNotFound) || errors.Is(err, billing.ErrInsufficientBalance) || errors.Is(err, billing.ErrConflict) {
					return nil, ErrWalletMismatch
				}
				return nil, err
			}
		}
	}
	payout := payoutRow{ID: id.New("cpo"), SettlementID: settlementID, Method: in.Method, Reference: &in.Reference, ActorUserID: &actor, Status: StatusPaid, ScopeID: scopeID, Note: in.Note, CreatedAt: time.Now().UTC()}
	if !in.OccurredAt.IsZero() {
		copy := in.OccurredAt
		payout.OccurredAt = &copy
	}
	if err := tx.Create(&payout).Error; err != nil {
		return nil, err
	}
	row.Status = StatusPaid
	if err := tx.Save(&row).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&entryRow{}).Where("settlement_id=? AND status=?", settlementID, StatusSettled).Update("status", StatusPaid).Error; err != nil {
		return nil, err
	}
	view := settleView(row)
	applyPayoutView(view, payout)
	return view, nil
}
func applyPayoutView(view *SettlementView, paid payoutRow) {
	view.PayoutID = paid.ID
	view.PayoutMethod = paid.Method
	view.PayoutOccurredAt = paid.OccurredAt
	view.PayoutRecordedAt = &paid.CreatedAt
	view.PayoutNote = paid.Note
	if paid.Reference != nil {
		view.PayoutReference = *paid.Reference
	}
}
