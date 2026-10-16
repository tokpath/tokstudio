package remediation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

// RefundDestination names the exact asset and receiving endpoint. It never
// inherits the sender of the original receipt.
type RefundDestination struct {
	Network       string `json:"network"`
	Asset         string `json:"asset"`
	TokenContract string `json:"token_contract"`
	Address       string `json:"address"`
}

type RefundDestinationProposalInput struct {
	Destination            RefundDestination `json:"destination"`
	AuthorizationReference string            `json:"authorization_reference"`
	ResponsibleExecution   bool              `json:"responsible_execution"`
}

type RefundDestinationProposal struct {
	ID                     string `json:"id"`
	LegID                  string `json:"leg_id"`
	OperatorID             string `json:"operator_id"`
	CustomerID             string `json:"customer_id"`
	BusinessKey            string `json:"-"`
	Fingerprint            string `json:"-"`
	RefundDestination      `gorm:"embedded"`
	AuthorizationReference string    `json:"authorization_reference"`
	ResponsibleExecution   bool      `json:"responsible_execution"`
	RouteVersion           int64     `json:"route_version"`
	CreatedAt              time.Time `json:"created_at"`
}

func (RefundDestinationProposal) TableName() string {
	return "remediation_refund_destination_proposals"
}

type RefundDestinationAuthorizationInput struct {
	ProposalID                  string            `json:"proposal_id"`
	Destination                 RefundDestination `json:"destination"`
	PayerAuthorizationReference string            `json:"payer_authorization_reference"`
	VerificationEvidence        string            `json:"verification_evidence"`
	ResponsibleExecution        bool              `json:"responsible_execution"`
	RouteExceptionReference     string            `json:"route_exception_reference"`
}

type RefundDestinationAuthorization struct {
	ID                          string         `json:"id"`
	LegID                       string         `json:"leg_id"`
	OperatorID                  string         `json:"operator_id"`
	ReceiptID                   string         `json:"receipt_id"`
	ProposalID                  *string        `json:"proposal_id,omitempty"`
	CustomerID                  *string        `json:"customer_id,omitempty"`
	AmountUSD                   ledger.Decimal `json:"amount_usd"`
	SettledBeforeUSD            ledger.Decimal `json:"settled_before_usd"`
	RouteVersion                int64          `json:"route_version"`
	BusinessKey                 string         `json:"-"`
	Fingerprint                 string         `json:"-"`
	RefundDestination           `gorm:"embedded"`
	PayerAuthorizationReference string    `json:"payer_authorization_reference"`
	VerificationEvidence        string    `json:"verification_evidence"`
	ResponsibleExecution        bool      `json:"responsible_execution"`
	RouteExceptionReference     string    `json:"route_exception_reference"`
	VerifiedBy                  string    `json:"verified_by"`
	CreatedAt                   time.Time `json:"created_at"`
}

func (RefundDestinationAuthorization) TableName() string {
	return "remediation_refund_destination_authorizations"
}

// RefundDestinationStatus exposes no payer identity, documents or financial
// verification evidence to the customer.
type RefundDestinationStatus struct {
	LegID                 string                     `json:"leg_id"`
	AmountUSD             ledger.Decimal             `json:"amount_usd"`
	SettledUSD            ledger.Decimal             `json:"settled_usd"`
	RemainingUSD          ledger.Decimal             `json:"remaining_usd"`
	OperatorID            string                     `json:"operator_id"`
	SourceOperatorID      string                     `json:"source_operator_id"`
	State                 string                     `json:"state"`
	DestinationRequired   bool                       `json:"destination_required"`
	Proposal              *RefundDestinationProposal `json:"proposal,omitempty"`
	AuthorizedDestination *RefundDestination         `json:"authorized_destination,omitempty"`
	AuthorizationID       string                     `json:"authorization_id,omitempty"`
	AuthorizedAt          *time.Time                 `json:"authorized_at,omitempty"`
	ResponsibleExecution  bool                       `json:"responsible_execution"`
	RouteVersion          int64                      `json:"route_version"`
	DestinationMutable    bool                       `json:"destination_mutable"`
	History               []RefundRouteVersion       `json:"history"`
}

