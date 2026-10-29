package remediation

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChargebackInput struct {
	ActorID        string         `json:"actor_id"`
	OperatorID     string         `json:"operator_id"`
	ReceiptID      string         `json:"receipt_id"`
	Provider       string         `json:"provider"`
	ProviderCaseID string         `json:"provider_case_id"`
	AmountMoney    ledger.Decimal `json:"amount_money"`
	Reason         string         `json:"reason"`
	Evidence       string         `json:"evidence"`
}
type Chargeback struct {
	ID                    string         `json:"id"`
	OperatorID            string         `json:"operator_id"`
	ReceiptID             string         `json:"receipt_id"`
	Provider              string         `json:"provider"`
	ProviderCaseID        string         `json:"provider_case_id"`
	Fingerprint           string         `json:"-"`
	AmountMoney           ledger.Decimal `json:"amount_money"`
	RecoveredMoney        ledger.Decimal `json:"recovered_money"`
	RestrictionsAppliedAt *time.Time     `json:"restrictions_applied_at,omitempty"`
	State                 string         `json:"state"`
	Reason                string         `json:"reason"`
	Evidence              string         `json:"evidence"`
	CreatedAt             time.Time      `json:"created_at"`
	ResolvedAt            *time.Time     `json:"resolved_at,omitempty"`
}

func (Chargeback) TableName() string { return "remediation_chargebacks" }

type chargebackHold struct {
	ID, ChargebackID, OperatorID                      string
	CustomerID, GrantID, OrderID, WalletReservationID *string
	HoldID, SourceAccountID                           string
	Amount                                            ledger.Decimal
	Kind, State                                       string
	CreatedAt                                         time.Time
}

func (chargebackHold) TableName() string { return "remediation_chargeback_holds" }

type chargebackEvent struct {
	ChargebackID, SourceEventID, OperatorID string
	AmountMoney                             ledger.Decimal
	State                                   string
}

func (chargebackEvent) TableName() string { return "remediation_chargeback_events" }

