package remediation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type RefundExecutionClosureInput struct {
	BusinessKey               string `json:"business_key"`
	ExpectedExecutionKey      string `json:"expected_execution_key"`
	Reason                    string `json:"reason"`
	NoFurtherPaymentReference string `json:"no_further_payment_reference"`
	Evidence                  string `json:"evidence"`
}

// RefundExecutionClosure preserves the instruction which was independently
// verified to have ended. It does not cancel the customer's remaining debt.
type RefundExecutionClosure struct {
	ID                        string         `json:"id"`
	LegID                     string         `json:"leg_id"`
	OperatorID                string         `json:"operator_id"`
	RouteVersion              int64          `json:"route_version"`
	ExecutionKey              string         `json:"execution_key"`
	ExecutionMode             string         `json:"execution_mode"`
	PaymentRefundID           string         `json:"payment_refund_id"`
	BusinessKey               string         `json:"-"`
	Fingerprint               string         `json:"-"`
	Reason                    string         `json:"reason"`
	NoFurtherPaymentReference string         `json:"no_further_payment_reference"`
	Evidence                  string         `json:"evidence"`
	ActorID                   string         `json:"actor_id"`
	SettledMoney              ledger.Decimal `json:"settled_money"`
	RemainingMoney            ledger.Decimal `json:"remaining_money"`
	CreatedAt                 time.Time      `json:"created_at"`
}

func (RefundExecutionClosure) TableName() string { return "remediation_refund_execution_closures" }

type ManualRefundFailureInput struct {
	BusinessKey               string    `json:"business_key"`
	ExpectedExecutionKey      string    `json:"expected_execution_key"`
	Result                    string    `json:"result"`
	ExternalReference         string    `json:"external_reference"`
	NoFurtherPaymentReference string    `json:"no_further_payment_reference"`
	Evidence                  string    `json:"evidence"`
	VerifiedAt                time.Time `json:"verified_at"`
}

type ManualRefundFailure struct {
	ID                        string    `json:"id"`
	LegID                     string    `json:"leg_id"`
	OperatorID                string    `json:"operator_id"`
	RouteVersion              int64     `json:"route_version"`
	ExecutionKey              string    `json:"execution_key"`
	BusinessKey               string    `json:"-"`
	Fingerprint               string    `json:"-"`
	Result                    string    `json:"result"`
	ExternalReference         string    `json:"external_reference"`
	NoFurtherPaymentReference string    `json:"no_further_payment_reference"`
	Evidence                  string    `json:"evidence"`
	VerifiedAt                time.Time `json:"verified_at"`
	VerifiedBy                string    `json:"verified_by"`
	CreatedAt                 time.Time `json:"created_at"`
}

func (ManualRefundFailure) TableName() string { return "remediation_manual_refund_failures" }

// RecordManualRefundFailure verifies an external terminal result, not a
// timeout or an internal decision to try another account. It never releases
// money: the remaining customer principal stays held until further settlement.
func (s *Service) RecordManualRefundFailure(ctx context.Context, actor, op, legID string, in ManualRefundFailureInput) (ManualRefundFailure, error) {
	var out ManualRefundFailure
	if in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.ExpectedExecutionKey == "" || (in.Result != "failed" && in.Result != "cancelled") || strings.TrimSpace(in.ExternalReference) == "" || len(in.ExternalReference) > 1000 || strings.TrimSpace(in.NoFurtherPaymentReference) == "" || len(in.NoFurtherPaymentReference) > 1000 || strings.TrimSpace(in.Evidence) == "" || len(in.Evidence) > 10000 || in.VerifiedAt.IsZero() {
		return out, ErrInvalid
	}
	in.VerifiedAt = in.VerifiedAt.UTC().Truncate(time.Microsecond)
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		c, l, e := s.lockLeg(ctx, t, op, legID)
		if e != nil {
			return e
		}
		fp := fingerprint(in)
		e = t.First(&out, "leg_id=? AND business_key=?", legID, in.BusinessKey).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if l.ExecutionMode != "manual" || l.ExecutionKey != in.ExpectedExecutionKey || (l.State != "executing" && l.State != "unknown" && l.State != "exception") || l.SettledMoney.Cmp(l.AmountMoney) >= 0 {
			return ErrState
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		if in.VerifiedAt.Before(l.CreatedAt) || in.VerifiedAt.After(at.Add(time.Minute)) {
			return ErrInvalid
		}
		out = ManualRefundFailure{ID: uuid.NewString(), LegID: legID, OperatorID: op, RouteVersion: l.RouteVersion, ExecutionKey: l.ExecutionKey, BusinessKey: in.BusinessKey, Fingerprint: fp, Result: in.Result, ExternalReference: in.ExternalReference, NoFurtherPaymentReference: in.NoFurtherPaymentReference, Evidence: in.Evidence, VerifiedAt: in.VerifiedAt, VerifiedBy: actor, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		l.State, c.State = "exception", "exception"
		if e = t.Save(&l).Error; e != nil {
			return e
		}
		if e = t.Save(&c).Error; e != nil {
			return e
		}
		return s.event(t, op, legID, "refund.manual_execution_failed", actor, in.Evidence, map[string]any{"case_id": c.ID, "failure": out})
	})
	return out, e
}

