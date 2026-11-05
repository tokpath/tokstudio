package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresLedgerWorkspaceNamesSourcesAndSeparatesAuxiliaryAccounts(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "ledger-reader@example.test")
	f.login(t, "customer", "ledger-reader@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	ctx := context.Background()
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	order, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: op, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountMoney: ledger.MustDecimal("10.000000000000000001")})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/ledger"
	status, out, _ := f.call(t, "POST", "/v1/operators/"+op+"/payments", map[string]any{"order_id": order.ID, "provider_receipt_id": "independent-ledger-receipt", "currency": "USD", "gross_money": "10.000000000000000001", "verification_evidence": "synthetic verified receipt", "paid_at": time.Now().UTC()}, uuid.NewString())
	mustOK(t, status, out)
	grant, err := f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: op, CustomerID: customer, BusinessKey: uuid.NewString(), ActorID: owner, Reason: "synthetic future gift", Sources: []ledger.GrantSource{{Kind: "manual_gift", Amount: ledger.FromInt64(5)}}, EffectiveAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, out)
	if out["status_supported"] != false {
		t.Fatal("meaningless status filter remains")
	}
	seen := false
	for _, raw := range out["items"].([]any) {
		row := raw.(map[string]any)
		v := row["values"].(map[string]any)
		d := v["display"].(map[string]any)
		if d["auxiliary"] != false {
			t.Fatal("auxiliary account in business list", v)
		}
		if !strings.Contains(d["owner"].(string), "ledger-reader@example.test") {
			t.Fatal("missing readable owner", d)
		}
		if row["id"] == grant.AccountID {
			seen = true
			if d["available"] != "0" || d["availability"] != "尚未生效" {
				t.Fatal("future credits shown as spendable", d)
			}
		}
	}
	if !seen {
		t.Fatal("grant missing")
	}
	status, out, _ = f.call(t, "GET", base+"?include_auxiliary=true", nil, "")
	mustOK(t, status, out)
	orderFound, receiptFound := false, false
	for _, raw := range out["items"].([]any) {
		v := raw.(map[string]any)["values"].(map[string]any)
		d := v["display"].(map[string]any)
		if v["purpose"] == "order:funds" || v["purpose"] == "receipt:unallocated" {
			if !strings.Contains(d["owner"].(string), "ledger-reader@example.test") {
				t.Fatal("business account owner not resolved", d)
			}
			source := d["source"].(map[string]any)
			if source["url"] == "" {
				t.Fatal("original business link missing", source)
			}
			if _, ok := d["available"]; ok {
				t.Fatal("bookkeeping funds exposed as spendable", d)
			}
			orderFound = orderFound || v["purpose"] == "order:funds"
			receiptFound = receiptFound || v["purpose"] == "receipt:unallocated"
		}
	}
	if !orderFound || !receiptFound {
		t.Fatal("complete list omitted order or receipt")
	}
	status, out, _ = f.call(t, "GET", base+"?include_auxiliary=true&q=ledger-reader%40example.test", nil, "")
	mustOK(t, status, out)
	searchPurposes := map[string]bool{}
	for _, raw := range out["items"].([]any) {
		v := raw.(map[string]any)["values"].(map[string]any)
		d := v["display"].(map[string]any)
		if !strings.Contains(d["owner"].(string), "ledger-reader@example.test") {
			t.Fatal("owner search returned an unrelated account", d)
		}
		searchPurposes[v["purpose"].(string)] = true
	}
	if !searchPurposes["order:funds"] || !searchPurposes["receipt:unallocated"] {
		t.Fatal("owner search missed auxiliary accounts", searchPurposes)
	}
	status, out, _ = f.call(t, "GET", base+"?view=journals", nil, "")
	mustOK(t, status, out)
	last := time.Now().Add(time.Hour)
	allocation := false
	for _, raw := range out["items"].([]any) {
		v := raw.(map[string]any)["values"].(map[string]any)
		at, err := time.Parse(time.RFC3339Nano, v["created_at"].(string))
		if err != nil || at.After(last) {
			t.Fatal("journals not newest first", v)
		}
		last = at
		if v["kind"] == "receipt_allocation" {
			allocation = true
			for _, rawEntry := range v["entries"].([]any) {
				entry := rawEntry.(map[string]any)
				d := entry["display"].(map[string]any)
				if d["owner"] == "" || d["source"] == nil {
					t.Fatal("nested entry has no business context", entry)
				}
				if !strings.Contains(entry["amount"].(string), "10.000000000000000001") {
					t.Fatal("precision lost", entry)
				}
			}
		}
	}
	if !allocation {
		t.Fatal("allocation journal missing")
	}
	// Evidence stays available, while display labels never substitute for IDs.
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), order.ID) {
		t.Fatal("original identifiers lost")
	}
}
