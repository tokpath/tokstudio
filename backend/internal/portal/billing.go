package portal

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/usage"
)

func (a *Application) billingDetails(ctx context.Context, r usage.Request, out gin.H) error {
	if len(r.Settlement) == 0 {
		return nil
	}
	var settlement usage.Settlement
	if e := json.Unmarshal(r.Settlement, &settlement); e != nil {
		return e
	}
	var quote catalog.ModelQuote
	if e := json.Unmarshal(r.PriceSnapshot, &quote); e != nil {
		return e
	}
	measurements := map[string]string{}
	units := map[string]string{}
	for _, m := range settlement.Usage.Measurements {
		measurements[m.Component] = m.Quantity
		units[m.Component] = m.Unit
	}
	labels := map[string]string{}
	if quote.Billing.Billing != nil {
		for _, component := range quote.Billing.Billing.Components(quote.Billing.Operation) {
			labels[component.Key] = component.Label
		}
	}
	components := []any{}
	displayed := ledger.Zero()
	for _, rate := range quote.Retail.Components {
		quantity, exists := measurements[rate.Key]
		if !exists {
			continue
		}
		single := quote.Retail
		single.Components = []catalog.ComponentRate{rate}
		exact, e := single.Charge(map[string]string{rate.Key: quantity})
		if e != nil {
			return e
		}
		amount, _ := ledger.Quantize(exact)
		displayed = displayed.Add(amount)
		description := rate.Rate + " / " + rate.Per + " " + rate.Unit
		if quote.Retail.FactorNumerator != "1" || quote.Retail.FactorDenominator != "1" {
			description += " × " + quote.Retail.FactorNumerator + "/" + quote.Retail.FactorDenominator
		}
		components = append(components, gin.H{"label": labels[rate.Key], "component": rate.Key, "quantity": quantity, "unit": units[rate.Key], "price_description": description, "amount_atg": amount})
	}
	difference := r.ActualATG.Sub(displayed)
	if !difference.IsZero() {
		components = append(components, gin.H{"component": "precision_carry", "quantity": "1", "unit": "adjustment", "price_description": "按账本累计精度余数结算", "amount_atg": difference})
	}
	out["components"] = components
	out["actual_atg"] = r.ActualATG
	out["merchant_shortfall_atg"] = r.MerchantShortfall
	allocations := []any{}
	for _, event := range settlement.Customer.Events {
		g, e := a.Ledger.GetGrant(ctx, nil, event.GrantID)
		if e != nil {
			return e
		}
		var sources []ledger.GrantSource
		if e = json.Unmarshal(event.Sources, &sources); e != nil {
			return e
		}
		kinds := []string{}
		parts := []any{}
		for _, source := range sources {
			kinds = append(kinds, source.Kind)
			parts = append(parts, gin.H{"kind": source.Kind, "amount_atg": source.Amount})
		}
		name := "权益来源 " + g.ID
		if g.OrderID != "" {
			name = "订单 " + g.OrderID
		}
		allocations = append(allocations, gin.H{"source_name": name, "kind": strings.Join(kinds, " + "), "amount_atg": event.Amount, "paid_basis_money": event.PaidMoney, "sources": parts})
	}
	out["allocations"] = allocations
	return nil
}
