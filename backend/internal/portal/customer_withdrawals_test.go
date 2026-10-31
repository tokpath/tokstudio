package portal

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresCustomerWithdrawalCancellationOnlyBeforeReview(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "withdrawal-actions@example.test")
	f.login(t, "customer", "withdrawal-actions@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op := f.a.Installation.OperatorID
	policy, err := f.a.Incentives.SetWithdrawalPolicy(context.Background(), nil, f.a.Installation.OwnerStaffID, op, incentives.WithdrawalPolicy{PayoutKind: "bank_cn", MinimumMoney: ledger.MustDecimal("1"), MethodReference: "synthetic reviewed bank rules"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var requestedID, commissionAccount string
	// Persist each legal workflow state for the projection. The requested row
	// has a real ledger hold against an explicit synthetic opening balance, so
	// the HTTP cancellation also proves funds are released exactly once.
	for _, state := range []string{"requested", "approved", "paid", "cancelled", "failed"} {
		hold := ledger.Hold{ID: uuid.NewString(), OperatorID: op, BusinessKey: "withdrawal-view:" + state, Fingerprint: "synthetic fixture", Purpose: "commission_withdrawal", Status: "active", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
		if state == "requested" {
			ctx := context.Background()
			account, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: "commission:available", Unit: ledger.MoneyUnit})
			if err != nil {
				t.Fatal(err)
			}
			commissionAccount = account.ID
			control, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: op, Purpose: "control:withdrawal-fixture", Unit: ledger.MoneyUnit, AllowNegative: true})
			if err != nil {
				t.Fatal(err)
			}
			amount := ledger.MustDecimal("5.000000000000000001")
			if _, err := f.a.Ledger.Post(ctx, nil, ledger.Posting{OperatorID: op, BusinessKey: "synthetic-withdrawal-opening", Kind: "test_fixture", SourceID: uuid.NewString(), ActorID: "test:fixture", Reason: "explicit synthetic opening commission", Lines: []ledger.Line{{AccountID: account.ID, Amount: amount}, {AccountID: control.ID, Amount: amount.Neg()}}}); err != nil {
				t.Fatal(err)
			}
			hold, err = f.a.Ledger.Reserve(ctx, nil, ledger.Reservation{OperatorID: op, BusinessKey: hold.BusinessKey, Purpose: hold.Purpose, ExpiresAt: hold.ExpiresAt, Amounts: []ledger.Line{{AccountID: account.ID, Amount: amount}}})
			if err != nil {
				t.Fatal(err)
			}
		} else if err := f.a.DB.Create(&hold).Error; err != nil {
			t.Fatal(err)
		}
		row := incentives.Withdrawal{MethodID: policy.MethodID, MethodName: policy.MethodName, ID: uuid.NewString(), OperatorID: op, CustomerID: customer, PolicyID: policy.ID, BusinessKey: "withdrawal-view:" + state, Fingerprint: "synthetic fixture", AmountMoney: ledger.MustDecimal("5.000000000000000001"), NetMoney: ledger.MustDecimal("5.000000000000000001"), FeeRemainder: "0", RecipientReference: "synthetic recipient", HoldID: hold.ID, Status: state, CreatedAt: time.Now()}
		if err := f.a.DB.Table("incentives_withdrawals").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if state == "requested" {
			requestedID = row.ID
			if err := f.a.DB.Exec("INSERT INTO incentives_commission_funding_commands(id,operator_id,customer_id,kind,reference,amount_money,attempt,state) VALUES(?,?,?,'withdrawal',?,?,1,'reserved')", uuid.NewString(), op, customer, row.ID, row.AmountMoney).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	status, page, _ := f.call(t, "GET", "/v1/me/withdrawals/workspace", nil, "")
	mustOK(t, status, page)
	var minimum map[string]any
	for _, raw := range page["summary"].([]any) {
		item := raw.(map[string]any)
		if item["label"] == "最低提现额度" {
			minimum = item
		}
	}
	if minimum["value"] != "1" || minimum["unit"] != "MONEY" || !strings.Contains(page["description"].(string), "1 结算币种") {
		t.Fatal("customer withdrawal page omitted the published minimum", minimum, page["description"])
	}
	if len(page["operations"].([]any)) != 0 {
		t.Fatal("customer withdrawal page still publishes a separate fee quote", page["operations"])
	}
	methods := page["payout_methods"].([]any)
	if len(methods) != 1 || methods[0].(map[string]any)["kind"] != "bank_cn" {
		t.Fatal("customer withdrawal page did not publish the configured payout profile", methods)
	}
	if len(page["items"].([]any)) != 5 {
		t.Fatal("missing withdrawal states", page)
	}
	for _, raw := range page["items"].([]any) {
		row := raw.(map[string]any)
		want := []any{}
		if row["status"] == "requested" {
			want = []any{"cancel"}
		}
		if !reflect.DeepEqual(row["allowed_actions"], want) {
			t.Fatalf("unexpected customer action for %s: %v", row["status"], row["allowed_actions"])
		}
	}
	found := false
	for _, raw := range page["row_operations"].([]any) {
		op := raw.(map[string]any)
		if op["name"] != "cancel" {
			continue
		}
		found = true
		if op["method"] != "POST" || op["path"] != "/api/v1/me/withdrawals/{id}/cancel" || len(op["fields"].([]any)) != 0 || !strings.Contains(op["confirmation"].(string), "释放本笔佣金占用") {
			t.Fatal("cancellation lost its exact resource or confirmation", op)
		}
	}
	if !found {
		t.Fatal("requested withdrawal has no cancellation operation")
	}
	cancelPath := "/v1/me/withdrawals/" + requestedID + "/cancel"
	dropped := &dropWithdrawalResponse{base: http.DefaultTransport, path: cancelPath}
	f.client.Transport = dropped
	businessKey := uuid.NewString()
	req, err := http.NewRequest("POST", f.server.URL+cancelPath, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", businessKey)
	req.Header.Set("X-CSRF-Token", f.csrf)
	req.Header.Set("Origin", f.server.URL)
	if response, err := f.client.Do(req); err == nil {
		response.Body.Close()
		t.Fatal("synthetic lost response was not applied")
	}
	if dropped.status != http.StatusOK {
		t.Fatal("first cancellation did not commit before its response was lost", dropped.status)
	}
	status, cancelled, _ := f.call(t, "POST", cancelPath, map[string]any{}, businessKey)
	mustOK(t, status, cancelled)
	if cancelled["id"] != requestedID || cancelled["status"] != "cancelled" {
		t.Fatal("unknown cancellation retry did not resolve the original application", cancelled)
	}
	account, err := f.a.Ledger.GetAccount(context.Background(), commissionAccount)
	if err != nil || !account.Held.IsZero() || account.Balance.String() != "5.000000000000000001" {
		t.Fatal("cancellation did not release precisely its hold without a fee", account, err)
	}
	var events int64
	if err := f.a.DB.Table("incentives_events").Where("aggregate_id=? AND kind='withdrawal.cancelled'", requestedID).Count(&events).Error; err != nil || events != 1 {
		t.Fatal("retry produced a second cancellation event", events, err)
	}
	status, after, _ := f.call(t, "GET", "/v1/me/withdrawals/workspace", nil, "")
	mustOK(t, status, after)
	for _, raw := range after["items"].([]any) {
		row := raw.(map[string]any)
		if row["id"] == requestedID && (row["status"] != "cancelled" || len(row["allowed_actions"].([]any)) != 0) {
			t.Fatal("completed cancellation still offered another cancellation", row)
		}
	}
}

type dropWithdrawalResponse struct {
	base    http.RoundTripper
	path    string
	dropped bool
	status  int
}

func (t *dropWithdrawalResponse) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err != nil || req.URL.Path != t.path || t.dropped {
		return response, err
	}
	t.dropped = true
	t.status = response.StatusCode
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return nil, errors.New("synthetic connection loss after cancellation committed")
}

func TestPostgresCommissionPageSetsWithdrawalMinimumOnly(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + f.a.Installation.OperatorID
	status, page, _ := f.call(t, "GET", base+"/withdrawals", nil, "")
	mustOK(t, status, page)
	for _, raw := range page["operations"].([]any) {
		if raw.(map[string]any)["name"] == "policy" {
			t.Fatal("withdrawal page still publishes fee and approval rules", raw)
		}
	}
	body := map[string]any{"threshold_money": "100", "direct_percent": "15", "indirect_percent": "5", "confirmation_days": 30, "reward_validity_days": 30, "invitee_atg": "0", "inviter_atg": "0", "kol_cross_activity_limit": 0, "minimum_money": "1.5", "expected_version": 0}
	status, out, _ := f.call(t, "POST", base+"/commissions", body, "")
	mustOK(t, status, out)
	policy, err := f.a.Incentives.CurrentWithdrawalPolicy(context.Background(), nil, f.a.Installation.OperatorID)
	if err != nil || policy.MinimumMoney.String() != "1.5" || !policy.FlatFeeMoney.IsZero() || !policy.FeeRate.IsZero() || policy.RequiresDualApproval {
		t.Fatal(policy, err)
	}
	status, page, _ = f.call(t, "GET", base+"/commissions", nil, "")
	mustOK(t, status, page)
	if page["items"].([]any)[0].(map[string]any)["values"].(map[string]any)["minimum_money"] != "1.5" {
		t.Fatal(page["items"])
	}
	status, out, _ = f.call(t, "POST", base+"/withdrawals", map[string]any{"minimum_money": "9"}, "")
	if status != 400 {
		t.Fatal(status, out)
	}
	version := page["items"].([]any)[0].(map[string]any)["version"]
	body["minimum_money"] = "0.999"
	body["expected_version"] = version
	status, out, _ = f.call(t, "POST", base+"/commissions", body, "")
	if status != 400 || out["error"].(map[string]any)["code"] != "invalid_input" || !strings.Contains(out["error"].(map[string]any)["message"].(string), "不能小于 1") {
		t.Fatal(status, out)
	}
	status, page, _ = f.call(t, "GET", base+"/commissions", nil, "")
	mustOK(t, status, page)
	if page["items"].([]any)[0].(map[string]any)["values"].(map[string]any)["minimum_money"] != "1.5" {
		t.Fatal("rejected minimum replaced the published rule", page["items"])
	}
}