func normalizeRefundDestination(d RefundDestination) (RefundDestination, error) {
	n, e := payments.NormalizeExternalRefundDestination(payments.ExternalRefundDestination{Network: d.Network, Asset: d.Asset, TokenContract: d.TokenContract, Address: d.Address, AuthorizationReference: "validation", VerificationEvidence: "validation"})
	if e != nil {
		return RefundDestination{}, ErrInvalid
	}
	return RefundDestination{Network: n.Network, Asset: n.Asset, TokenContract: n.TokenContract, Address: n.Address}, nil
}

func destinationMutable(c Case, l RefundLeg) bool {
	return (c.State == "pending_review" || c.State == "approved" || c.State == "paying") && (l.State == "reserved" || l.State == "approved") && l.ExecutionKey == ""
}

func (s *Service) ProposeRefundDestination(ctx context.Context, customer, brand, legID, key string, in RefundDestinationProposalInput) (RefundDestinationProposal, error) {
	var out RefundDestinationProposal
	if key == "" || len(key) > 150 || strings.TrimSpace(in.AuthorizationReference) == "" || len(in.AuthorizationReference) > 1000 {
		return out, ErrInvalid
	}
	d, e := normalizeRefundDestination(in.Destination)
	if e != nil {
		return out, e
	}
	in.Destination = d
	e = s.within(ctx, nil, func(t *gorm.DB) error {
		user, e := s.Organization.GetCustomer(ctx, t, customer)
		if e != nil {
			return e
		}
		if user.BrandID != brand {
			return ErrScope
		}
		var leg RefundLeg
		if e = t.First(&leg, "id=?", legID).Error; e != nil {
			return e
		}
		c, l, e := s.lockLeg(ctx, t, leg.OperatorID, legID)
		if e != nil {
			return e
		}
		if c.CustomerID == nil || *c.CustomerID != customer {
			return ErrScope
		}
		fp := fingerprint(in)
		e = t.First(&out, "leg_id=? AND business_key=?", legID, key).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !destinationMutable(c, l) {
			return ErrState
		}
		var count int64
		if e = t.Model(&RefundDestinationAuthorization{}).Where("leg_id=? AND route_version=?", legID, l.RouteVersion).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return ErrState
		}
		if required, e := s.externalDestinationRequired(ctx, t, l); e != nil {
			return e
		} else if !required && !in.ResponsibleExecution {
			return ErrState
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		if l.RouteVersion > 1 && !in.ResponsibleExecution {
			return ErrReview
		}
		out = RefundDestinationProposal{ID: uuid.NewString(), LegID: legID, OperatorID: c.OperatorID, CustomerID: customer, BusinessKey: key, Fingerprint: fp, RefundDestination: d, AuthorizationReference: in.AuthorizationReference, ResponsibleExecution: in.ResponsibleExecution, RouteVersion: l.RouteVersion, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return s.event(t, c.OperatorID, legID, "refund.destination_proposed", customer, in.AuthorizationReference, map[string]any{"proposal_id": out.ID, "destination": d})
	})
	return out, e
}

// AuthorizeRefundDestination is a finance verification of consent from the
// original payer and of the receiving address. For a customer refund it also
// requires that customer's latest immutable destination proposal.
func (s *Service) AuthorizeRefundDestination(ctx context.Context, actor, op, legID, key string, in RefundDestinationAuthorizationInput) (RefundDestinationAuthorization, error) {
	var out RefundDestinationAuthorization
	if key == "" || len(key) > 150 || strings.TrimSpace(in.PayerAuthorizationReference) == "" || len(in.PayerAuthorizationReference) > 1000 || strings.TrimSpace(in.VerificationEvidence) == "" || len(in.VerificationEvidence) > 10000 {
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
		e = t.First(&out, "leg_id=? AND business_key=?", legID, key).Error
		if e == nil {
			if out.BusinessKey != key || out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !destinationMutable(c, l) {
			return ErrState
		}
		var count int64
		if e = t.Model(&RefundDestinationAuthorization{}).Where("leg_id=? AND route_version=?", legID, l.RouteVersion).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return ErrConflict
		}
		if required, e := s.externalDestinationRequired(ctx, t, l); e != nil {
			return e
		} else if !required && !in.ResponsibleExecution {
			return ErrState
		}
		d := in.Destination
		responsible := l.SourceOperatorID != l.OperatorID || in.ResponsibleExecution || l.RouteVersion > 1
		var proposalID *string
		if c.CustomerID != nil {
			var latest RefundDestinationProposal
			if e = t.Where("leg_id=? AND customer_id=? AND route_version=?", legID, *c.CustomerID, l.RouteVersion).Order("created_at DESC,id DESC").First(&latest).Error; e != nil {
				return e
			}
			if latest.ID != in.ProposalID {
				return ErrConflict
			}
			// The staff form cannot replace the customer's endpoint.
			if d != (RefundDestination{}) {
				normalized, e := normalizeRefundDestination(d)
				if e != nil {
					return e
				}
				if normalized != latest.RefundDestination {
					return ErrConflict
				}
			}
			d = latest.RefundDestination
			if responsible && (l.SourceOperatorID == l.OperatorID || l.RouteVersion > 1) && !latest.ResponsibleExecution {
				return ErrReview
			}
			proposalID = &latest.ID
		} else if in.ProposalID != "" {
			return ErrScope
		}
		d, e = normalizeRefundDestination(d)
		if e != nil {
			return e
		}
		if responsible && l.SourceOperatorID == l.OperatorID && strings.TrimSpace(in.RouteExceptionReference) == "" {
			return ErrReview
		}
		if responsible {
			e = s.validateAssumedDestination(ctx, t, l, d)
		} else {
			e = s.validateDestinationSource(ctx, t, l, d)
		}
		if e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = RefundDestinationAuthorization{ID: uuid.NewString(), LegID: l.ID, OperatorID: op, ReceiptID: l.ReceiptID, ProposalID: proposalID, CustomerID: c.CustomerID, AmountUSD: l.AmountUSD.Sub(l.SettledUSD), SettledBeforeUSD: l.SettledUSD, RouteVersion: l.RouteVersion, BusinessKey: key, Fingerprint: fp, RefundDestination: d, PayerAuthorizationReference: in.PayerAuthorizationReference, VerificationEvidence: in.VerificationEvidence, ResponsibleExecution: responsible, RouteExceptionReference: in.RouteExceptionReference, VerifiedBy: actor, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return s.event(t, op, legID, "refund.destination_authorized", actor, in.VerificationEvidence, out)
	})
	return out, e
}

func (s *Service) externalDestinationRequired(ctx context.Context, t *gorm.DB, l RefundLeg) (bool, error) {
	if l.RouteVersion > 1 {
		return true, nil
	}
	var routes int64
	if e := t.Model(&RefundDestinationAuthorization{}).Where("leg_id=? AND responsible_execution=true", l.ID).Count(&routes).Error; e != nil {
		return false, e
	}
	if routes > 0 {
		return true, nil
	}
	r, e := s.Commerce.GetReceipt(ctx, t, l.SourceOperatorID, l.ReceiptID)
	if e != nil {
		return false, e
	}
	return l.SourceOperatorID != l.OperatorID || r.Provider == "evm_wallet" || strings.HasPrefix(r.Provider, "evm_wallet:") || r.Provider == "nowpayments" || strings.HasPrefix(r.Provider, "nowpayments:"), nil
}

func (s *Service) validateDestinationSource(ctx context.Context, t *gorm.DB, l RefundLeg, d RefundDestination) error {
	if l.SourceOperatorID != l.OperatorID {
		return s.validateAssumedDestination(ctx, t, l, d)
	}
	if s.Payments == nil {
		return ErrNotReady
	}
	adapter := *s.Payments
	adapter.DB = t
	source, e := adapter.PaymentSourceForReceipt(ctx, l.SourceOperatorID, l.ReceiptID)
	if e != nil {
		return e
	}
	if source.Driver != "evm_wallet" && source.Driver != "nowpayments" {
		return ErrScope
	}
	m, e := adapter.Merchant(ctx, l.SourceOperatorID, source.MerchantID)
	if e != nil {
		return e
	}
	return payments.ValidateExternalRefundDestinationForSource(m, source, payments.ExternalRefundDestination{Network: d.Network, Asset: d.Asset, TokenContract: d.TokenContract, Address: d.Address, AuthorizationReference: "source-validation", VerificationEvidence: "source-validation"})
}

func (s *Service) requireAuthorizedDestination(ctx context.Context, t *gorm.DB, l RefundLeg) (RefundDestinationAuthorization, error) {
	var out RefundDestinationAuthorization
	if e := t.First(&out, "leg_id=? AND operator_id=? AND route_version=?", l.ID, l.OperatorID, l.RouteVersion).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return out, ErrReview
		}
		return out, e
	}
	if out.ReceiptID != l.ReceiptID || out.SettledBeforeUSD.Add(out.AmountUSD).Cmp(l.AmountUSD) != 0 {
		return out, ErrConflict
	}
	if out.ResponsibleExecution {
		return out, s.validateAssumedDestination(ctx, t, l, out.RefundDestination)
	}
	return out, s.validateDestinationSource(ctx, t, l, out.RefundDestination)
}

