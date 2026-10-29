package cooperation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type ProcurementRefundAccounting struct {
	RefundID    string         `gorm:"primaryKey" json:"refund_id"`
	BusinessKey string         `json:"business_key"`
	Fingerprint string         `json:"-"`
	FeeMoney    ledger.Decimal `json:"fee_money"`
	SettledAt   time.Time      `json:"settled_at"`
	Evidence    string         `json:"evidence"`
	VerifiedBy  string         `json:"verified_by"`
	JournalID   *string        `json:"journal_id,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (ProcurementRefundAccounting) TableName() string {
	return "cooperation_procurement_refund_accounting"
}

// ConfirmRefundAccounting records a later verified refund fee exactly once.
// The supplier bears this operating cost; the completed buyer refund and its
// original-receipt capacity are never changed by this accounting fact.
func (p *ProcurementRefunds) ConfirmRefundAccounting(ctx context.Context, actor string, in ProcurementRefundAccounting) (ProcurementRefundAccounting, error) {
	out := in
	if !p.Ready() || in.BusinessKey == "" || in.Evidence == "" || !in.FeeMoney.Valid() || in.FeeMoney.Sign() < 0 || in.SettledAt.IsZero() {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=?", in.RefundID).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
			return e
		}
		if leg.State != "completed" {
			return ErrConflict
		}
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		if in.SettledAt.After(now.Add(5 * time.Minute)) {
			return ErrConflict
		}
		fp := hash([]string{in.RefundID, in.BusinessKey, in.FeeMoney.String(), in.SettledAt.UTC().Format(time.RFC3339Nano), in.Evidence})
		var old ProcurementRefundAccounting
		q := tx.Where("refund_id=?", in.RefundID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			out = old
			return nil
		}
		out.Fingerprint = fp
		out.VerifiedBy = actor
		out.CreatedAt = now
		out.JournalID = nil
		if in.FeeMoney.Sign() > 0 {
			fees, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: leg.SupplierID, OwnerID: leg.SupplierID, Purpose: "control:refund_fees", Unit: "MONEY", AllowNegative: true})
			if e != nil {
				return e
			}
			cash, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: leg.SupplierID, OwnerID: leg.SupplierID, Purpose: "control:external_cash", Unit: "MONEY", AllowNegative: true})
			if e != nil {
				return e
			}
			journal, e := p.Cooperation.Ledger.Post(ctx, tx, ledger.Posting{OperatorID: leg.SupplierID, BusinessKey: "procurement-refund-accounting:" + leg.ID, Kind: "procurement_refund_fee", SourceID: leg.ID, ActorID: actor, Reason: in.Evidence, Lines: []ledger.Line{{AccountID: fees.ID, Amount: in.FeeMoney.Neg()}, {AccountID: cash.ID, Amount: in.FeeMoney}}})
			if e != nil {
				return e
			}
			out.JournalID = &journal.ID
		}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Model(&leg).Updates(map[string]any{"fee_status": "confirmed", "fee_money": in.FeeMoney}).Error
	})
	return out, e
}
