package remediation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

// assumedRefundSource reads only the exact original receipt already linked to
// the customer's migrated assets. It never opens the former merchant's config.
func (s *Service) assumedRefundSource(ctx context.Context, t *gorm.DB, l RefundLeg) (commerce.Receipt, error) {
	var c Case
	if e := t.First(&c, "id=? AND operator_id=?", l.CaseID, l.OperatorID).Error; e != nil {
		return commerce.Receipt{}, e
	}
	if c.CustomerID == nil {
		if c.Kind == "unallocated" && l.SourceOperatorID == l.OperatorID {
			return s.Commerce.GetReceipt(ctx, t, l.SourceOperatorID, l.ReceiptID)
		}
		return commerce.Receipt{}, ErrScope
	}
	if l.OperatorID != l.SourceOperatorID {
		if e := s.Organization.RequireAssumedCustomerResponsibility(ctx, t, l.OperatorID, l.SourceOperatorID, *c.CustomerID); e != nil {
			return commerce.Receipt{}, e
		}
	} else {
		own, e := s.Organization.CurrentOwnership(ctx, t, *c.CustomerID)
		if e != nil {
			return commerce.Receipt{}, e
		}
		if own.OperatorID != l.OperatorID {
			return commerce.Receipt{}, ErrScope
		}
	}
	r, e := s.Commerce.GetReceipt(ctx, t, l.SourceOperatorID, l.ReceiptID)
	if e != nil {
		return r, e
	}
	switch c.Kind {
	case "wallet":
		allocations, e := s.Commerce.WalletRefundAllocations(ctx, t, l.OperatorID, *c.CustomerID, c.ID)
		if e != nil {
			return r, e
		}
		var links []struct{ AllocationID string }
		if e = t.Table("remediation_wallet_links").Where("leg_id=?", l.ID).Find(&links).Error; e != nil {
			return r, e
		}
		allowed := map[string]bool{}
		for _, link := range links {
			allowed[link.AllocationID] = true
		}
		total := ledger.Zero()
		for _, a := range allocations {
			if allowed[a.ID] && a.SourceOperatorID == l.SourceOperatorID && a.ReceiptID == l.ReceiptID {
				total = total.Add(a.AmountUSD)
			}
		}
		if total.Cmp(l.AmountUSD) != 0 {
			return r, ErrScope
		}
	case "package_exception":
		if c.OrderID == nil {
			return r, ErrScope
		}
		o, e := s.Commerce.GetOrder(ctx, t, *c.CustomerID, *c.OrderID)
		if e != nil {
			return r, e
		}
		if o.OperatorID != l.OperatorID || o.OriginalOperatorID != l.SourceOperatorID {
			return r, ErrScope
		}
		funding, e := s.Commerce.OrderWalletFunding(ctx, t, l.OperatorID, *c.CustomerID, o.ID)
		if e != nil {
			return r, e
		}
		found := false
		for _, a := range funding {
			if a.ReceiptID == l.ReceiptID && a.SourceOperatorID == l.SourceOperatorID {
				found = true
			}
		}
		// Direct external funding is also included in the order's immutable
		// allocations; use the public per-receipt exposure projection.
		if !found {
			exposure, e := s.Commerce.ReceiptExposure(ctx, t, l.SourceOperatorID, l.ReceiptID)
			if e != nil {
				return r, e
			}
			for _, p := range exposure.Orders {
				if p.OrderID == o.ID && p.CustomerID == *c.CustomerID {
					found = true
				}
			}
		}
		if !found {
			return r, ErrScope
		}
	default:
		return r, ErrScope
	}
	return r, nil
}

// RequireResponsibleRefundExecution authorizes recording an actual execution
// from the CURRENT responsible operator's own account. It never grants access
// to an original collector's merchant or changes the original receipt scope.
func (s *Service) RequireResponsibleRefundExecution(ctx context.Context, t *gorm.DB, actor, op, legID string, d payments.ExternalRefundDestination) (payments.ResponsibleRefundBasis, error) {
	var out payments.ResponsibleRefundBasis
	if t == nil {
		return out, ErrInvalid
	}
	if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return out, e
	}
	_, l, e := s.lockLeg(ctx, t, op, legID)
	if e != nil {
		return out, e
	}
	return s.responsibleRefundBasis(ctx, t, l, d)
}

func (s *Service) responsibleRefundBasis(ctx context.Context, t *gorm.DB, l RefundLeg, d payments.ExternalRefundDestination) (payments.ResponsibleRefundBasis, error) {
	var out payments.ResponsibleRefundBasis
	if l.State != "executing" && l.State != "unknown" && l.State != "exception" && l.State != "completed" && !(l.RouteVersion > 1 && l.State == "approved") {
		return out, ErrState
	}
	a, e := s.destinationForProof(t, l, d.AuthorizationReference)
	if e != nil {
		return out, e
	}
	if !a.ResponsibleExecution {
		return out, ErrReview
	}
	if a.RouteVersion == l.RouteVersion && (l.ExecutionMode != "manual" || l.ExecutionKey == "") {
		return out, ErrState
	}
	n, e := normalizeRefundDestination(RefundDestination{Network: d.Network, Asset: d.Asset, TokenContract: d.TokenContract, Address: d.Address})
	if e != nil {
		return out, e
	}
	if n != a.RefundDestination || d.AuthorizationReference != a.ID {
		return out, ErrConflict
	}
	r, e := s.assumedRefundSource(ctx, t, l)
	if e != nil {
		return out, e
	}
	return payments.ResponsibleRefundBasis{RefundID: l.ID, OperatorID: l.OperatorID, SourceOperatorID: l.SourceOperatorID, ReceiptID: l.ReceiptID, Currency: r.Currency, AmountUSD: l.AmountUSD, RemainingUSD: l.AmountUSD.Sub(l.SettledUSD), AuthorizedAt: a.CreatedAt, CreatedAt: l.CreatedAt}, nil
}

func (s *Service) validateAssumedDestination(ctx context.Context, t *gorm.DB, l RefundLeg, d RefundDestination) error {
	r, e := s.assumedRefundSource(ctx, t, l)
	if e != nil {
		return e
	}
	if d.Asset != r.Currency {
		return ErrConflict
	}
	if r.Provider == "evm_wallet" || strings.HasPrefix(r.Provider, "evm_wallet:") {
		var evidence struct {
			Network       string `json:"chain_id"`
			TokenContract string `json:"token_contract"`
		}
		if json.Unmarshal([]byte(r.VerificationEvidence), &evidence) != nil || evidence.Network == "" || evidence.TokenContract == "" {
			return ErrReview
		}
		expected, e := normalizeRefundDestination(RefundDestination{Network: evidence.Network, Asset: r.Currency, TokenContract: evidence.TokenContract, Address: d.Address})
		if e != nil || d != expected {
			return ErrConflict
		}
	} else if r.Provider == "nowpayments" || strings.HasPrefix(r.Provider, "nowpayments:") {
		// Provider currency codes are preserved. The new payer's independently
		// verified network/contract metadata and explicit consent must match;
		// the old merchant is never decrypted to infer it.
		if d.Network == "" || d.TokenContract == "" {
			return ErrReview
		}
	} else if d.Network != "bank" || d.TokenContract != "" {
		return ErrConflict
	}
	return nil
}
