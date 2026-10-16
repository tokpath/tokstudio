package commerce

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type ManualGiftInput struct {
	ActorStaffID             string         `json:"actor_staff_id"`
	OperatorID               string         `json:"operator_id"`
	CustomerID               string         `json:"customer_id"`
	BusinessKey              string         `json:"business_key"`
	Reason                   string         `json:"reason"`
	Evidence                 string         `json:"evidence"`
	AmountATG                ledger.Decimal `json:"amount_atg"`
	ExpiresAt                *time.Time     `json:"expires_at"`
	ExpectedOwnershipVersion int64          `json:"expected_ownership_version"`
}
type ManualGift struct {
	ID                       string         `json:"id"`
	OperatorID               string         `json:"operator_id"`
	OriginalOperatorID       string         `json:"original_operator_id"`
	CustomerID               string         `json:"customer_id"`
	GrantID                  string         `json:"grant_id"`
	OwnershipVersion         int64          `json:"ownership_version"`
	OriginalOwnershipVersion int64          `json:"original_ownership_version"`
	BusinessKey              string         `json:"-"`
	Fingerprint              string         `json:"-"`
	AmountATG                ledger.Decimal `json:"amount_atg"`
	Reason                   string         `json:"reason"`
	Evidence                 string         `json:"evidence"`
	ActorID                  string         `json:"actor_id"`
	CoverageReference        string         `json:"coverage_reference"`
	CoverageVersion          int64          `json:"coverage_version"`
	EffectiveAt              time.Time      `json:"effective_at"`
	ExpiresAt                *time.Time     `json:"expires_at"`
	CreatedAt                time.Time      `json:"created_at"`
}

func (ManualGift) TableName() string { return "commerce_manual_gifts" }

// IssueManualGift is an independently funded zero-paid grant, never a rewrite
// of an order, usage charge, cash wallet or a customer's earned commission.
func (s *Service) IssueManualGift(ctx context.Context, tx *gorm.DB, in ManualGiftInput) (ManualGift, error) {
	var out ManualGift
	if !validID(in.OperatorID) || !validID(in.CustomerID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || (in.Reason != "compensation" && in.Reason != "targeted_gift") || strings.TrimSpace(in.Evidence) == "" || len(in.Evidence) > 4000 || !in.AmountATG.Valid() || in.AmountATG.Sign() <= 0 || in.ExpectedOwnershipVersion <= 0 {
		return out, ErrInvalidInput
	}
	if in.ExpiresAt != nil {
		normalized := in.ExpiresAt.UTC().Truncate(time.Microsecond)
		in.ExpiresAt = &normalized
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.org.RequirePermission(ctx, t, in.ActorStaffID, in.OperatorID, "marketing.manage"); e != nil {
			return e
		}
		if _, e := s.org.LockOwnership(ctx, t, in.CustomerID, in.OperatorID, in.ExpectedOwnershipVersion); e != nil {
			return e
		}
		if e := s.lockBusinessScope(ctx, t, in.OperatorID); e != nil {
			return e
		}
		if e := lockCommand(t, "manual-gift", in.OperatorID+":"+in.CustomerID+":"+in.BusinessKey); e != nil {
			return e
		}
		fp := fingerprint(in)
		e := t.First(&out, "operator_id=? AND customer_id=? AND business_key=?", in.OperatorID, in.CustomerID, in.BusinessKey).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if s.coverage == nil {
			return ErrNotReady
		}
		if e = s.org.RequireBusiness(ctx, t, in.OperatorID, organization.Activity); e != nil {
			return e
		}
		customer, e := s.org.GetCustomer(ctx, t, in.CustomerID)
		if e != nil {
			return e
		}
		if customer.Status != "active" && customer.Status != "stopped" {
			return ErrOrderState
		}
		var at time.Time
		if e = t.Raw("SELECT clock_timestamp()").Row().Scan(&at); e != nil {
			return e
		}
		at = at.UTC()
		if in.ExpiresAt != nil && !in.ExpiresAt.After(at) {
			return ErrInvalidInput
		}
		id := uuid.NewString()
		// SalesCoverage uses a commitment proposal DTO; no purchase order or
		// fictional receipt is created for this independent gift.
		proposal := Order{ID: id, OperatorID: in.OperatorID, CustomerID: in.CustomerID, OwnershipVersion: in.ExpectedOwnershipVersion, MerchantGiftATG: in.AmountATG}
		commitment, e := s.coverage.Reserve(ctx, t, proposal)
		if e != nil {
			return e
		}
		grant, e := s.ledger.IssueGrant(ctx, t, ledger.GrantInput{OperatorID: in.OperatorID, CustomerID: in.CustomerID, BusinessKey: "manual-gift:" + id, ActorID: in.ActorStaffID, Reason: in.Reason + " | " + in.Evidence, PaidUSD: ledger.Zero(), Sources: []ledger.GrantSource{{Kind: "manual_gift", Amount: in.AmountATG}}, EffectiveAt: at, ExpiresAt: in.ExpiresAt})
		if e != nil {
			return e
		}
		if e = s.coverage.Fulfill(ctx, t, proposal); e != nil {
			return e
		}
		out = ManualGift{ID: id, OperatorID: in.OperatorID, OriginalOperatorID: in.OperatorID, CustomerID: in.CustomerID, GrantID: grant.ID, OwnershipVersion: in.ExpectedOwnershipVersion, OriginalOwnershipVersion: in.ExpectedOwnershipVersion, BusinessKey: in.BusinessKey, Fingerprint: fp, AmountATG: in.AmountATG, Reason: in.Reason, Evidence: in.Evidence, ActorID: in.ActorStaffID, CoverageReference: commitment.Reference, CoverageVersion: commitment.Version, EffectiveAt: at, ExpiresAt: in.ExpiresAt, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return s.event(t, in.OperatorID, id, "manual_gift.delivered", map[string]any{"actor_id": in.ActorStaffID, "customer_id": in.CustomerID, "evidence": in.Evidence, "before": nil, "after": out})
	})
	return out, e
}

func (s *Service) ListManualGifts(ctx context.Context, tx *gorm.DB, actor, op, customer, cursor string, limit int) ([]ManualGift, string, error) {
	if !validID(op) || limit < 1 || limit > 200 || (customer != "" && !validID(customer)) {
		return nil, "", ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, tx, actor, op, "marketing.manage"); e != nil {
		return nil, "", e
	}
	if customer != "" {
		own, e := s.org.CurrentOwnership(ctx, tx, customer)
		if e != nil {
			return nil, "", e
		}
		if own.OperatorID != op {
			return nil, "", ErrScope
		}
	}
	at, id, e := organization.ParseAuditCursor(cursor)
	if e != nil {
		return nil, "", ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Where("operator_id=?", op)
	if customer != "" {
		q = q.Where("customer_id=?", customer)
	}
	if at != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *at, id)
	}
	rows := []ManualGift{}
	if e = q.Order("created_at DESC,id DESC").Limit(limit).Find(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) == limit {
		last := rows[len(rows)-1]
		next = organization.NextAuditCursor([]organization.AuditEvent{{ID: last.ID, CreatedAt: last.CreatedAt}}, 1)
	}
	return rows, next, nil
}