// RecordChargeback records a verified external collection loss even if its
// customer effect is not yet known. A merchant bank freeze is not proof that
// any customer's originally valid payment became invalid.
func (s *Service) RecordChargeback(ctx context.Context, in ChargebackInput) (Chargeback, error) {
	var out Chargeback
	if !validAmount(in.AmountMoney) || in.Provider == "" || in.ProviderCaseID == "" || in.Reason == "" || in.Evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorID, in.OperatorID, "refunds.manage"); e != nil {
			return e
		}
		if e := s.lockReceiptBusiness(ctx, t, in.OperatorID, in.ReceiptID); e != nil {
			return e
		}
		if e := lock(t, in.OperatorID+":receipt:"+in.ReceiptID); e != nil {
			return e
		}
		e := t.First(&out, "operator_id=? AND provider=? AND provider_case_id=?", in.OperatorID, in.Provider, in.ProviderCaseID).Error
		if e == nil {
			if out.Fingerprint != fingerprint(in) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		r, e := s.Commerce.GetReceipt(ctx, t, in.OperatorID, in.ReceiptID)
		if e != nil {
			return e
		}
		if in.AmountMoney.Cmp(r.GrossMoney) > 0 {
			return ErrInvalid
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Chargeback{ID: uuid.NewString(), OperatorID: in.OperatorID, ReceiptID: in.ReceiptID, Provider: in.Provider, ProviderCaseID: in.ProviderCaseID, Fingerprint: fingerprint(in), AmountMoney: in.AmountMoney, State: "disputed", Reason: in.Reason, Evidence: in.Evidence, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		if e = s.flagChargebackCommissionSources(ctx, t, in.ActorID, out, r.GrossMoney); e != nil {
			return e
		}
		loss, e := s.account(ctx, t, in.OperatorID, in.OperatorID, "control:collection_loss")
		if e != nil {
			return e
		}
		cash, e := s.account(ctx, t, in.OperatorID, in.OperatorID, "control:external_cash")
		if e != nil {
			return e
		}
		if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: in.OperatorID, BusinessKey: "chargeback:" + out.ID, Kind: "external_chargeback", SourceID: out.ID, ActorID: in.ActorID, Reason: in.Evidence, Lines: []ledger.Line{{AccountID: loss.ID, Amount: in.AmountMoney.Neg()}, {AccountID: cash.ID, Amount: in.AmountMoney}}}); e != nil {
			return e
		}
		return s.event(t, in.OperatorID, out.ID, "chargeback.recorded", in.ActorID, in.Evidence, out)
	})
	return out, e
}
func fraction(amount, numerator, denominator ledger.Decimal) ledger.Decimal {
	if denominator.Sign() <= 0 {
		return ledger.Zero()
	}
	d, _ := ledger.Quantize(new(big.Rat).Mul(amount.Rat(), new(big.Rat).Quo(numerator.Rat(), denominator.Rat())))
	return d
}
func min(a, b ledger.Decimal) ledger.Decimal {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

// RestrictChargeback applies only to traced receipt assets within the current
// operator's authority. After a takeover the original collector records its
// loss, but cannot seize the brand's newly assumed customer assets.
func (s *Service) RestrictChargeback(ctx context.Context, actor, op, id, evidence string) error {
	return s.within(ctx, nil, func(t *gorm.DB) error { return s.applyChargebackRestrictions(ctx, t, actor, op, id, evidence) })
}

func (s *Service) ResolveChargeback(ctx context.Context, actor, op, id, classification, evidence string) (Chargeback, error) {
	var out Chargeback
	if evidence == "" || (classification != "invalid_payment" && classification != "merchant_collection_risk" && classification != "reversed") {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		var e error
		out, e = s.lockChargeback(ctx, t, op, id)
		if e != nil {
			return e
		}
		if out.State == classification {
			return nil
		}
		if out.State != "disputed" {
			return ErrState
		}
		if classification == "invalid_payment" && out.RestrictionsAppliedAt == nil {
			return ErrReview
		}
		if classification == "reversed" && out.RecoveredMoney.Cmp(out.AmountMoney) != 0 {
			return ErrState
		}
		if e := lock(t, op+":receipt:"+out.ReceiptID); e != nil {
			return e
		}
		r, e := s.Commerce.GetReceipt(ctx, t, op, out.ReceiptID)
		if e != nil {
			return e
		}
		if classification == "invalid_payment" {
			var already ledger.Decimal
			if e = t.Model(&capacity{}).Select("COALESCE(sum(amount_money),0)::text").Where("operator_id=? AND receipt_id=? AND state<>'released'", op, out.ReceiptID).Row().Scan(&already); e != nil {
				return e
			}
			var other ledger.Decimal
			if e = t.Model(&Chargeback{}).Select("COALESCE(sum(amount_money),0)::text").Where("operator_id=? AND receipt_id=? AND id<>? AND state='invalid_payment'", op, out.ReceiptID, id).Row().Scan(&other); e != nil {
				return e
			}
			if already.Add(other).Add(out.AmountMoney).Cmp(r.GrossMoney) > 0 {
				return ErrConflict
			}
		}
		var holds []chargebackHold
		if e = t.Where("chargeback_id=? AND state='held'", id).Order("customer_id,id").Find(&holds).Error; e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		customers := map[string]bool{}
		for _, h := range holds {
			if h.CustomerID != nil {
				own, e := s.Organization.CurrentOwnership(ctx, t, *h.CustomerID)
				if e != nil {
					return e
				}
				if _, e = s.Organization.LockOwnership(ctx, t, *h.CustomerID, h.OperatorID, own.Version); e != nil {
					return e
				}
				customers[*h.CustomerID] = true
			}
			if classification != "invalid_payment" {
				if h.WalletReservationID != nil {
					if e = s.Commerce.ReleaseWalletRefund(ctx, t, op, *h.CustomerID, *h.WalletReservationID); e != nil {
						return e
					}
				}
				if _, e = s.Ledger.Release(ctx, t, op, h.HoldID, "chargeback-release:"+id); e != nil {
					return e
				}
				h.State = "released"
			} else if h.Kind == "grant" {
				if _, e = s.Ledger.CancelReservedGrant(ctx, t, op, *h.CustomerID, *h.GrantID, h.HoldID, "chargeback:"+id+":"+h.ID, actor, evidence, at); e != nil {
					return e
				}
				h.State = "cancelled"
			} else {
				if h.WalletReservationID != nil {
					if e = s.Commerce.CompleteWalletRefund(ctx, t, op, *h.CustomerID, *h.WalletReservationID); e != nil {
						return e
					}
				}
				loss, e := s.account(ctx, t, op, op, "control:collection_loss")
				if e != nil {
					return e
				}
				if _, e = s.Ledger.Capture(ctx, t, h.HoldID, ledger.Posting{OperatorID: op, BusinessKey: "chargeback-invalid:" + h.ID, Kind: "invalid_payment_assets", SourceID: id, ActorID: actor, Reason: evidence, Lines: []ledger.Line{{AccountID: h.SourceAccountID, Amount: h.Amount.Neg()}, {AccountID: loss.ID, Amount: h.Amount}}}, nil); e != nil {
					return e
				}
				if h.Kind == "order_funds" && h.OrderID != nil && h.CustomerID != nil {
					if e = s.Commerce.MarkFundingException(ctx, t, op, *h.CustomerID, *h.OrderID, evidence); e != nil {
						return e
					}
				}
				h.State = "cancelled"
			}
			if e = t.Save(&h).Error; e != nil {
				return e
			}
		}
		var events []chargebackEvent
		if e = t.Where("chargeback_id=?", id).Order("source_event_id").Find(&events).Error; e != nil {
			return e
		}
		if e = s.Incentives.ReleaseCommissionRestrictions(ctx, t, actor, op, "chargeback:"+id, evidence); e != nil {
			return e
		}
		for _, event := range events {
			if classification == "invalid_payment" {
				if e = s.Incentives.ReverseEvent(ctx, t, incentives.CorrectionInput{ActorStaffID: actor, OperatorID: event.OperatorID, OriginalEventID: event.SourceEventID, BusinessKey: "chargeback:" + id + ":" + event.SourceEventID, Evidence: evidence, AmountMoney: event.AmountMoney}); e != nil {
					return e
				}
			}
			if event.State == "disputed" {
				if e = s.Incentives.ReleaseEventDispute(ctx, t, actor, event.OperatorID, event.SourceEventID, "chargeback:"+id, evidence); e != nil {
					return e
				}
			}
			state := "released"
			if classification == "invalid_payment" {
				state = "reversed"
			}
			if e = t.Model(&chargebackEvent{}).Where("chargeback_id=? AND source_event_id=?", id, event.SourceEventID).Update("state", state).Error; e != nil {
				return e
			}
		}
		// Released credits which expired during the dispute do not acquire a new
		// lifetime. Their original paid-basis expiry still follows confirmation.
		for customer := range customers {
			events, e := s.Ledger.ExpireCredits(ctx, t, op, customer, at)
			if e != nil {
				return e
			}
			for _, event := range events {
				g, e := s.Ledger.GetGrant(ctx, t, event.GrantID)
				if e != nil {
					return e
				}
				if e = s.Incentives.ObserveGrantEvent(ctx, t, g, event); e != nil {
					return e
				}
			}
		}
		out.State = classification
		claimState := "released"
		if classification == "invalid_payment" {
			claimState = "invalid"
		}
		if e = t.Table("remediation_chargeback_order_claims").Where("chargeback_id=?", id).Update("state", claimState).Error; e != nil {
			return e
		}
		out.ResolvedAt = &at
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		return s.event(t, op, id, "chargeback.resolved", actor, evidence, out)
	})
	return out, e
}

