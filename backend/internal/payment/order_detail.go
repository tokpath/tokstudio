package payment

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type OrderFinancialFacts struct {
	Order              *OrderView        `json:"order"`
	Allocations        []OrderAllocation `json:"allocations"`
	SubscriptionStatus string            `json:"subscription_status,omitempty"`
	Events             []OrderEvent      `json:"events"`
}
type OrderAllocation struct {
	ID               string `json:"id"`
	PoolChannelOrgID string `json:"pool_channel_org_id"`
	ChannelOrgID     string `json:"channel_org_id"`
	GrantedMinor     int64  `json:"granted_minor"`
	ConsumedMinor    int64  `json:"consumed_minor"`
	Status           string `json:"status"`
}
type OrderEvent struct {
	ID             string    `json:"id"`
	Adapter        string    `json:"adapter"`
	SignatureValid bool      `json:"signature_valid"`
	ProcessedAt    time.Time `json:"processed_at"`
}

func (s *Service) OrderFacts(ctx context.Context, orderID, ownerID string) (*OrderFinancialFacts, error) {
	order, err := s.GetOrder(ctx, orderID, "")
	if err != nil {
		return nil, err
	}
	if order.PayeeChannelOrgID != ownerID {
		return nil, ErrNotFound
	}
	view := &OrderFinancialFacts{Order: order, Allocations: []OrderAllocation{}, Events: []OrderEvent{}}
	if order.ReferenceType == "topup" && order.ReferenceID != "" {
		if err := s.db.WithContext(ctx).Table("billing_quota_allocations").Where("source_type = ? AND source_id = ?", "topup", order.ReferenceID).Order("created_at, id").Scan(&view.Allocations).Error; err != nil {
			return nil, err
		}
	}
	if order.ReferenceType == PurposeSubscription && order.ReferenceID != "" {
		var sub struct{ Status string }
		if err := s.db.WithContext(ctx).Table("plans_subscriptions").Where("id = ?", order.ReferenceID).Take(&sub).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		view.SubscriptionStatus = sub.Status
	}
	if err := s.db.WithContext(ctx).Table("payment_events").Select("id,adapter,signature_valid,processed_at").Where("order_id = ?", orderID).Order("processed_at,id").Scan(&view.Events).Error; err != nil {
		return nil, err
	}
	return view, nil
}

type RefundPreview struct {
	Order              *OrderView `json:"order"`
	CreditReclaimMinor int64      `json:"credit_reclaim_minor"`
	Subscription       bool       `json:"subscription"`
	CanRefund          bool       `json:"can_refund"`
	BlockedReason      string     `json:"blocked_reason,omitempty"`
}

// Exercise the existing local reversal inside a transaction that always rolls back.
// This validates the actual wallet/allocation/entitlement constraints without moving cash.
func (s *Service) PreviewRefund(ctx context.Context, orderID, ownerID string) (*RefundPreview, error) {
	order, err := s.GetOrder(ctx, orderID, "")
	if err != nil {
		return nil, err
	}
	if order.PayeeChannelOrgID != ownerID {
		return nil, ErrNotFound
	}
	view := &RefundPreview{Order: order, Subscription: order.Purpose == PurposeSubscription}
	if order.Status != StatusPaid {
		view.BlockedReason = "order_status"
		return view, nil
	}
	rollback := errors.New("refund preview rollback")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != StatusPaid {
			return ErrOrderNotPending
		}
		switch row.Purpose {
		case PurposeWallet:
			view.CreditReclaimMinor = row.CreditMinor
			if row.ReferenceID != nil && s.billing != nil {
				if _, err := s.billing.RefundTopupTx(tx, *row.ReferenceID); err != nil {
					return err
				}
			}
		case PurposeSubscription:
			if row.ReferenceID != nil && s.plans != nil {
				if err := s.plans.ReverseSourceTx(tx, *row.ReferenceID); err != nil {
					return err
				}
			}
		}
		view.CanRefund = true
		return rollback
	})
	if errors.Is(err, rollback) {
		return view, nil
	}
	switch {
	case errors.Is(err, billing.ErrInsufficientBalance), errors.Is(err, billing.ErrInsufficientQuota):
		view.BlockedReason = "credit_unavailable"
		return view, nil
	case errors.Is(err, billing.ErrTopupNotPending), errors.Is(err, ErrOrderNotPending):
		view.BlockedReason = "order_status"
		return view, nil
	}
	return nil, err
}
