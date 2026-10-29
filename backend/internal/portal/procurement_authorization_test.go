package portal

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	appcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

func TestPostgresProcurementAuthorizationRevisionCASAndPaymentBoundaries(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.Channel, Name: "authorization buyer"})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, child.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: parent, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{FromMoney: ledger.Zero(), GiftPercent: ledger.FromInt64(900)}}, ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":false,"method":"non_refundable","terms_reference":"original-exit"}`), TermsReference: "fixed-quote", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	merchant := func() string {
		t.Helper()
		id := uuid.NewString()
		_, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: id, OperatorID: parent, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_revision", "publishable_key": "pk_test_revision", "webhook_secret": "whsec_revision", "currency": "USD"}}, 0)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	firstMerchant, secondMerchant := merchant(), merchant()
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + child.Operator.ID + "/procurement"
	base := map[string]any{"action": "authorization", "amount_money": "10", "supplier_id": parent, "quote_id": quote.ID, "trigger_available_atg": "10.000000000000000001", "period_limit_money": "20", "period": "month", "mode": "automatic", "merchant_id": firstMerchant, "enabled": true}
	body := func(id string, version int, consent string) map[string]any {
		v := map[string]any{}
		for key, value := range base {
			v[key] = value
		}
		v["expected_version"] = version
		v["consent_confirmed"] = consent != ""
		if version > 0 {
			v["id"] = id
		}
		return v
	}
	post := func(input map[string]any, key string) (int, map[string]any) {
		status, out, _ := f.call(t, "POST", path, input, key)
		return status, out
	}
	first := body("", 0, "confirmed-contract")
	first["payment"] = map[string]any{"payment_method": "pm_original_revision", "customer_ref": "cus_original_revision"}
	status, out := post(first, "")
	mustOK(t, status, out)
	id := out["id"].(string)
	read := func(version float64) {
		t.Helper()
		status, out, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
		if len(out["views"].([]any)) != 1 || out["views"].([]any)[0].(map[string]any)["value"] != "buyer" {
			t.Fatal("channel exposes an irrelevant wholesale view", out["views"])
		}
		for _, op := range out["operations"].([]any) {
			if op.(map[string]any)["name"] == "authorization" {
				t.Fatal("existing authorization still offers a misleading version-zero creation form")
			}
		}
		found := false
		for _, item := range out["items"].([]any) {
			row := item.(map[string]any)
			if row["id"] != id {
				continue
			}
			found = true
			v := row["values"].(map[string]any)
			if row["version"] != version || v["quote_id"] != quote.ID || v["trigger_available_atg"] != "10.000000000000000001" {
				t.Fatal("revision is not bound to the original authorization configuration", row)
			}
			if v["payment_ciphertext"] != nil || v["payment"] != nil {
				t.Fatal("saved sensitive payment materials were exposed")
			}
		}
		if !found {
			t.Fatal("authorization row missing")
		}
		encoded, _ := json.Marshal(out)
		if strings.Contains(string(encoded), "pm_original_revision") || strings.Contains(string(encoded), "cus_original_revision") {
			t.Fatal("original sensitive materials leaked into the workspace response")
		}
		for _, value := range out["row_operations"].([]any) {
			op := value.(map[string]any)
			if op["name"] != "authorization" {
				continue
			}
			for _, value := range op["fields"].([]any) {
				field := value.(map[string]any)
				if field["name"] == "renewed_consent_reference" || field["name"] == "consent_reference" || field["name"] == "single_limit_money" {
					t.Fatal("old consent was filled as a new confirmation", field)
				}
			}
			return
		}
		t.Fatal("missing authorization revision operation")
	}
	read(1)
	revision := body(id, 1, "confirmed-contract") // Explicitly confirming the same contract is allowed.
	revision["period_limit_money"] = "30"
	key := uuid.NewString()
	status, out = post(revision, key)
	mustOK(t, status, out)
	if out["authorization"].(map[string]any)["authorized_by"] != owner || !strings.HasPrefix(out["authorization"].(map[string]any)["consent_reference"].(string), "procurement-consent:"+owner+":") {
		t.Fatal("idempotent command did not retain its sanitized authorization evidence", out)
	}
	if out["id"] != id || out["version"] != float64(2) {
		t.Fatal("revision cancelled/recreated instead of updating the original", out)
	}
	status, out = post(revision, key)
	mustOK(t, status, out)
	read(2)
	for _, testCase := range []struct {
		name     string
		change   func(map[string]any)
		expected int
	}{
		{"changed merchant without materials", func(v map[string]any) { v["merchant_id"] = secondMerchant }, 400},
		{"half payment material", func(v map[string]any) { v["payment"] = map[string]any{"payment_method": "pm_partial"} }, 400},
		{"missing current consent", func(v map[string]any) { delete(v, "consent_confirmed") }, 400},
		{"foreign authorization identifier", func(v map[string]any) { v["id"] = uuid.NewString() }, 403},
		{"stale version", func(v map[string]any) { v["expected_version"] = 1 }, 409},
	} {
		v := body(id, 2, "confirmed-again")
		testCase.change(v)
		status, out = post(v, "")
		if status != testCase.expected {
			t.Fatal(testCase.name, status, out)
		}
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); status, _ := post(body(id, 2, "confirmed-concurrent"), ""); results <- status }()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("concurrent revisions bypassed CAS", counts)
	}
	status, out = post(revision, key)
	mustOK(t, status, out)
	if out["version"] != float64(2) {
		t.Fatal("same-key original retry did not return the original response", out)
	}
	read(3)
	var stored cooperation.ReplenishmentAuthorization
	if err = f.a.DB.Where("id=?", id).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := appcrypto.Open(f.a.AutoReplenishments.EncryptionKey, stored.PaymentCiphertext)
	if err != nil || !strings.Contains(plain, "pm_original_revision") || !strings.Contains(plain, "cus_original_revision") {
		t.Fatal("same-merchant revision lost the original encrypted authorization", err)
	}
	if stored.Version != 3 || stored.AuthorizedBy != owner || stored.UpdatedAt.IsZero() {
		t.Fatal("revision did not record its own actor/time/version", stored.Version)
	}
	changed := body(id, 3, "confirmed-changed-merchant")
	changed["merchant_id"], changed["payment"] = secondMerchant, map[string]any{"payment_method": "pm_changed_revision", "customer_ref": "cus_changed_revision"}
	status, out = post(changed, "")
	mustOK(t, status, out)
	if err = f.a.DB.Where("id=?", id).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	plain, err = appcrypto.Open(f.a.AutoReplenishments.EncryptionKey, stored.PaymentCiphertext)
	if err != nil || stored.Version != 4 || stored.MerchantID == nil || *stored.MerchantID != secondMerchant || !strings.Contains(plain, "pm_changed_revision") || strings.Contains(plain, "pm_original_revision") {
		t.Fatal("changed merchant did not require and save its new complete material", err)
	}
	status, out, _ = f.call(t, "POST", path+"/"+id+"/cancel_authorization", map[string]any{"version": 3}, "")
	if status != 409 {
		t.Fatal("stale cancellation was not a reviewable CAS conflict", status, out)
	}
	status, out, _ = f.call(t, "POST", path+"/"+uuid.NewString()+"/cancel_authorization", map[string]any{"version": 3}, "")
	if status != 404 {
		t.Fatal("cancellation accepted a different authorization object", status, out)
	}
	status, out, _ = f.call(t, "GET", path+"?view=supplier", nil, "")
	mustOK(t, status, out)
	for _, item := range out["items"].([]any) {
		if item.(map[string]any)["id"] == id {
			t.Fatal("supplier view mixed in buyer's own authorization")
		}
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+parent+"/procurement", nil, "")
	mustOK(t, status, out)
	if len(out["views"].([]any)) != 1 || out["views"].([]any)[0].(map[string]any)["value"] != "supplier" {
		t.Fatal("platform did not open its actual supply view", out["views"])
	}
	foundQuote := false
	for _, item := range out["items"].([]any) {
		if item.(map[string]any)["id"] == quote.ID {
			foundQuote = true
		}
	}
	if !foundQuote {
		t.Fatal("platform default page does not show its real supplier quote")
	}
}
