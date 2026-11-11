package payments

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"time"
)

type MerchantInput struct {
	Merchant
	Credentials     map[string]string `json:"credentials"`
	ExpectedVersion uint64            `json:"expected_version"`
}

func (in MerchantInput) Configuration() Merchant {
	m := in.Merchant
	m.Credentials = in.Credentials
	return m
}

type PublicMethod struct {
	Kind                           string         `json:"method_kind"`
	FeeRuleID                      string         `json:"fee_rule_id"`
	Name                           string         `json:"name"`
	FlatFeeMoney                   ledger.Decimal `json:"flat_fee_money"`
	FeeRate                        ledger.Decimal `json:"fee_rate"`
	ID                             string         `json:"id"`
	SettlementCurrency             string         `json:"settlement_currency"`
	SettlementSupported            bool           `json:"settlement_supported"`
	UnsupportedReason              string         `json:"unsupported_reason,omitempty"`
	RequiresSettlementConfirmation bool           `json:"requires_settlement_confirmation"`
	Driver                         string         `json:"driver"`
	Currency                       string         `json:"currency"`
	Instructions                   string         `json:"instructions,omitempty"`
	Configured                     bool           `json:"configured"`
	RefundAPI                      bool           `json:"refund_api"`
	AutoCharge                     bool           `json:"auto_charge"`
}

func (s *Service) ListMerchantConfigs(ctx context.Context, actor, operator string) ([]MerchantView, error) {
	if s.Authorize == nil {
		return nil, ErrForbidden
	}
	if e := s.Authorize.RequirePermission(ctx, s.DB, actor, operator, "payments.manage"); e != nil {
		return nil, e
	}
	var rows []merchantRow
	if e := s.DB.WithContext(ctx).Table("payments_merchants").Where("operator_id=?", operator).Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []MerchantView{}
	for _, row := range rows {
		if s.Drivers[row.Driver] == nil {
			continue
		}
		m, e := s.openMerchant(row)
		if e != nil {
			return nil, e
		}
		d := s.Drivers[m.Driver]
		if d == nil {
			continue
		}
		view := MerchantView{Merchant: m, MissingFields: d.Validate(m), FieldsSet: []string{}}
		for k, v := range m.Credentials {
			if v != "" {
				view.FieldsSet = append(view.FieldsSet, k)
			}
		}
		out = append(out, view)
	}
	return out, nil
}
func (s *Service) PublicMethods(ctx context.Context, operator string) ([]PublicMethod, error) {
	settings, e := ledger.GetMonetary(ctx, s.DB)
	if e != nil {
		return nil, e
	}
	var rows []merchantRow
	if e := s.DB.WithContext(ctx).Table("payments_merchants").Where("operator_id=? AND enabled=true", operator).Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []PublicMethod{}
	for _, row := range rows {
		if s.Drivers[row.Driver] == nil {
			continue
		}
		m, e := s.openMerchant(row)
		if e != nil {
			return nil, e
		}
		d := s.Drivers[m.Driver]
		if d == nil || ready(m, d) != nil {
			continue
		}
		currency := merchantCurrency(m)
		supported, reason := SettlementSupport(m, settings.Currency)
		_, fiatPayment := fiatMinorDigits(currency)
		if currency != settings.Currency && fiatPayment && m.Driver != "kvoucher" {
			fx, err := s.FXSettings(ctx)
			if err != nil {
				return nil, err
			}
			if !fx.Enabled {
				supported = false
				reason = "请等待平台启用外币汇率报价来源。"
			}
		}
		if settings.Status != "locked" {
			supported = false
			reason = "系统清算设置尚未正式生效，暂不可付款。"
		}
		view := PublicMethod{ID: m.ID, Driver: m.Driver, Currency: currency, Configured: true, SettlementCurrency: settings.Currency, SettlementSupported: supported, UnsupportedReason: reason, RequiresSettlementConfirmation: m.Driver == "manual" && currency != settings.Currency && !fiatPayment, RefundAPI: m.Driver == "stripe" || m.Driver == "alipay" || m.Driver == "wechat", AutoCharge: m.Driver == "stripe"}
		if m.Driver == "manual" {
			view.Instructions = m.Credentials["instructions"]
		}
		rule, err := paymentmethods.Current(ctx, s.DB, operator, methodIdentity(m))
		if err != nil {
			return nil, err
		}
		if rule.Paused || !rule.Collection {
			continue
		}
		if rule.Kind != "" {
			var identity paymentmethods.Identity
			if err = s.DB.WithContext(ctx).Where("operator_id=? AND method_id=?", operator, rule.MethodID).First(&identity).Error; err != nil {
				return nil, err
			}
			if identity.MerchantID != m.ID {
				continue
			}
		}
		if rule.Kind == "offline" {
			view.Instructions = "请联系经营方，按约定完成线下付款。"
		}
		view.Kind = rule.Kind
		view.FeeRuleID = rule.ID
		view.Name = rule.Name
		view.FlatFeeMoney = rule.FlatFeeMoney
		view.FeeRate = rule.FeeRate
		out = append(out, view)
	}
	return out, nil
}

