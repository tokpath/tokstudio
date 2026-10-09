package payment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPurchaseConflict = errors.New("purchase operation parameters changed")

type SubscriptionPurchaseInput struct {
	UserID, ChannelID, BrandOwnerID, PlanID, Adapter, MethodRef, OperationID string
	AutoRenew                                                                bool
}
type subscriptionPurchaseRow struct {
	ID             string  `gorm:"column:id;primaryKey"`
	UserID         string  `gorm:"column:user_id"`
	RequestHash    string  `gorm:"column:request_hash"`
	SubscriptionID *string `gorm:"column:subscription_id"`
	OrderID        *string `gorm:"column:order_id"`
}

func (subscriptionPurchaseRow) TableName() string { return "payment_subscription_purchases" }

// Subscription creation, the payment order and their durable operation identity
// commit together. Provider checkout can be retried using this original order.
func (s *Service) CreateSubscriptionPurchase(ctx context.Context, in SubscriptionPurchaseInput) (*plans.SubscriptionView, *OrderView, error) {
	if in.UserID == "" || strings.TrimSpace(in.OperationID) == "" || len(in.OperationID) > 128 {
		return nil, nil, ErrInvalidAmount
	}
	identity := crypto.HashToken("subscription:" + in.UserID + ":" + in.OperationID)
	payload := in
	payload.OperationID = ""
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	hash := crypto.HashToken(string(raw))
	var subscription *plans.SubscriptionView
	var order *OrderView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := subscriptionPurchaseRow{ID: identity, UserID: in.UserID, RequestHash: hash}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", identity).First(&row).Error; err != nil {
			return err
		}
		if row.RequestHash != hash || row.UserID != in.UserID {
			return ErrPurchaseConflict
		}
		if row.OrderID != nil && row.SubscriptionID != nil {
			var err error
			subscription, err = s.plans.GetSubscription(ctx, *row.SubscriptionID, in.UserID)
			if err != nil {
				return err
			}
			order, err = s.GetOrder(ctx, *row.OrderID, in.UserID)
			return err
		}
		// Current checkout has no verifiable future-payment authorization flow.
		// Retain prior purchase snapshots, but never create a new off-session mandate.
		if in.AutoRenew || strings.TrimSpace(in.MethodRef) != "" {
			return ErrNoAutoRenew
		}
		var err error
		subscription, err = s.plans.CreateSubscriptionTx(tx, in.UserID, in.ChannelID, in.PlanID, in.Adapter, in.MethodRef, in.BrandOwnerID, in.AutoRenew)
		if err != nil {
			return err
		}
		plan, err := s.plans.GetPlan(ctx, in.PlanID)
		if err != nil {
			return err
		}
		scoped := *s
		scoped.db = tx
		order, err = scoped.CreateOrder(ctx, CreateOrderInput{UserID: in.UserID, ChannelOrgID: in.ChannelID, Adapter: in.Adapter, Purpose: PurposeSubscription, ReferenceType: PurposeSubscription, ReferenceID: subscription.ID, AmountMinor: plan.PriceMinor, Currency: plan.Currency})
		if err != nil {
			return err
		}
		return tx.Model(&subscriptionPurchaseRow{}).Where("id = ?", identity).Updates(map[string]any{"subscription_id": subscription.ID, "order_id": order.ID}).Error
	})
	return subscription, order, err
}

func (s *Service) GetSubscriptionPurchase(ctx context.Context, userID, operationID string) (*OrderView, error) {
	var row subscriptionPurchaseRow
	identity := crypto.HashToken("subscription:" + userID + ":" + operationID)
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", identity, userID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if row.OrderID == nil {
		return nil, ErrNotFound
	}
	return s.GetOrder(ctx, *row.OrderID, userID)
}