func (s *Service) destinationStatus(ctx context.Context, t *gorm.DB, l RefundLeg) (RefundDestinationStatus, error) {
	required, e := s.externalDestinationRequired(ctx, t, l)
	if e != nil {
		return RefundDestinationStatus{}, e
	}
	var c Case
	if e = t.First(&c, "id=?", l.CaseID).Error; e != nil {
		return RefundDestinationStatus{}, e
	}
	out := RefundDestinationStatus{LegID: l.ID, AmountUSD: l.AmountUSD, SettledUSD: l.SettledUSD, RemainingUSD: l.AmountUSD.Sub(l.SettledUSD), OperatorID: l.OperatorID, SourceOperatorID: l.SourceOperatorID, State: l.State, DestinationRequired: required, ResponsibleExecution: l.SourceOperatorID != l.OperatorID || l.RouteVersion > 1, RouteVersion: l.RouteVersion, DestinationMutable: destinationMutable(c, l)}
	var proposal RefundDestinationProposal
	e = t.Where("leg_id=? AND route_version=?", l.ID, l.RouteVersion).Order("created_at DESC,id DESC").First(&proposal).Error
	if e == nil {
		out.Proposal = &proposal
		if proposal.ResponsibleExecution {
			out.ResponsibleExecution = true
		}
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	var authorization RefundDestinationAuthorization
	e = t.First(&authorization, "leg_id=? AND route_version=?", l.ID, l.RouteVersion).Error
	if e == nil {
		out.AuthorizedDestination = &authorization.RefundDestination
		out.AuthorizationID = authorization.ID
		out.AuthorizedAt = &authorization.CreatedAt
		out.ResponsibleExecution = authorization.ResponsibleExecution
		out.DestinationMutable = false
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	out.History, e = s.refundRouteHistory(t, l)
	if e != nil {
		return out, e
	}
	return out, nil
}

func (s *Service) RefundDestinationStatusForStaff(ctx context.Context, actor, op, legID string) (RefundDestinationStatus, error) {
	var out RefundDestinationStatus
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		var leg RefundLeg
		if e := t.First(&leg, "id=? AND operator_id=?", legID, op).Error; e != nil {
			return e
		}
		var e error
		out, e = s.destinationStatus(ctx, t, leg)
		return e
	})
	return out, e
}

