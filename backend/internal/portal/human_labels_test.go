package portal

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestSupplierLabelsUseTheCredentialNamePeopleSee(t *testing.T) {
	providerID := uuid.NewString()
	labels := supplierLabels([]catalog.CredentialView{
		{ID: "named", Name: "onerouter", ProviderID: providerID, Fingerprint: "abc"},
		{ID: "blank", Name: "  ", ProviderID: providerID, Fingerprint: "c07e3f6b0758"},
	})
	if labels["named"] != "onerouter" || labels["blank"] != "供应商凭据 · c07e3f6b0758" {
		t.Fatal(labels)
	}
}

func TestPostgresSupplierModelReferencesDistinguishSuppliersAndSearchTheirNames(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op := f.a.Installation.OperatorID
	for _, supplier := range []struct {
		name    string
		enabled bool
	}{{"North supplier", true}, {"South supplier", true}, {"Retired supplier", false}} {
		credentialID := uuid.NewString()
		if _, err := f.a.Catalog.SaveCredential(ctx, op, credentialID, uuid.NewString(), supplier.name, "private-test-secret", true, 0); err != nil {
			t.Fatal(err)
		}
		model := catalog.SupplierModel{ID: uuid.NewString(), CredentialID: credentialID, Provider: "openai", Model: "same-model", Enabled: supplier.enabled, Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}}
		if err := f.a.Catalog.SaveSupplierModel(ctx, op, model, 0); err != nil {
			t.Fatal(err)
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for _, kind := range []string{"supplier-models", "cost-models"} {
		path := "/v1/operators/" + op + "/references/" + kind
		status, out, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
		want := map[string]bool{"same-model · North supplier · openai": true, "same-model · South supplier · openai": true}
		if kind == "cost-models" {
			want["same-model · Retired supplier · openai"] = true
		}
		items := out["items"].([]any)
		if len(items) != len(want) {
			t.Fatal(kind, out)
		}
		for _, raw := range items {
			label := raw.(map[string]any)["label"].(string)
			if !want[label] {
				t.Fatal("missing supplier name or leaked unrelated data", kind, raw)
			}
			delete(want, label)
		}
		status, out, _ = f.call(t, "GET", path+"?q="+url.QueryEscape("south supplier"), nil, "")
		mustOK(t, status, out)
		items = out["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["label"] != "same-model · South supplier · openai" {
			t.Fatal("supplier-name search did not disambiguate models", kind, out)
		}
	}
}

func TestHumanRecordTitleUsesSavedNamesAndBusinessFacts(t *testing.T) {
	internal := uuid.NewString()
	if got := humanRecordTitle(gin.H{"name": "采购月包", "id": internal}, "procurement", nil); got != "采购月包" {
		t.Fatal(got)
	}
	title := humanRecordTitle(gin.H{"provider_receipt_id": internal, "created_at": "2026-10-18T12:34:56Z"}, "payments", nil)
	if strings.Contains(title, internal) || !strings.Contains(title, "收款") || !strings.Contains(title, "2026-10-18") {
		t.Fatal(title)
	}
}
func TestPostgresHumanLabelsRespectExactServiceAndSupplyScopes(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op, owner, brand := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID, f.a.Installation.BrandID
	child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "直属渠道", LegalName: "渠道公司"})
	if e != nil {
		t.Fatal(e)
	}
	createCustomer := func(operator, name, email string) string {
		cid := uuid.NewString()
		now := time.Now().UTC()
		customer := organization.Customer{ID: cid, BrandID: brand, DisplayName: name, Status: "active", RegisteredAt: now, NewCustomerRecordedAt: now}
		if e := f.a.DB.Table("organization_customers").Create(&customer).Error; e != nil {
			t.Fatal(e)
		}
		if e := f.a.DB.Exec("INSERT INTO organization_customer_identities(id,brand_id,customer_id,email_normalized,is_current,verified_at) VALUES(?,?,?,?,true,?)", uuid.NewString(), brand, cid, email, now).Error; e != nil {
			t.Fatal(e)
		}
		own := organization.Ownership{ID: uuid.NewString(), CustomerID: cid, BrandID: brand, OperatorID: operator, Version: 1, EffectiveAt: now, EvidenceReference: "scope fixture"}
		if e := f.a.DB.Table("organization_service_ownerships").Create(&own).Error; e != nil {
			t.Fatal(e)
		}
		return cid
	}
	own := createCustomer(op, "同名客户", "own@labels.test")
	foreign := createCustomer(child.Operator.ID, "下级秘密客户", "secret@labels.test")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Params = gin.Params{{Key: "operatorId", Value: op}}
	c.Set("identity", &identity{Session: browserSession{Kind: "staff", SubjectID: owner}, Staff: Staff{ID: owner}})
	p := page("orders")
	p["items"] = []any{entity("own-row", "active", 1, gin.H{"customer_id": own, "requested_by": own, "subject_id": own}), entity("foreign-row", "active", 1, gin.H{"customer_id": foreign, "requested_by": foreign, "subject_id": foreign}), entity("supply", "active", 1, gin.H{"supplier_id": op, "buyer_id": child.Operator.ID}), entity("empty-references", "active", 1, gin.H{"order_id": uuid.NewString(), "receipt_id": uuid.NewString(), "campaign_id": uuid.NewString(), "package_id": uuid.NewString(), "key_id": uuid.NewString(), "grant_id": uuid.NewString(), "notification_id": uuid.NewString(), "contract_attachment_id": uuid.NewString(), "credential_id": uuid.NewString(), "staff_id": owner, "model_id": "synthetic-not-visible", "subject_id": "policy:" + op})}
	if e := f.a.projectResourceLabels(c, p, "orders"); e != nil {
		t.Fatal(e)
	}
	rows := p["items"].([]any)
	names := rows[0].(gin.H)["presentation"].(humanPresentation).Labels
	if names["customer_id"] != "同名客户 · own@labels.test" || names["requested_by"] != names["customer_id"] || names["subject_id"] != names["customer_id"] {
		t.Fatal(names)
	}
	names = rows[1].(gin.H)["presentation"].(humanPresentation).Labels
	if names["customer_id"] != "" || names["requested_by"] != "" || names["subject_id"] != "" {
		t.Fatal("parent leaked child customer identity", names)
	}
	names = rows[2].(gin.H)["presentation"].(humanPresentation).Labels
	if !strings.Contains(names["buyer_id"], "直属渠道") || !strings.Contains(names["supplier_id"], "Test Operator") {
		t.Fatal(names)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/references/export-subjects?kind=ledger", nil, "")
	if status != 400 {
		t.Fatal("obsolete export selector remains available", status, out)
	}
	// Customer presentation never resolves another customer, even if a future
	// projection accidentally includes an unrelated customer_id.
	c.Params = nil
	c.Set("identity", &identity{Session: browserSession{Kind: "customer", SubjectID: own}, Customer: organization.Principal{Customer: organization.Customer{ID: own}, Ownership: organization.Ownership{OperatorID: op}}})
	if e := f.a.projectResourceLabels(c, p, "account"); e != nil {
		t.Fatal(e)
	}
	if names = rows[1].(gin.H)["presentation"].(humanPresentation).Labels; names["customer_id"] != "" {
		t.Fatal("customer presentation leaked another customer", names)
	}
}

