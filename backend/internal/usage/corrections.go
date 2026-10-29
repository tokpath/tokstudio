package usage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChargeCorrectionInput struct {
	OperatorID  string         `json:"operator_id"`
	RequestID   string         `json:"request_id"`
	BusinessKey string         `json:"business_key"`
	ActorID     string         `json:"actor_id"`
	Evidence    string         `json:"evidence"`
	AmountATG   ledger.Decimal `json:"amount_atg"`
}
type ChargeCorrection struct {
	ID          string          `json:"id"`
	RequestID   string          `json:"request_id"`
	OperatorID  string          `json:"operator_id"`
	BusinessKey string          `json:"business_key"`
	Fingerprint string          `json:"-"`
	AmountATG   ledger.Decimal  `json:"amount_atg"`
	ReturnedATG ledger.Decimal  `json:"returned_atg"`
	PaidMoney     ledger.Decimal  `json:"paid_money"`
	Reversals   json.RawMessage `gorm:"type:jsonb" json:"reversals"`
	Evidence    string          `json:"evidence"`
	ActorID     string          `json:"actor_id"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (ChargeCorrection) TableName() string { return "usage_charge_corrections" }

// CorrectCustomerCharge corrects the retail charge and original Key window.
// It cannot fabricate a procurement refund: supplier billing corrections are
// approved separately by the responsible seller against its original charge.
func (s *Service) CorrectCustomerCharge(ctx context.Context, tx *gorm.DB, in ChargeCorrectionInput, commissions *incentives.Service) (ChargeCorrection, error) {
	var out ChargeCorrection
	if !in.AmountATG.Valid() || in.AmountATG.Sign() <= 0 || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.Evidence == "" || commissions == nil {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorID, in.OperatorID, "refunds.manage"); e != nil {
			return e
		}
		var req Request
		if e := t.First(&req, "id=?", in.RequestID).Error; e != nil {
			return e
		}
		own, e := s.Organization.CurrentOwnership(ctx, t, req.CustomerID)
		if e != nil {
			return e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, req.CustomerID, in.OperatorID, own.Version); e != nil {
			return e
		}
		if e = s.Cooperation.Lock(ctx, t, in.OperatorID); e != nil {
			return e
		}
		if e = commissions.LockBusiness(ctx, t, in.OperatorID); e != nil {
			return e
		}
		var key Key
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, "id=?", req.KeyID).Error; e != nil {
			return e
		}
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&req, "id=?", req.ID).Error; e != nil {
			return e
		}
		e = t.First(&out, "request_id=? AND business_key=?", in.RequestID, in.BusinessKey).Error
		if e == nil {
			if out.Fingerprint != s.fingerprint(in) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if req.Status != "completed" || req.CorrectedATG.Add(in.AmountATG).Cmp(req.ActualATG) > 0 {
			return ErrConflict
		}
		at, e := s.businessTime(t)
		if e != nil {
			return e
		}
		// A reduction first eliminates an unpaid merchant shortfall. Return only
		// the amount the customer would not owe after the corrected total.
		newActual := req.ActualATG.Sub(req.CorrectedATG).Sub(in.AmountATG)
		targetReturned := req.ChargedATG.Sub(newActual)
		if targetReturned.Sign() < 0 {
			targetReturned = ledger.Zero()
		}
		toReturn := targetReturned.Sub(req.CorrectedChargedATG)
		var settlement Settlement
		if e = json.Unmarshal(req.Settlement, &settlement); e != nil {
			return e
		}
		reversals := []ledger.GrantReversal{}
		paid := ledger.Zero()
		if toReturn.Sign() > 0 {
			weights := []ledger.Decimal{}
			for _, event := range settlement.Customer.Events {
				var previous ledger.Decimal
				if e = t.Model(&ledger.GrantReversal{}).Select("COALESCE(sum(amount),0)::text").Where("original_event_id=?", event.ID).Row().Scan(&previous); e != nil {
					return e
				}
				weights = append(weights, event.Amount.Sub(previous))
			}
			parts, e := ledger.Proportional(toReturn, weights)
			if e != nil {
				return e
			}
			for i, event := range settlement.Customer.Events {
				if parts[i].IsZero() {
					continue
				}
				r, e := s.Ledger.ReverseUsage(ctx, t, ledger.ReverseUsageInput{OperatorID: in.OperatorID, CustomerID: req.CustomerID, OriginalEventID: event.ID, BusinessKey: "request:" + req.ID + ":" + in.BusinessKey + ":" + event.ID, ActorID: in.ActorID, Reason: in.Evidence, Amount: parts[i], At: at})
				if e != nil {
					return e
				}
				if r.Reversal.PaidMoney.Sign() > 0 {
					if e = commissions.ReverseEvent(ctx, t, incentives.CorrectionInput{ActorStaffID: in.ActorID, OperatorID: in.OperatorID, OriginalEventID: event.ID, BusinessKey: "usage-correction:" + r.Reversal.ID, Evidence: in.Evidence, AmountMoney: r.Reversal.PaidMoney}); e != nil {
						return e
					}
				}
				if e = s.observe(ctx, t, r.ExpiryEvents); e != nil {
					return e
				}
				reversals = append(reversals, r.Reversal)
				paid = paid.Add(r.Reversal.PaidMoney)
			}
		}
		b, _ := json.Marshal(reversals)
		out = ChargeCorrection{ID: uuid.NewString(), RequestID: req.ID, OperatorID: in.OperatorID, BusinessKey: in.BusinessKey, Fingerprint: s.fingerprint(in), AmountATG: in.AmountATG, ReturnedATG: toReturn, PaidMoney: paid, Reversals: b, Evidence: in.Evidence, ActorID: in.ActorID, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return t.Model(&Request{}).Where("id=?", req.ID).Updates(map[string]any{"corrected_atg": req.CorrectedATG.Add(in.AmountATG), "corrected_charged_atg": targetReturned}).Error
	})
	return out, e
}
