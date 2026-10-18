package portal

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPostgresResourceSearchRunsBeforePaginationAndKeepsOperatorScope(t *testing.T) {
	f := setupPortal(t)
	op, brand, owner := f.a.Installation.OperatorID, f.a.Installation.BrandID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(context.Background(), nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "Other operator", LegalName: "Other legal"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var targetCustomer, targetOrder, targetReceipt string
	for i := 0; i < 62; i++ {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", 1000+i)
		operator, name, email, status := op, fmt.Sprintf("Customer %d", i), fmt.Sprintf("ordinary-%d@search.test", i), "active"
		if i >= 60 {
			name, email, status = "Needle Studio", fmt.Sprintf("needle-%d@search.test", i), "suspended"
		}
		if i == 61 {
			operator = child.Operator.ID
		}
		customer := organization.Customer{ID: id, BrandID: brand, DisplayName: name, Status: status, RegisteredAt: now, NewCustomerRecordedAt: now}
		if err = f.a.DB.Table("organization_customers").Create(&customer).Error; err != nil {
			t.Fatal(err)
		}
		if err = f.a.DB.Exec("INSERT INTO organization_customer_identities (id,brand_id,customer_id,email_normalized,is_current,verified_at) VALUES (?,?,?,?,true,?)", uuid.NewString(), brand, id, email, now).Error; err != nil {
			t.Fatal(err)
		}
		ownership := organization.Ownership{ID: uuid.NewString(), CustomerID: id, BrandID: brand, OperatorID: operator, Version: 1, EffectiveAt: now, EvidenceReference: "synthetic scope"}
		if err = f.a.DB.Table("organization_service_ownerships").Create(&ownership).Error; err != nil {
			t.Fatal(err)
		}
		orderID := uuid.NewString()
		orderStatus := "pending"
		if i >= 60 {
			orderStatus = "closed"
		}
		order := commerce.Order{ID: orderID, OperatorID: operator, BrandID: brand, CustomerID: id, OwnershipVersion: 1, BusinessKey: fmt.Sprintf("search-order-%d", i), Fingerprint: "synthetic", Kind: "topup", DueUSD: ledger.MustDecimal("1"), InternalStatus: "none", CoverageStatus: "none", Status: orderStatus, Period: "lifetime", Timezone: "UTC", ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(-time.Duration(i) * time.Minute)}
		if err = f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		receiptID := fmt.Sprintf("10000000-0000-4000-8000-%012d", 1000+i)
		receipt := commerce.Receipt{ID: receiptID, OperatorID: operator, Provider: "manual", ProviderReceiptID: fmt.Sprintf("bank-%d", i), Fingerprint: "synthetic", GrossUSD: ledger.MustDecimal("1"), FeeStatus: "pending", Currency: "USD", OriginalAmount: "1", PayerReference: "synthetic payer", RecipientReference: "synthetic bank", VerificationEvidence: "synthetic evidence", VerifiedBy: owner, ObservedAt: now, CreatedAt: now}
		if i >= 60 {
			net, fee := ledger.MustDecimal("1"), ledger.Zero()
			receipt.NetUSD, receipt.FeeUSD, receipt.FeeStatus = &net, &fee, "confirmed"
			receipt.ProviderReceiptID = "needle-bank"
		}
		if err = f.a.DB.Table("commerce_receipts").Create(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		if i == 60 {
			targetCustomer, targetOrder, targetReceipt = id, orderID, receiptID
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + op
	for _, test := range []struct{ resource, query, status, target string }{{"customers", "Needle Studio", "suspended", targetCustomer}, {"orders", "needle-60@search.test", "closed", targetOrder}, {"orders", "Needle Studio", "closed", targetOrder}, {"payments", "needle-bank", "confirmed", targetReceipt}} {
		t.Run(test.resource+test.query, func(t *testing.T) {
			status, out, _ := f.call(t, "GET", scope+"/"+test.resource, nil, "")
			mustOK(t, status, out)
			if len(out["items"].([]any)) != 50 || out["next_cursor"] == nil {
				t.Fatal("initial page must stop before the target", out)
			}
			for _, row := range out["items"].([]any) {
				if row.(map[string]any)["id"] == test.target {
					t.Fatal("target unexpectedly in first page")
				}
			}
			path := scope + "/" + test.resource + "?q=" + url.QueryEscape(test.query) + "&status=" + test.status
			status, out, _ = f.call(t, "GET", path, nil, "")
			mustOK(t, status, out)
			rows := out["items"].([]any)
			if len(rows) != 1 || rows[0].(map[string]any)["id"] != test.target || out["search_supported"] != true || out["status_supported"] != true {
				t.Fatal("query did not search the full exact operator scope", out)
			}
			status, out, _ = f.call(t, "GET", path+"-does-not-exist", nil, "")
			mustOK(t, status, out)
			if len(out["items"].([]any)) != 0 {
				t.Fatal("status predicate was not applied", out)
			}
		})
	}
}

func TestPostgresRefundSearchPagesCasesBeforeLoadingTheirDetails(t *testing.T) {
	f := setupPortal(t)
	op := f.a.Installation.OperatorID
	now := time.Now().UTC()
	target := ""
	for i := 0; i < 55; i++ {
		r := remediation.Case{ID: uuid.NewString(), OperatorID: op, BusinessKey: fmt.Sprintf("search-refund-%d", i), Fingerprint: "synthetic", Kind: "unallocated", AmountUSD: ledger.MustDecimal("1"), State: "pending_review", Reason: "ordinary", Evidence: "synthetic", RequestedBy: f.a.Installation.OwnerStaffID, RequiredReviews: 1, CreatedAt: now.Add(-time.Duration(i) * time.Minute)}
		if i == 54 {
			r.Reason = "needle-refund"
			target = r.ID
		}
		if err := f.a.DB.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + op + "/refunds"
	status, out, _ := f.call(t, "GET", scope, nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 50 || out["next_cursor"] == "" {
		t.Fatal("missing case pagination", out)
	}
	status, out, _ = f.call(t, "GET", scope+"?q=needle-refund&status=pending_review", nil, "")
	mustOK(t, status, out)
	rows := out["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != target {
		t.Fatal("case beyond first page not found", out)
	}
}

func TestResourceSearchCapabilitiesDoNotAdvertiseTruncatedLists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, resource := range []string{"procurement", "exits", "evidence", "audit", "catalog-changes", "notifications"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "operatorId", Value: "operator"}}
		c.Request = httptest.NewRequest("GET", "/?q=needle", nil)
		p := page(resource)
		p["items"] = []any{entity("ordinary", "pending", 0, gin.H{})}
		prepareResourceFilters(c, p, resource)
		if p["search_supported"] != false || p["status_supported"] != false || len(p["items"].([]any)) != 1 {
			t.Fatal("bounded source advertised scope-wide filtering", resource, p)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "operatorId", Value: "operator"}}
	c.Request = httptest.NewRequest("GET", "/?q=needle&status=active", nil)
	p := page("plans")
	p["items"] = []any{entity("ordinary", "active", 0, gin.H{}), entity("needle", "active", 0, gin.H{})}
	prepareResourceFilters(c, p, "plans")
	if p["search_supported"] != true || len(p["items"].([]any)) != 1 {
		t.Fatal("complete authorized source was not filtered", p)
	}
}