func TestPostgresReferenceSelectorsSearchNamesBeforePaging(t *testing.T) {
	f := setupPortal(t)
	op := f.a.Installation.OperatorID
	now := time.Now().UTC()
	for i := 0; i < 62; i++ {
		id := fmt.Sprintf("21000000-0000-4000-8000-%012d", i)
		name := fmt.Sprintf("可读套餐%02d", i)
		p := commerce.Package{ID: id, OperatorID: op, Name: name, NewPurchaseEnabled: false, RenewalEnabled: false, CreatedAt: now}
		if e := f.a.DB.Table("commerce_packages").Create(&p).Error; e != nil {
			t.Fatal(e)
		}
		receipt := commerce.Receipt{ID: fmt.Sprintf("22000000-0000-4000-8000-%012d", i), OperatorID: op, Provider: "manual", ProviderReceiptID: fmt.Sprintf("bank-%02d", i), Fingerprint: "synthetic", GrossMoney: ledger.MustDecimal("2"), FeeStatus: "pending", Currency: "USD", OriginalAmount: "2", ObservedAt: now, CreatedAt: now}
		if e := f.a.DB.Table("commerce_receipts").Create(&receipt).Error; e != nil {
			t.Fatal(e)
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for _, kind := range []string{"plans", "receipts"} {
		path := "/v1/operators/" + op + "/references/" + kind
		status, out, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
		items := out["items"].([]any)
		if len(items) != 50 || out["next_cursor"] == "" {
			t.Fatal("missing selector pagination", kind, out)
		}
		for _, raw := range items {
			row := raw.(map[string]any)
			if strings.Contains(row["label"].(string), row["value"].(string)) {
				t.Fatal("UUID appended to readable selector label", row)
			}
		}
		status, out, _ = f.call(t, "GET", path+"?cursor="+out["next_cursor"].(string), nil, "")
		mustOK(t, status, out)
		if len(out["items"].([]any)) != 12 {
			t.Fatal("selector failed to continue original results", kind, out)
		}
		q := "可读套餐61"
		if kind == "receipts" {
			q = "bank-61"
		}
		status, out, _ = f.call(t, "GET", path+"?q="+url.QueryEscape(q), nil, "")
		mustOK(t, status, out)
		items = out["items"].([]any)
		if len(items) != 1 || !strings.Contains(items[0].(map[string]any)["label"].(string), q) {
			t.Fatal("selector filtered only its first page", kind, out)
		}
	}
}

func TestPostgresOperatorsOfferFirstOwnerOnlyWithoutAnyOwnerMembership(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	ids := []string{}
	for i := 0; i < 2; i++ {
		child, e := f.a.Organization.CreateOperator(context.Background(), nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: fmt.Sprintf("渠道%d", i), LegalName: "真实主体"})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, child.Operator.ID)
	}
	membership := organization.Membership{ID: uuid.NewString(), OperatorID: ids[0], StaffID: owner, Role: "owner", Status: "suspended", CreatedAt: time.Now().UTC()}
	if e := f.a.DB.Table("organization_staff_memberships").Create(&membership).Error; e != nil {
		t.Fatal(e)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/operators", nil, "")
	mustOK(t, status, out)
	found := map[string]bool{}
	for _, raw := range out["items"].([]any) {
		row := raw.(map[string]any)
		for _, action := range row["allowed_actions"].([]any) {
			if action == "invite_owner" {
				found[row["id"].(string)] = true
			}
		}
	}
	if found[ids[0]] || !found[ids[1]] {
		t.Fatal("first-owner action does not match existing membership guard", found)
	}
}