// CloseRefundExecution opens a new destination version only after finance has
// verified that the old instruction will make no further payment. Unknown
// outcomes and payment observations not yet matched to this debt remain blocked.
func (s *Service) CloseRefundExecution(ctx context.Context, actor, op, legID string, in RefundExecutionClosureInput) (RefundExecutionClosure, error) {
	var out RefundExecutionClosure
	if in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.ExpectedExecutionKey == "" || (in.Reason != "failed" && in.Reason != "terminated" && in.Reason != "settled_partial") || strings.TrimSpace(in.NoFurtherPaymentReference) == "" || len(in.NoFurtherPaymentReference) > 1000 || strings.TrimSpace(in.Evidence) == "" || len(in.Evidence) > 10000 {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		c, l, e := s.lockLeg(ctx, t, op, legID)
		if e != nil {
			return e
		}
		fp := fingerprint(in)
		e = t.First(&out, "leg_id=? AND business_key=?", legID, in.BusinessKey).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if l.State == "unknown" {
			return ErrReview
		}
		if l.ExecutionKey != in.ExpectedExecutionKey || (l.State != "executing" && l.State != "exception") || (l.ExecutionMode != "manual" && l.ExecutionMode != "provider") || l.SettledMoney.Cmp(l.AmountMoney) >= 0 {
			return ErrState
		}
		if in.Reason == "settled_partial" && l.SettledMoney.Sign() <= 0 {
			return ErrState
		}
		if l.ExecutionMode == "manual" && l.SettledMoney.IsZero() && l.MisdirectedMoney.IsZero() {
			var n int64
			if e = t.Model(&ManualRefundFailure{}).Where("leg_id=? AND route_version=? AND execution_key=?", legID, l.RouteVersion, l.ExecutionKey).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 || l.State != "exception" {
				return ErrReview
			}
		}
		if s.Payments != nil {
			facts, e := s.Payments.RefundExecutionClosureFacts(ctx, t, op, legID)
			if e != nil {
				return e
			}
			if l.ExecutionMode == "provider" && len(facts) == 0 {
				return ErrReview
			}
			for _, f := range facts {
				switch f.State {
				case "failed", "cancelled":
				case "succeeded", "verified_pending_settlement":
					if f.FactID == "" {
						return ErrReview
					}
					var matched int64
					if e = t.Table("remediation_payment_fact_bindings").Where("operator_id=? AND leg_id=? AND fact_id=?", op, legID, "provider:"+f.FactID).Count(&matched).Error; e != nil {
						return e
					}
					if matched != 1 {
						return ErrReview
					}
				default:
					return ErrReview
				}
			}
		} else if l.ExecutionMode == "provider" {
			return ErrNotReady
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = RefundExecutionClosure{ID: uuid.NewString(), LegID: legID, OperatorID: op, RouteVersion: l.RouteVersion, ExecutionKey: l.ExecutionKey, ExecutionMode: l.ExecutionMode, PaymentRefundID: l.PaymentRefundID, BusinessKey: in.BusinessKey, Fingerprint: fp, Reason: in.Reason, NoFurtherPaymentReference: in.NoFurtherPaymentReference, Evidence: in.Evidence, ActorID: actor, SettledMoney: l.SettledMoney, RemainingMoney: l.AmountMoney.Sub(l.SettledMoney), CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		if s.Payments != nil {
			if e = s.Payments.SealRefundExecutions(ctx, t, op, legID, out.ID); e != nil {
				return e
			}
		}
		l.RouteVersion++
		l.ExecutionKey, l.ExecutionMode, l.PaymentRefundID = "", "", ""
		l.State = "approved"
		if e = t.Save(&l).Error; e != nil {
			return e
		}
		c.State = "paying"
		if e = t.Save(&c).Error; e != nil {
			return e
		}
		return s.event(t, op, legID, "refund.execution_closed", actor, in.Evidence, map[string]any{"case_id": c.ID, "closure": out, "next_route_version": l.RouteVersion})
	})
	return out, e
}

func (s *Service) destinationForProof(t *gorm.DB, l RefundLeg, authorizationID string) (RefundDestinationAuthorization, error) {
	var a RefundDestinationAuthorization
	if e := t.First(&a, "id=? AND leg_id=? AND operator_id=?", authorizationID, l.ID, l.OperatorID).Error; e != nil {
		return a, e
	}
	if a.ReceiptID != l.ReceiptID || a.SettledBeforeMoney.Add(a.AmountMoney).Cmp(l.AmountMoney) != 0 || a.RouteVersion > l.RouteVersion {
		return a, ErrConflict
	}
	if a.RouteVersion < l.RouteVersion {
		var count int64
		if e := t.Model(&RefundExecutionClosure{}).Where("leg_id=? AND route_version=?", l.ID, a.RouteVersion).Count(&count).Error; e != nil {
			return a, e
		}
		if count != 1 {
			return a, ErrConflict
		}
	}
	return a, nil
}

func (s *Service) historicalProviderInstruction(t *gorm.DB, l RefundLeg, refundID string) (bool, error) {
	var n int64
	e := t.Model(&RefundExecutionClosure{}).Where("leg_id=? AND execution_mode='provider' AND payment_refund_id=?", l.ID, refundID).Count(&n).Error
	return n > 0, e
}

type RefundRouteVersion struct {
	Version              int64              `json:"version"`
	AuthorizationID      string             `json:"authorization_id,omitempty"`
	Destination          *RefundDestination `json:"destination,omitempty"`
	AuthorizedAt         *time.Time         `json:"authorized_at,omitempty"`
	AmountMoney          ledger.Decimal     `json:"amount_money"`
	SettledBeforeMoney   ledger.Decimal     `json:"settled_before_money"`
	ClosureID            string             `json:"closure_id,omitempty"`
	ClosedAt             *time.Time         `json:"closed_at,omitempty"`
	ClosedReason         string             `json:"closed_reason,omitempty"`
	ResponsibleExecution bool               `json:"responsible_execution"`
}

func (s *Service) refundRouteHistory(t *gorm.DB, l RefundLeg) ([]RefundRouteVersion, error) {
	out := []RefundRouteVersion{}
	var auths []RefundDestinationAuthorization
	if e := t.Where("leg_id=?", l.ID).Order("route_version").Find(&auths).Error; e != nil {
		return out, e
	}
	var closures []RefundExecutionClosure
	if e := t.Where("leg_id=?", l.ID).Order("route_version").Find(&closures).Error; e != nil {
		return out, e
	}
	byVersion := map[int64]RefundRouteVersion{}
	for _, a := range auths {
		d, at := a.RefundDestination, a.CreatedAt
		byVersion[a.RouteVersion] = RefundRouteVersion{Version: a.RouteVersion, AuthorizationID: a.ID, Destination: &d, AuthorizedAt: &at, AmountMoney: a.AmountMoney, SettledBeforeMoney: a.SettledBeforeMoney, ResponsibleExecution: a.ResponsibleExecution}
	}
	for _, c := range closures {
		v := byVersion[c.RouteVersion]
		v.Version, v.ClosureID, v.ClosedAt, v.ClosedReason = c.RouteVersion, c.ID, &c.CreatedAt, c.Reason
		if v.AuthorizationID == "" {
			v.AmountMoney = c.SettledMoney.Add(c.RemainingMoney)
		}
		byVersion[c.RouteVersion] = v
	}
	for version := int64(1); version <= l.RouteVersion; version++ {
		if v, ok := byVersion[version]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *Service) RefundRouteHistory(ctx context.Context, actor, op, legID string) ([]RefundRouteVersion, error) {
	out := []RefundRouteVersion{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		var l RefundLeg
		if e := t.First(&l, "id=? AND operator_id=?", legID, op).Error; e != nil {
			return e
		}
		var e error
		out, e = s.refundRouteHistory(t, l)
		return e
	})
	return out, e
}
