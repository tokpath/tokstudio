package payments

import (
	"context"
	"encoding/json"
	"strings"
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
	ID             string   `json:"id"`
	Driver         string   `json:"driver"`
	Currency       string   `json:"currency"`
	Network        string   `json:"network,omitempty"`
	TokenContract  string   `json:"token_contract,omitempty"`
	Instructions   string   `json:"instructions,omitempty"`
	Configured     bool     `json:"configured"`
	RefundAPI      bool     `json:"refund_api"`
	AutoCharge     bool     `json:"auto_charge"`
	RefundChannels []string `json:"refund_channels,omitempty"`
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
		m, e := s.openMerchant(row)
		if e != nil {
			return nil, e
		}
		d := s.Drivers[m.Driver]
		if d == nil {
			return nil, ErrUnsupported
		}
		view := MerchantView{Merchant: m, MissingFields: d.Validate(m), FieldsSet: []string{}}
		if e := s.applyAssetNetwork(ctx, s.DB, &m, true); e != nil {
			view.MissingFields = append(view.MissingFields, "active_platform_asset_version")
		}
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
	var rows []merchantRow
	if e := s.DB.WithContext(ctx).Table("payments_merchants").Where("operator_id=? AND enabled=true", operator).Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []PublicMethod{}
	for _, row := range rows {
		m, e := s.openMerchant(row)
		if e != nil {
			return nil, e
		}
		if e := s.applyAssetNetwork(ctx, s.DB, &m, true); e != nil {
			continue
		}
		d := s.Drivers[m.Driver]
		if d == nil || ready(m, d) != nil {
			continue
		}
		currency := "CNY"
		switch m.Driver {
		case "stripe":
			currency = m.Credentials["currency"]
		case "nowpayments":
			currency = strings.ToUpper(m.Credentials["pay_currency"])
		case "evm_wallet":
			currency = m.Credentials["asset"]
		case "manual":
			currency = m.Credentials["currency"]
		case "kvoucher":
			currency = "USD"
		}
		view := PublicMethod{ID: m.ID, Driver: m.Driver, Currency: currency, Configured: true, RefundAPI: m.Driver == "stripe" || m.Driver == "alipay" || m.Driver == "wechat", AutoCharge: m.Driver == "stripe"}
		if m.Driver == "evm_wallet" {
			view.Network = m.Credentials["chain_id"]
			view.TokenContract = m.Credentials["token_contract"]
		}
		if m.Driver == "nowpayments" {
			view.Network = m.Credentials["pay_network"]
			view.TokenContract = m.Credentials["pay_token_contract"]
		}
		if m.Driver == "manual" {
			view.Instructions = m.Credentials["instructions"]
		}
		if m.Driver == "kvoucher" {
			view.RefundChannels = normalizeRefundChannels(m.Credentials["refund_channels"])
		}
		out = append(out, view)
	}
	return out, nil
}

type IntentView struct {
	Driver        string    `json:"driver"`
	Network       string    `json:"network,omitempty"`
	TokenContract string    `json:"token_contract,omitempty"`
	OrderID       string    `json:"order_id"`
	MerchantID    string    `json:"merchant_id"`
	ProviderID    string    `json:"provider_id,omitempty"`
	State         string    `json:"state"`
	Checkout      *Checkout `json:"checkout,omitempty"`
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
	view.Driver, view.Network, view.TokenContract = snapshot.PaymentDriver, snapshot.Network, snapshot.TokenContract
	if row.State == "created" && row.CheckoutCiphertext != "" {
		var checkout Checkout
		if e := s.open(row.CheckoutCiphertext, &checkout); e != nil {
			return view, e
		}
		view.Checkout = &checkout
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
