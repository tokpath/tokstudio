package payment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type walletPurchaseRow struct {
	ID          string  `gorm:"column:id;primaryKey"`
	UserID      string  `gorm:"column:user_id"`
	RequestHash string  `gorm:"column:request_hash"`
	OrderID     *string `gorm:"column:order_id"`
}

func (walletPurchaseRow) TableName() string { return "payment_wallet_purchases" }

// The operation, topup and order commit together before provider checkout.
func (s *Service) CreateWalletPurchase(ctx context.Context, in CreateOrderInput, operationID string) (*OrderView, error) {
	if in.UserID == "" || strings.TrimSpace(operationID) == "" || len(operationID) > 128 || in.Purpose != PurposeWallet || in.PayMajor <= 0 {
		return nil, ErrInvalidAmount
	}
	id := crypto.HashToken("wallet:" + in.UserID + ":" + operationID)
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	hash := crypto.HashToken(string(raw))
	var order *OrderView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := walletPurchaseRow{ID: id, UserID: in.UserID, RequestHash: hash}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&row).Error; err != nil {
			return err
		}
		if row.RequestHash != hash || row.UserID != in.UserID {
			return ErrPurchaseConflict
		}
		if row.OrderID != nil {
			var err error
			order, err = s.GetOrder(ctx, *row.OrderID, in.UserID)
			return err
		}
		scoped := *s
		scoped.db = tx
		var err error
		order, err = scoped.CreateOrder(ctx, in)
		if err != nil {
			return err
		}
		return tx.Model(&walletPurchaseRow{}).Where("id = ?", id).Update("order_id", order.ID).Error
	})
	return order, err
}
func (s *Service) GetWalletPurchase(ctx context.Context, userID, operationID string) (*OrderView, error) {
	var row walletPurchaseRow
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", crypto.HashToken("wallet:"+userID+":"+operationID), userID).First(&row).Error; err != nil {
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