func (s *Service) CustomerRefundDestinations(ctx context.Context, customer, brand, caseID string) ([]RefundDestinationStatus, error) {
	out := []RefundDestinationStatus{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		user, e := s.Organization.GetCustomer(ctx, t, customer)
		if e != nil {
			return e
		}
		if user.BrandID != brand {
			return ErrScope
		}
		var c Case
		if e = t.First(&c, "id=? AND customer_id=?", caseID, customer).Error; e != nil {
			return e
		}
		legs, e := s.Legs(ctx, t, c.OperatorID, c.ID)
		if e != nil {
			return e
		}
		for _, leg := range legs {
			status, e := s.destinationStatus(ctx, t, leg)
			if e != nil {
				return e
			}
			out = append(out, status)
		}
		return nil
	})
	return out, e
}

// RequireExternalRefundFact matches an already executed external transfer to
// its durable instruction. It deliberately does not reapply NEW-payment risk
// checks: a later dispute cannot erase a real transfer from the evidence log.
func (s *Service) RequireExternalRefundFact(ctx context.Context, t *gorm.DB, actor string, r payments.Refund, d payments.ExternalRefundDestination) error {
	if t == nil || s.Payments == nil || s.Organization == nil {
		return ErrNotReady
	}
	op := r.Order.OperatorID
	if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	_, l, e := s.lockLeg(ctx, t, op, r.ID)
	if e != nil {
		return e
	}
	return s.matchExternalRefundFact(ctx, t, l, r, d)
}

