package portal

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresBalanceWithdrawalRequiresDisplayedTermsAndAppearsInReviews(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "withdrawal-integrity@example.test")
	f.login(t, "customer", "withdrawal-integrity@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op, owner, ctx := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID, t.Context()
	// Cash-out deliberately needs no original top-up/refund allocation.
	account, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: "customer:money", Unit: ledger.MoneyUnit})
	if err != nil {
		t.Fatal(err)
	}
	control, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: op, Purpose: "control:test-opening", Unit: ledger.MoneyUnit, AllowNegative: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Ledger.Post(ctx, nil, ledger.Posting{OperatorID: op, BusinessKey: uuid.NewString(), Kind: "test_opening", SourceID: customer, ActorID: "test", Reason: "isolated cash fixture", Lines: []ledger.Line{{AccountID: account.ID, Amount: ledger.FromInt64(100)}, {AccountID: control.ID, Amount: ledger.FromInt64(-100)}}})
	if err != nil {
		t.Fatal(err)
	}
	method, err := f.a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, op, incentives.WithdrawalPolicy{Wallet: "balance", MethodID: uuid.NewString(), MethodName: "Test wallet", PayoutKind: "alipay", Enabled: true, MinimumMoney: ledger.FromInt64(1), FlatFeeMoney: ledger.FromInt64(1)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.a.Organization.GetBusinessStatus(ctx, nil, op)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: organization.Settlement, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "configured test payout method"}, f.a)
	if err != nil {
		t.Fatal(err)
	}
	status, page, _ := f.call(t, "GET", "/v1/me/refunds/workspace", nil, "")
	mustOK(t, status, page)
	shown := page["payout_methods"].([]any)[0].(map[string]any)
	if shown["policy_id"] != method.ID {
		t.Fatal("missing displayed terms", shown)
	}
	method.FlatFeeMoney = ledger.FromInt64(5)
	latest, err := f.a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, op, method, method.Version)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"method_id": method.MethodID, "policy_id": method.ID, "amount_money": "10", "recipient": map[string]string{"payee_name": "Test Person", "account": "test@example.test"}}
	status, rejected, _ := f.call(t, "POST", "/v1/me/balance-withdrawals", body, uuid.NewString())
	if status != 409 || rejected["error"].(map[string]any)["code"] != "withdrawal_terms_changed" {
		t.Fatal(status, rejected)
	}
	var count int64
	f.a.DB.Table("incentives_withdrawals").Where("customer_id=?", customer).Count(&count)
	if count != 0 {
		t.Fatal("stale terms created a withdrawal")
	}
	body["policy_id"] = latest.ID
	key := uuid.NewString()
	var id string
	for range 2 {
		status, out, _ := f.call(t, "POST", "/v1/me/balance-withdrawals", body, key)
		mustOK(t, status, out)
		if id != "" && id != out["id"] {
			t.Fatal("duplicate application", out)
		}
		id = out["id"].(string)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "POST", "/v1/operators/"+op+"/balance-withdrawals/"+id+"/fail", map[string]any{"evidence": "资料与收款账户不一致"}, uuid.NewString())
	mustOK(t, status, out)
	var held ledger.Decimal
	if err := f.a.DB.Table("ledger_accounts").Select("held").Where("id=?", account.ID).Row().Scan(&held); err != nil || !held.IsZero() {
		t.Fatal("rejected withdrawal retained a hold", held, err)
	}
	if _, err := f.a.Incentives.RelayEvents(ctx, nil, 50, operations.IncentiveNotifications{Operations: f.a.Operations, Audience: f.a.Incentives}); err != nil {
		t.Fatal(err)
	}
	var notice operations.Notification
	if err := f.a.DB.Where("recipient_id=? AND title=?", customer, "余额提现已拒绝").First(&notice).Error; err != nil || !strings.Contains(notice.Body, "拒绝理由：资料与收款账户不一致。") {
		t.Fatal(notice.Body, err)
	}
}
