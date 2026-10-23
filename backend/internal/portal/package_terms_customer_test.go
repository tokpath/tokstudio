package portal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresReadablePackageTermsReachPurchaseAndRenewalConfirmation(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	f.registered(t, "readable-terms-customer@example.test")
	f.login(t, "customer", "readable-terms-customer@example.test", "customer-password-test")
	var customer organization.Customer
	if err := f.a.DB.Table("organization_customers").Where("brand_id=?", f.a.Installation.BrandID).Take(&customer).Error; err != nil {
		t.Fatal(err)
	}
	publish := func(packageID string, version int64, terms string) commerce.PackageQuote {
		t.Helper()
		q, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: parent, PackageID: packageID, ExpectedVersion: version, Name: "Readable customer plan", PriceUSD: ledger.MustDecimal("10"), PaidATG: ledger.MustDecimal("100"), GiftATG: ledger.MustDecimal("20"), Period: ledger.Month, Timezone: "UTC", TermsSummary: terms, NewPurchaseEnabled: true, RenewalEnabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	original := publish("", 0, "原服务支持约定")
	// Seed a delivered-package projection fixture. Financial delivery and grants
	// are independently covered by commerce tests; this test exercises browser
	// DTOs and acceptance against immutable quote facts, without adding money.
	now := time.Now().UTC()
	expiry := now.AddDate(0, 1, 0)
	order := commerce.Order{ID: uuid.NewString(), OperatorID: parent, OriginalOperatorID: parent, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customer.ID, BusinessKey: uuid.NewString(), Fingerprint: "presentation-fixture", OwnershipVersion: 1, Kind: "package", QuoteID: &original.ID, DueUSD: ledger.MustDecimal("10"), PaidATG: ledger.MustDecimal("100"), PackageGiftATG: ledger.MustDecimal("20"), InternalStatus: "none", CoverageStatus: "none", Status: "completed", Period: ledger.Month, Timezone: "UTC", ExpiresAt: expiry, CreatedAt: now, CompletedAt: &now}
	if err := f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Table("commerce_deliveries").Create(&commerce.Delivery{ID: uuid.NewString(), OrderID: order.ID, EffectiveAt: now, ExpiresAt: &expiry, DeliveredAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	subscription, err := f.a.Commerce.EnableAutoRenewal(ctx, nil, customer.ID, order.ID, "customer authorizes renewal fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	next := publish(original.PackageID, 1, "新的客户支持约定")
	status, out, _ := f.call(t, "GET", "/v1/public/plans", nil, "")
	mustOK(t, status, out)
	item := out["items"].([]any)[0].(map[string]any)
	if item["quote_id"] != next.ID || !strings.Contains(item["terms_summary"].(string), "新的客户支持约定") || strings.Contains(item["description"].(string), "package-terms:") {
		t.Fatal("catalog does not show readable current terms", item)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": original.PackageID}, "")
	mustOK(t, status, out)
	if !strings.Contains(out["terms_summary"].(string), "新的客户支持约定") {
		t.Fatal("purchase confirmation omitted actual terms", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, out)
	item = out["items"].([]any)[0].(map[string]any)
	renewal := item["renewal_quote"].(map[string]any)
	if item["requires_confirmation"] != true || !strings.Contains(item["terms_summary"].(string), "原服务支持约定") || renewal["quote_id"] != next.ID || !strings.Contains(renewal["terms_summary"].(string), "新的客户支持约定") {
		t.Fatal("renewal does not distinguish accepted and proposed terms", item)
	}
	path := "/v1/me/plans/" + subscription.ID + "/renewal"
	for _, quoteID := range []string{"", original.ID} {
		status, out, _ = f.call(t, "PATCH", path, map[string]any{"auto_renew": true, "version": subscription.Version, "confirmed_changed_terms": true, "quote_id": quoteID}, "")
		if status != 409 {
			t.Fatal("renewal accepted missing or stale displayed quote", status, out)
		}
	}
	newer := publish(original.PackageID, 2, "进一步变更的约定")
	status, out, _ = f.call(t, "PATCH", path, map[string]any{"auto_renew": true, "version": subscription.Version, "confirmed_changed_terms": true, "quote_id": next.ID}, "")
	if status != 409 {
		t.Fatal("renewal consent silently moved to a newly published quote", status, out)
	}
	status, out, _ = f.call(t, "PATCH", path, map[string]any{"auto_renew": true, "version": subscription.Version, "confirmed_changed_terms": true, "quote_id": newer.ID}, "")
	mustOK(t, status, out)
	stored, err := f.a.Commerce.GetSubscription(ctx, nil, customer.ID, subscription.ID)
	if err != nil || stored.AuthorizedQuoteID != newer.ID {
		t.Fatal("confirmed renewal was not bound to the displayed quote", stored, err)
	}
	// A customer who did not authorize renewal at purchase must receive the
	// same protection when enabling it later, rather than bypassing consent.
	standalone := order
	standalone.ID, standalone.BusinessKey = uuid.NewString(), uuid.NewString()
	if err = f.a.DB.Table("commerce_orders").Create(&standalone).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Table("commerce_deliveries").Create(&commerce.Delivery{ID: uuid.NewString(), OrderID: standalone.ID, EffectiveAt: now, ExpiresAt: &expiry, DeliveredAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	path = "/v1/me/plans/" + standalone.ID + "/renewal"
	status, out, _ = f.call(t, "PATCH", path, map[string]any{"auto_renew": true, "version": 1, "quote_id": newer.ID}, "")
	if status != 409 {
		t.Fatal("first-time renewal bypassed changed-term consent", status, out)
	}
	status, out, _ = f.call(t, "PATCH", path, map[string]any{"auto_renew": true, "version": 1, "quote_id": newer.ID, "confirmed_changed_terms": true}, "")
	mustOK(t, status, out)
	created, err := f.a.Commerce.GetSubscription(ctx, nil, customer.ID, out["id"].(string))
	if err != nil || created.AuthorizedQuoteID != newer.ID {
		t.Fatal(created, err)
	}
	// Publishing an in-app reminder is sufficient for an existing mandate;
	// this customer never reads a notification and no email jobs are delivered.
	if err = f.a.runRenewal(ctx, workerPayload{ID: created.ID, CustomerID: customer.ID, OperatorID: parent}); err != nil {
		t.Fatal(err)
	}
	created, err = f.a.Commerce.GetSubscription(ctx, nil, customer.ID, created.ID)
	if err != nil || created.NoticeDeliveredAt == nil || !strings.HasPrefix(created.NoticeEvidence, "in-app-published:") {
		t.Fatal("renewal still requires reading mail or notifications", created, err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ = f.call(t, "POST", "/v1/operators/"+parent+"/plans", map[string]any{"package_id": original.PackageID, "expected_version": 3, "name": "Readable customer plan", "price_usd": "10", "paid_atg": "100", "gift_atg": "20", "period": "month", "new_purchase_enabled": true, "renewal_enabled": false}, "")
	mustOK(t, status, out)
	pkg, err := f.a.Commerce.GetPackage(ctx, nil, parent, original.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.runPackageChange(ctx, workerPayload{ID: *pkg.CurrentQuoteID, OperatorID: parent}); err != nil {
		t.Fatal(err)
	}
	if err = f.a.runPackageChange(ctx, workerPayload{ID: *pkg.CurrentQuoteID, OperatorID: parent}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = f.a.DB.Table("operations_notifications").Where("recipient_id=? AND title=?", customer.ID, "套餐已停止续费").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("stop-renewal notice was lost or duplicated", count, err)
	}
}