// RecordChargebackRecovery records verified cash recovered from the provider or
// collection guarantor. It does not claim that previously invalid customer funds
// became valid; that would require a separate documented business correction.
func (s *Service) RecordChargebackRecovery(ctx context.Context, actor, op, id, key, evidence string, amount ledger.Decimal) (Chargeback, error) {
	var out Chargeback
	if key == "" || evidence == "" || !validAmount(amount) {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=? AND operator_id=?", id, op).Error; e != nil {
			return e
		}
		var old struct {
			AmountMoney ledger.Decimal
			Evidence    string
		}
		q := t.Table("remediation_chargeback_recoveries").Where("chargeback_id=? AND business_key=?", id, key).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.AmountMoney.Cmp(amount) != 0 || old.Evidence != evidence {
				return ErrConflict
			}
			return nil
		}
		if out.RecoveredMoney.Add(amount).Cmp(out.AmountMoney) > 0 {
			return ErrInvalid
		}
		cash, e := s.account(ctx, t, op, op, "control:external_cash")
		if e != nil {
			return e
		}
		loss, e := s.account(ctx, t, op, op, "control:collection_loss")
		if e != nil {
			return e
		}
		if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: op, BusinessKey: "chargeback-recovered:" + id + ":" + key, Kind: "chargeback_recovery", SourceID: id, ActorID: actor, Reason: evidence, Lines: []ledger.Line{{AccountID: cash.ID, Amount: amount.Neg()}, {AccountID: loss.ID, Amount: amount}}}); e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		if e = t.Exec("INSERT INTO remediation_chargeback_recoveries(id,chargeback_id,business_key,amount_money,evidence,created_at) VALUES(?,?,?,?,?,?)", uuid.NewString(), id, key, amount, evidence, at).Error; e != nil {
			return e
		}
		out.RecoveredMoney = out.RecoveredMoney.Add(amount)
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		return s.event(t, op, id, "chargeback.cash_recovered", actor, evidence, map[string]any{"amount_money": amount, "reference": key})
	})
	return out, e
}
func (s *Service) Chargebacks(ctx context.Context, op string) ([]Chargeback, error) {
	var out []Chargeback
	e := s.DB.WithContext(ctx).Where("operator_id=?", op).Order("created_at DESC,id").Limit(200).Find(&out).Error
	return out, e
}
