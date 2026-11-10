package portal

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func setupUSDTWithdrawal(t *testing.T) (*testApp, ledger.Account, incentives.WithdrawalPolicy) {
	t.Helper()
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "usdt@example.test")
	f.login(t, "customer", "usdt@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op, owner, ctx := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID, t.Context()
	account, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: "customer:money", Unit: ledger.MoneyUnit})
	if err != nil {
		t.Fatal(err)
	}
	control, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: op, Purpose: "control:test-opening", Unit: ledger.MoneyUnit, AllowNegative: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Ledger.Post(ctx, nil, ledger.Posting{OperatorID: op, BusinessKey: uuid.NewString(), Kind: "test_opening", SourceID: customer, ActorID: "test", Reason: "isolated USDT withdrawal fixture", Lines: []ledger.Line{{AccountID: account.ID, Amount: ledger.FromInt64(100)}, {AccountID: control.ID, Amount: ledger.FromInt64(-100)}}})
	if err != nil {
		t.Fatal(err)
	}
	method, err := f.a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, op, incentives.WithdrawalPolicy{Wallet: "balance", MethodID: uuid.NewString(), MethodName: "USDT", PayoutKind: "usdt", Enabled: true, MinimumMoney: ledger.FromInt64(1), FeeRate: ledger.MustDecimal("0.03")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.a.Organization.GetBusinessStatus(ctx, nil, op)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: organization.Settlement, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "isolated USDT payout fixture"}, f.a)
	if err != nil {
		t.Fatal(err)
	}
	return f, account, method
}

func TestPostgresUSDTWithdrawalRejectsInvalidAddressBeforeHoldingFunds(t *testing.T) {
	f, account, method := setupUSDTWithdrawal(t)
	body := map[string]any{"method_id": method.MethodID, "policy_id": method.ID, "amount_money": "15", "recipient": map[string]string{"network": "TRC20", "address": "test"}}
	address, _ := url.Parse(f.server.URL)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		f.client.Jar.SetCookies(address, []*http.Cookie{{Name: "NEXT_LOCALE", Value: locale, Path: "/"}})
		for _, path := range []string{"/v1/me/balance-withdrawals", "/v1/me/withdrawals"} {
			status, rejected, _ := f.call(t, "POST", path, body, uuid.NewString())
			problem, ok := rejected["error"].(map[string]any)
			source := "USDT 钱包地址无效或与所选网络不匹配，请核对钱包地址和网络后重新提交。"
			if !ok || status != 400 || problem["code"] != "invalid_payout_address" || problem["retryable"] != false || problem["message"] != httpx.LocalizedText(locale, source) || (locale != "zh" && problem["message"] == source) {
				t.Fatal("invalid address must have a localized, actionable error", locale, path, status, rejected)
			}
		}
	}
	var count int64
	if err := f.a.DB.Table("incentives_withdrawals").Where("customer_id=?", account.OwnerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid address created a withdrawal", count, err)
	}
	current, err := f.a.Ledger.GetAccount(t.Context(), account.ID)
	if err != nil || !current.Held.IsZero() || current.Balance.String() != "100" {
		t.Fatal("invalid address changed balance or holds", current, err)
	}
	for network, recipient := range map[string]string{"TRC20": "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", "ERC20": "0x1234567890123456789012345678901234567890"} {
		body["recipient"] = map[string]string{"network": network, "address": recipient}
		key, id := uuid.NewString(), ""
		for range 2 {
			status, out, _ := f.call(t, "POST", "/v1/me/balance-withdrawals", body, key)
			mustOK(t, status, out)
			if id != "" && out["id"] != id {
				t.Fatal("retry created a duplicate withdrawal", out)
			}
			id = out["id"].(string)
		}
		var row incentives.Withdrawal
		if err := f.a.DB.Table("incentives_withdrawals").Where("id=?", id).First(&row).Error; err != nil || row.AmountMoney.String() != "15" || row.FeeMoney.String() != "0.45" || row.NetMoney.String() != "14.55" || row.RecipientReference != "网络："+network+"\n钱包地址："+recipient {
			t.Fatal("valid address, amount or fee changed", row, err)
		}
		current, err = f.a.Ledger.GetAccount(t.Context(), account.ID)
		if err != nil || current.Held.String() != "15" || current.Balance.String() != "100" {
			t.Fatal("accepted withdrawal must hold exactly 15 once", current, err)
		}
		status, out, _ := f.call(t, "POST", "/v1/me/balance-withdrawals/"+id+"/cancel", map[string]any{}, uuid.NewString())
		mustOK(t, status, out)
		current, err = f.a.Ledger.GetAccount(t.Context(), account.ID)
		if err != nil || !current.Held.IsZero() || current.Balance.String() != "100" {
			t.Fatal("cancellation did not release the hold", current, err)
		}
	}
}

// Opt-in browser fixture with real auth, HTTP handlers and isolated ledger funds.
func TestExternalUSDTWithdrawalAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_USDT_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated USDT withdrawal server")
	}
	f, _, _ := setupUSDTWithdrawal(t)
	f.server.Close()
	f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	f.a.Config.PublicBaseURL = f.a.Config.WebOrigin
	listener, err := net.Listen("tcp", "127.0.0.1:18081")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]string{"email": "usdt@example.test", "password": "customer-password-test"})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Hour); time.Now().Before(deadline); {
		if _, err := os.Stat(ready); os.IsNotExist(err) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