type IntentView struct {
	Problem    string    `json:"problem,omitempty"`
	Driver     string    `json:"driver"`
	OrderID    string    `json:"order_id"`
	MerchantID string    `json:"merchant_id"`
	ProviderID string    `json:"provider_id,omitempty"`
	State      string    `json:"state"`
	Checkout   *Checkout `json:"checkout,omitempty"`
}

func (s *Service) PaymentIntent(ctx context.Context, operator, merchant, order string) (IntentView, error) {
	var row intentRow
	if e := s.DB.WithContext(ctx).Table("payments_intents").Where("operator_id=? AND merchant_id=? AND order_id=?", operator, merchant, order).Take(&row).Error; e != nil {
		return IntentView{}, e
	}
	view := IntentView{OrderID: row.OrderID, MerchantID: row.MerchantID, ProviderID: row.ProviderID, State: row.State}
	var snapshot Order
	if e := s.open(row.OrderCiphertext, &snapshot); e != nil {
		return view, e
	}
	view.Driver = snapshot.PaymentDriver
	_, supportedCurrency := fiatMinorDigits(snapshot.Money.Currency)
	if s.Drivers[view.Driver] == nil || !supportedCurrency {
		return view, nil
	}
	if (row.State == "created" || row.State == "outcome_unknown" || row.State == "failed") && row.CheckoutCiphertext != "" {
		var checkout Checkout
		if e := s.open(row.CheckoutCiphertext, &checkout); e != nil {
			return view, e
		}
		if checkout.Problem == CheckoutAmountMismatch {
			view.Problem = checkout.Problem
		}
		if row.State == "created" {
			view.Checkout = &checkout
		}
	}
	return view, nil
}

// PaymentSource is an immutable origin for original-route refunds. Checkout
// secrets, saved payment methods and customer tokens are not projected.
type PaymentSource struct {
	MerchantID    string `json:"merchant_id"`
	Driver        string `json:"driver"`
	ReceiptID     string `json:"receipt_id"`
	FactID        string `json:"fact_id"`
	OriginalOrder Order  `json:"original_order"`
	Fact          Fact   `json:"fact"`
}

type RefundExecutionView struct {
	ExecutionID      string    `json:"execution_id"`
	BusinessRefundID string    `json:"business_refund_id"`
	State            string    `json:"state"`
	ProviderID       string    `json:"provider_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RefundExecutions is a trusted exact merchant/obligation query for recovery.
// A supplementary real transfer has a separate execution, never a changed
// original mutation or an implicit permission to send again.
func (s *Service) RefundExecutions(ctx context.Context, operator, merchant, businessID string) ([]RefundExecutionView, error) {
	out := []RefundExecutionView{}
	e := s.DB.WithContext(ctx).Table("payments_refund_intents").Where("operator_id=? AND merchant_id=? AND business_refund_id=?", operator, merchant, businessID).Select("refund_id AS execution_id,business_refund_id,state,COALESCE(provider_id,'') AS provider_id,created_at").Order("created_at,refund_id").Limit(1000).Scan(&out).Error
	return out, e
}

func (s *Service) PaymentSourceForReceipt(ctx context.Context, operator, receiptID string) (PaymentSource, error) {
	var row struct {
		MerchantID, OrderID, ID string
		Fact                    []byte
	}
	out := PaymentSource{ReceiptID: receiptID}
	if e := s.DB.WithContext(ctx).Table("payments_verified_facts").Select("id,merchant_id,order_id,fact").Where("operator_id=? AND receipt_id=? AND kind='receipt'", operator, receiptID).Take(&row).Error; e != nil {
		return out, e
	}
	m, e := s.Merchant(ctx, operator, row.MerchantID)
	if e != nil {
		return out, e
	}
	o, e := s.loadOrder(ctx, m, row.OrderID)
	if e != nil {
		return out, e
	}
	o.PaymentMethod = ""
	o.CustomerRef = ""
	o.AuthorizationID = ""
	out.MerchantID = m.ID
	out.Driver = m.Driver
	out.FactID = row.ID
	out.OriginalOrder = o
	if e = json.Unmarshal(row.Fact, &out.Fact); e != nil {
		return out, e
	}
	out.Fact.proof = true
	return out, nil
}

type ReceiptLink struct {
	MerchantID string `json:"merchant_id"`
	OrderID    string `json:"order_id"`
	ReceiptID  string `json:"receipt_id"`
	FactID     string `json:"fact_id"`
}

func (s *Service) ReceiptLinks(ctx context.Context, operator string, limit int) ([]ReceiptLink, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	out := []ReceiptLink{}
	e := s.DB.WithContext(ctx).Table("payments_verified_facts").Select("merchant_id,order_id,receipt_id,id AS fact_id").Where("operator_id=? AND kind='receipt' AND receipt_id<>''", operator).Order("observed_at,id").Limit(limit).Scan(&out).Error
	return out, e
}

func (s *Service) ReceiptsForOrder(ctx context.Context, operator, merchant, order string) ([]ReceiptLink, error) {
	out := []ReceiptLink{}
	e := s.DB.WithContext(ctx).Table("payments_verified_facts").Select("merchant_id,order_id,receipt_id,id AS fact_id").Where("operator_id=? AND merchant_id=? AND order_id=? AND kind='receipt' AND receipt_id<>''", operator, merchant, order).Order("observed_at,id").Find(&out).Error
	return out, e
}
