package app_test

import (
	"context"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestPersonalWalletRecordsCompleteAndScoped(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	prefix := fmt.Sprintf("wallet-records-%d", time.Now().UnixNano())
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "@example.test", "password": "password1"})
	other := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-other@example.test", "password": "password1"})
	userID := userIDOf(reg)
	token := tokenOf(reg)
	wallet, err := a.Billing.EnsureWallet(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_ledger(id,wallet_id,event_type,amount_minor,balance_after_minor,reserved_after_minor,idempotency_key,created_at) SELECT ?||'-ledger-'||lpad(n::text,3,'0'),?,'topup',1,1,0,?||'-ledger-'||n,'2026-10-10T00:00:00Z'::timestamptz FROM generate_series(1,151)n`, prefix, wallet.ID, prefix).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO payment_orders(id,user_id,adapter,purpose,amount_minor,currency,status,created_at) SELECT ?||'-order-'||lpad(n::text,3,'0'),?,'alipay','wallet',1,'CNY','paid','2026-10-10T00:00:00Z'::timestamptz FROM generate_series(1,151)n`, prefix, userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_commission_recoveries(id,wallet_id,settlement_id,commission_entry_id,credit_ledger_id,payout_ledger_id,amount_minor,recovered_minor,status,created_at) SELECT ?||'-claim-'||lpad(n::text,3,'0'),?,?||'-settlement-'||n,?||'-entry-'||n,?||'-ledger-001',?||'-ledger-001',1,1,'closed','2026-10-10T00:00:00Z'::timestamptz FROM generate_series(1,151)n`, prefix, wallet.ID, prefix, prefix, prefix, prefix).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_commission_recovery_receipts(id,recovery_id,amount_minor,reference,note,actor_user_id,idempotency_key) VALUES (?,?,1,?,'private internal note','private-actor',?)`, prefix+"-receipt", prefix+"-claim-151", prefix+"-receipt-reference", prefix+"-receipt").Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ledger", "orders", "recoveries"} {
		cursor := ""
		seen := map[string]bool{}
		var total int
		firstCursor := ""
		for pages := 0; pages < 10; pages++ {
			status, body := doJSON(t, "GET", server.URL+"/v1/me/wallet-records?kind="+kind+"&cursor="+url.QueryEscape(cursor)+"&user_id="+url.QueryEscape(userIDOf(other)), token, false, nil)
			if status != 200 {
				t.Fatalf("%s %d %+v", kind, status, body)
			}
			total = int(body["total"].(float64))
			items := body["items"].([]any)
			if len(items) > 25 {
				t.Fatal("unbounded page")
			}
			for _, raw := range items {
				item := raw.(map[string]any)
				id := item["id"].(string)
				if seen[id] {
					t.Fatal("same-time cursor repeated a record")
				}
				seen[id] = true
				if kind == "recoveries" {
					for _, rawReceipt := range item["receipts"].([]any) {
						receipt := rawReceipt.(map[string]any)
						if receipt["reference"] != prefix+"-receipt-reference" {
							t.Fatalf("original receipt reference missing %+v", receipt)
						}
						if receipt["actor_user_id"] != nil || receipt["note"] != "" {
							t.Fatalf("private recovery metadata leaked %+v", receipt)
						}
					}
				}
			}
			cursor = body["next_cursor"].(string)
			if pages == 0 {
				firstCursor = cursor
			}
			if cursor == "" {
				break
			}
		}
		if total < 151 || len(seen) != total {
			t.Fatalf("%s incomplete all pages %d/%d", kind, len(seen), total)
		}
		if status, _ := doJSON(t, "GET", server.URL+"/v1/me/wallet-records?kind="+kind+"&cursor="+url.QueryEscape(firstCursor), tokenOf(other), false, nil); status != 400 {
			t.Fatal("foreign user's cursor accepted")
		}
		status, empty := doJSON(t, "GET", server.URL+"/v1/me/wallet-records?kind="+kind, tokenOf(other), false, nil)
		if status != 200 || empty["total"] != float64(0) {
			t.Fatalf("another account saw records %+v", empty)
		}
	}
	if status, body := doJSON(t, "GET", server.URL+"/v1/me/wallet-records?cursor=invalid", token, false, nil); status != 400 || body["total"] != nil {
		t.Fatal("invalid cursor disguised as empty")
	}
}