func (s *Service) matchExternalRefundFact(ctx context.Context, t *gorm.DB, l RefundLeg, r payments.Refund, d payments.ExternalRefundDestination) error {
	if l.SourceOperatorID != r.Order.OperatorID || r.USDAmount.Cmp(l.AmountUSD) != 0 {
		return ErrState
	}
	if l.State != "executing" && l.State != "unknown" && l.State != "completed" && l.State != "exception" && !(l.RouteVersion > 1 && l.State == "approved") {
		return ErrState
	}
	if s.Payments == nil {
		return ErrNotReady
	}
	adapter := *s.Payments
	adapter.DB = t
	source, e := adapter.PaymentSourceForReceipt(ctx, l.SourceOperatorID, l.ReceiptID)
	if e != nil {
		return e
	}
	if source.MerchantID != r.Order.MerchantID || source.OriginalOrder.ID != r.Order.ID {
		return ErrScope
	}
	a, e := s.destinationForProof(t, l, d.AuthorizationReference)
	if e != nil {
		return e
	}
	if a.RouteVersion == l.RouteVersion && (l.ExecutionMode != "manual" || l.ExecutionKey == "") {
		return ErrState
	}
	n, e := normalizeRefundDestination(RefundDestination{Network: d.Network, Asset: d.Asset, TokenContract: d.TokenContract, Address: d.Address})
	if e != nil {
		return e
	}
	if d.AuthorizationReference != a.ID || n != a.RefundDestination {
		return ErrConflict
	}
	return nil
}

func (s *Service) validateExternalRefundProof(ctx context.Context, t *gorm.DB, l RefundLeg, x payments.ExternalRefundExecution) error {
	a, e := s.destinationForProof(t, l, x.Destination.AuthorizationReference)
	if e != nil {
		return e
	}
	n, e := normalizeRefundDestination(RefundDestination{Network: x.Destination.Network, Asset: x.Destination.Asset, TokenContract: x.Destination.TokenContract, Address: x.Destination.Address})
	if e != nil {
		return e
	}
	if x.Destination.AuthorizationReference != a.ID || n != a.RefundDestination || x.VerificationMethod == "" || x.VerifiedBy == "" || x.RecipientAddress == "" {
		return ErrConflict
	}
	return nil
}

func externalRefundMisdirected(x payments.ExternalRefundExecution) bool {
	actual, e := normalizeRefundDestination(RefundDestination{Network: x.Destination.Network, Asset: x.Destination.Asset, TokenContract: x.Destination.TokenContract, Address: x.RecipientAddress})
	approved, e2 := normalizeRefundDestination(RefundDestination{Network: x.Destination.Network, Asset: x.Destination.Asset, TokenContract: x.Destination.TokenContract, Address: x.Destination.Address})
	return e != nil || e2 != nil || actual.Address != approved.Address
}
