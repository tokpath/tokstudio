package payment

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type orderRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	UserID          string    `gorm:"column:user_id"`
	Adapter         string    `gorm:"column:adapter"`
	Purpose         string    `gorm:"column:purpose"`
	ReferenceType   *string   `gorm:"column:reference_type"`
	ReferenceID     *string   `gorm:"column:reference_id"`
	AmountMinor     int64     `gorm:"column:amount_minor"`
	Currency        string    `gorm:"column:currency"`
	Status          string    `gorm:"column:status"`
	ProviderTradeID *string   `gorm:"column:provider_trade_id"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (orderRow) TableName() string { return "payment_orders" }

type eventRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	Adapter         string    `gorm:"column:adapter"`
	ExternalEventID string    `gorm:"column:external_event_id"`
	OrderID         *string   `gorm:"column:order_id"`
	SignatureValid  bool      `gorm:"column:signature_valid"`
	PayloadJSON     []byte    `gorm:"column:payload_json"`
	ProcessedAt     time.Time `gorm:"column:processed_at"`
}

func (eventRow) TableName() string { return "payment_events" }

type Service struct {
	db      *gorm.DB
	outbox  *outbox.Service
	plans   *plans.Service
	billing *billing.Service
	signKey string
}

func New(db *gorm.DB, publisher *outbox.Service, plansSvc *plans.Service, billingSvc *billing.Service, signKey string) *Service {
	return &Service{db: db, outbox: publisher, plans: plansSvc, billing: billingSvc, signKey: signKey}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "payment", sub
}

func (s *Service) SignKey() string { return s.signKey }

func (s *Service) CreateOrder(ctx context.Context, in CreateOrderInput) (*OrderView, error) {
	if !ValidAdapter(in.Adapter) {
		return nil, ErrInvalidAdapter
	}
	if in.AmountMinor <= 0 || in.UserID == "" {
		return nil, ErrInvalidAmount
	}
	if in.Purpose == "" {
		in.Purpose = PurposeWallet
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.ReferenceType == PurposeSubscription && in.ReferenceID != "" {
		var existing orderRow
		if err := s.db.WithContext(ctx).Where("reference_id = ? AND purpose = ? AND status = ?", in.ReferenceID, PurposeSubscription, StatusPending).
			Order("created_at DESC").First(&existing).Error; err == nil {
			return orderView(existing), nil
		}
	}
	now := time.Now().UTC()
	row := orderRow{
		ID: id.New("pay"), UserID: in.UserID, Adapter: strings.ToLower(in.Adapter),
		Purpose: in.Purpose, AmountMinor: in.AmountMinor, Currency: in.Currency,
		Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if in.ReferenceType != "" {
		row.ReferenceType = &in.ReferenceType
	}
	if in.ReferenceID != "" {
		row.ReferenceID = &in.ReferenceID
	}
	if in.Purpose == PurposeWallet && s.billing != nil {
		top, err := s.billing.CreateTopup(ctx, in.UserID, "", in.AmountMinor, in.Adapter)
		if err != nil {
			return nil, err
		}
		refType := "topup"
		row.ReferenceType = &refType
		row.ReferenceID = &top.ID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return orderView(row), nil
}

func (s *Service) ListOrders(ctx context.Context, status string) ([]OrderView, error) {
	var rows []orderRow
	q := s.db.WithContext(ctx).Order("created_at DESC").Limit(200)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OrderView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *orderView(row))
	}
	return out, nil
}

func (s *Service) GetOrder(ctx context.Context, id, userID string) (*OrderView, error) {
	var row orderRow
	q := s.db.WithContext(ctx).Where("id = ?", id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return orderView(row), nil
}

func (s *Service) Checkout(order *OrderView, publicBase string) *CheckoutView {
	base := strings.TrimRight(publicBase, "/")
	return &CheckoutView{
		Order:      order,
		Adapter:    order.Adapter,
		Sandbox:    true,
		WebhookURL: base + "/v1/payments/" + order.Adapter + "/webhook",
		AutoRenew:  SupportsAutoRenew(order.Adapter),
	}
}

func (s *Service) HandleWebhook(ctx context.Context, adapter, signature string, body []byte) (*EventView, error) {
	if !ValidAdapter(adapter) {
		return nil, ErrInvalidAdapter
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, ErrInvalidEvent
	}
	eventID := asString(payload["event_id"])
	orderID := asString(payload["order_id"])
	status := asString(payload["status"])
	tradeID := asString(payload["trade_id"])
	if eventID == "" || status == "" {
		return nil, ErrInvalidEvent
	}
	valid := VerifyWebhook(s.signKey, eventID, orderID, status, signature)
	now := time.Now().UTC()
	view := &EventView{Adapter: adapter, ExternalEventID: eventID, OrderID: orderID, SignatureValid: valid}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing eventRow
		if err := tx.Where("external_event_id = ?", eventID).First(&existing).Error; err == nil {
			view.ID = existing.ID
			view.SignatureValid = existing.SignatureValid
			view.Duplicate = true
			if !existing.SignatureValid {
				return ErrInvalidSignature
			}
			return nil
		}
		row := eventRow{
			ID: id.New("pev"), Adapter: adapter, ExternalEventID: eventID,
			SignatureValid: valid, PayloadJSON: body, ProcessedAt: now,
		}
		if orderID != "" {
			row.OrderID = &orderID
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		view.ID = row.ID
		if !valid {
			return ErrInvalidSignature
		}
		if _, err := s.outbox.EnqueueTx(tx, "payment.webhook.received", "payment_event", row.ID, map[string]any{
			"adapter": adapter, "external_event_id": eventID, "status": status,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return view, err
	}
	if view.Duplicate {
		return view, nil
	}
	view.Status = status
	if orderID == "" {
		return view, nil
	}
	switch status {
	case StatusPaid:
		if err := s.markPaid(ctx, orderID, tradeID); err != nil {
			return view, err
		}
	case StatusFailed:
		_ = s.db.WithContext(ctx).Model(&orderRow{}).Where("id = ? AND status = ?", orderID, StatusPending).
			Updates(map[string]any{"status": StatusFailed, "updated_at": now})
	case StatusRefunded:
		if err := s.markRefunded(ctx, orderID); err != nil {
			return view, err
		}
	}
	return view, nil
}

func (s *Service) ConfirmManual(ctx context.Context, orderID string) (*OrderView, error) {
	var row orderRow
	if err := s.db.WithContext(ctx).Where("id = ?", orderID).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	if row.Adapter != AdapterManual && row.Adapter != AdapterStripe && row.Adapter != AdapterAlipay && row.Adapter != AdapterWechat {
		return nil, ErrInvalidAdapter
	}
	if err := s.markPaid(ctx, orderID, "manual:"+orderID); err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, orderID, "")
}

func (s *Service) Refund(ctx context.Context, orderID string) (*OrderView, error) {
	if err := s.markRefunded(ctx, orderID); err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, orderID, "")
}

func (s *Service) markPaid(ctx context.Context, orderID, tradeID string) error {
	var row orderRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&row).Error; err != nil {
			return ErrNotFound
		}
		if row.Status == StatusPaid {
			return nil
		}
		if row.Status != StatusPending {
			return ErrOrderNotPending
		}
		now := time.Now().UTC()
		row.Status = StatusPaid
		row.UpdatedAt = now
		if tradeID != "" {
			row.ProviderTradeID = &tradeID
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		return err
	}
	return s.fulfill(ctx, row)
}

func (s *Service) fulfill(ctx context.Context, row orderRow) error {
	switch row.Purpose {
	case PurposeSubscription:
		if row.ReferenceID == nil || s.plans == nil {
			return nil
		}
		_, err := s.plans.ActivatePaid(ctx, *row.ReferenceID, time.Now().UTC())
		return err
	case PurposeWallet:
		if row.ReferenceID == nil || s.billing == nil {
			return nil
		}
		_, err := s.billing.ConfirmTopup(ctx, *row.ReferenceID, "payment:"+row.ID)
		return err
	case PurposeRenewal:
		return nil
	}
	return nil
}

func (s *Service) markRefunded(ctx context.Context, orderID string) error {
	var row orderRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&row).Error; err != nil {
			return ErrNotFound
		}
		if row.Status == StatusRefunded {
			return nil
		}
		if row.Status != StatusPaid {
			return ErrOrderNotPending
		}
		row.Status = StatusRefunded
		row.UpdatedAt = time.Now().UTC()
		return tx.Save(&row).Error
	})
	if err != nil {
		return err
	}
	switch row.Purpose {
	case PurposeSubscription:
		if row.ReferenceID != nil && s.plans != nil {
			return s.plans.ReverseSource(ctx, *row.ReferenceID)
		}
	case PurposeWallet:
		if row.ReferenceID != nil && s.billing != nil {
			_, err := s.billing.RefundTopup(ctx, *row.ReferenceID)
			return err
		}
	}
	return nil
}

// RenewCharger 给套餐续费 Worker 用：Stripe 沙箱可扣款；method_ref 含 fail 则模拟拒付。
func (s *Service) RenewCharger() func(subID, adapter, methodRef string) error {
	return func(subID, adapter, methodRef string) error {
		return s.ChargeRenewal(context.Background(), subID, adapter, methodRef)
	}
}

func (s *Service) ChargeRenewal(ctx context.Context, subID, adapter, methodRef string) error {
	if !SupportsAutoRenew(adapter) {
		return ErrNoAutoRenew
	}
	if strings.Contains(strings.ToLower(methodRef), "fail") {
		return ErrChargeFailed
	}
	sub, err := s.plans.GetSubscription(ctx, subID, "")
	if err != nil {
		return err
	}
	plan, err := s.plans.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	refType := PurposeSubscription
	row := orderRow{
		ID: id.New("pay"), UserID: sub.UserID, Adapter: adapter, Purpose: PurposeRenewal,
		ReferenceType: &refType, ReferenceID: &subID, AmountMinor: plan.PriceMinor, Currency: plan.Currency,
		Status: StatusPaid, CreatedAt: now, UpdatedAt: now,
	}
	trade := "sandbox-renew:" + subID
	row.ProviderTradeID = &trade
	return s.db.WithContext(ctx).Create(&row).Error
}

func orderView(row orderRow) *OrderView {
	view := &OrderView{
		ID: row.ID, UserID: row.UserID, Adapter: row.Adapter, Purpose: row.Purpose,
		AmountMinor: row.AmountMinor, Currency: row.Currency, Status: row.Status, CreatedAt: row.CreatedAt,
	}
	if row.ReferenceType != nil {
		view.ReferenceType = *row.ReferenceType
	}
	if row.ReferenceID != nil {
		view.ReferenceID = *row.ReferenceID
	}
	if row.ProviderTradeID != nil {
		view.TradeID = *row.ProviderTradeID
	}
	return view
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
