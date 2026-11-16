package portal

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func readAcceptanceCSV(t *testing.T, f *testApp, path, locale string) ([]string, []map[string]string) {
	t.Helper()
	req, err := http.NewRequest("GET", f.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Language", locale)
	response, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("CSV download: %d %v %s", response.StatusCode, err, data)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || len(rows) == 0 {
		t.Fatal("invalid CSV", rows, err)
	}
	var records []map[string]string
	for _, row := range rows[1:] {
		record := map[string]string{}
		for i, value := range row {
			record[rows[0][i]] = value
		}
		records = append(records, record)
	}
	return rows[0], records
}

func TestPostgresTakeoverPreservesHistoricalExportsAndPendingNotifications(t *testing.T) {
	s := setupExitAcceptance(t)
	a, ctx := s.f.a, t.Context()
	owner, platform := a.Installation.OwnerStaffID, a.Installation.OperatorID
	if err := a.DB.Exec("UPDATE organization_customers SET display_name=? WHERE id=?", "接管测试客户", s.customer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := a.Incentives.SetPolicy(ctx, nil, owner, s.channel, incentives.PolicyInput{ExpectedVersion: 1, ThresholdMoney: ledger.Zero(), DirectRate: ledger.MustDecimal("0.1"), RewardValiditySeconds: 86400}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Incentives.GetQualification(ctx, nil, s.channel, s.customer); err != nil {
		t.Fatal(err)
	}
	policy, err := a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, s.channel, incentives.WithdrawalPolicy{Wallet: "balance", MethodID: uuid.NewString(), MethodName: "Synthetic cash", PayoutKind: "bank_cn", Enabled: true, MinimumMoney: ledger.FromInt64(1)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	state, err := a.Organization.GetBusinessStatus(ctx, nil, s.channel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: s.channel, Feature: organization.Settlement, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "synthetic payout readiness"}, exitReadiness{}); err != nil {
		t.Fatal(err)
	}
	w, err := a.Incentives.RequestWithdrawal(ctx, nil, incentives.WithdrawalInput{Wallet: "balance", OperatorID: s.channel, CustomerID: s.customer, BusinessKey: "late-notification", PolicyID: policy.ID, RecipientReference: "synthetic recipient", AmountMoney: ledger.FromInt64(5)})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Incentives.CancelWithdrawal(ctx, nil, s.customer, w.ID); err != nil {
		t.Fatal(err)
	}
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	before, err := a.Commerce.FinancialFacts(ctx, nil, s.channel, from, to)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Remediation.BeginTakeover(ctx, remediation.TakeoverInput{ActorID: owner, ChannelID: s.channel, BrandOperatorID: s.oem, BusinessKey: "acceptance-seams", Evidence: "isolated regression"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = a.Remediation.RefreshTakeover(ctx, owner, s.oem, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Remediation.ExecuteTakeover(ctx, owner, s.oem, p.ID, p.SnapshotFingerprint); err != nil {
		t.Fatal(err)
	}

	t.Run("late-notices-follow-customer-and-failures-retry-independently", func(t *testing.T) {
		// An older malformed event and a forged cross-brand recipient must neither
		// leak data nor block legitimate notices for this customer or the platform.
		for _, event := range []incentives.Event{
			{ID: uuid.NewString(), OperatorID: s.channel, AggregateID: uuid.NewString(), Kind: "qualification.acquired", Payload: json.RawMessage(`{}`), CreatedAt: time.Now().Add(-time.Hour)},
			{ID: uuid.NewString(), OperatorID: platform, AggregateID: s.customer, Kind: "qualification.acquired", Payload: json.RawMessage(`{}`), CreatedAt: time.Now().Add(-time.Hour)},
		} {
			if err := a.DB.Create(&event).Error; err != nil {
				t.Fatal(err)
			}
		}
		if _, err := a.Incentives.SetPolicy(ctx, nil, owner, platform, incentives.PolicyInput{ThresholdMoney: ledger.FromInt64(100), RewardValiditySeconds: 86400}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := a.queueIncentiveNotifications(ctx); err != nil {
				t.Fatal(err)
			}
			if err := a.runPortalJobs(ctx, "portal-incentive-notifications", "acceptance-worker", 100); err != nil {
				t.Fatal(err)
			}
		}
		notices, err := a.Operations.ListNotifications(ctx, s.customer, s.oem, nil, 100)
		if err != nil {
			t.Fatal(err)
		}
		titles := map[string]int{}
		for _, notice := range notices {
			titles[notice.Title]++
		}
		for _, title := range []string{"推广资格已取得", "余额提现已申请", "余额提现已取消"} {
			if titles[title] != 1 {
				t.Fatalf("missing or duplicated transferred notice %s: %v", title, titles)
			}
		}
		var misplaced, pending, healthy int64
		if err := a.DB.Model(&operations.Notification{}).Where("recipient_id=? AND operator_id<>? AND business_key LIKE 'incentive:%'", s.customer, s.oem).Count(&misplaced).Error; err != nil || misplaced != 0 {
			t.Fatal("notification escaped customer scope", misplaced, err)
		}
		if err := a.DB.Model(&incentives.Event{}).Where("published_at IS NULL").Count(&pending).Error; err != nil || pending != 2 {
			t.Fatal("valid events stuck behind failed events", pending, err)
		}
		if err := a.DB.Model(&operations.Notification{}).Where("operator_id=? AND title=?", platform, "推广经营规则已更新").Count(&healthy).Error; err != nil || healthy != 1 {
			t.Fatal("another brand's notice was blocked", healthy, err)
		}
		var jobs int64
		if err := a.DB.Table("durable_jobs").Where("queue='portal-incentive-notifications' AND status='pending' AND attempts>=1").Count(&jobs).Error; err != nil || jobs != 2 {
			t.Fatal("failed events lost their independent retry", jobs, err)
		}
	})

	t.Run("historical-order-csv-stays-with-original-seller", func(t *testing.T) {
		after, err := a.Commerce.FinancialFacts(ctx, nil, s.channel, from, to)
		if err != nil || after.SalesMoney.Cmp(before.SalesMoney) != 0 || after.SalesMoney.String() != "100" {
			t.Fatal("takeover rewrote sales facts", before, after, err)
		}
		s.f.login(t, "staff", "owner@example.test", "owner-password-test")
		for _, operator := range []string{s.channel, s.oem, platform} {
			base := "/v1/operators/" + operator + "/exports"
			status, out, _ := s.f.call(t, "POST", base, map[string]any{"kind": "orders", "from": from, "to": to}, "")
			mustOK(t, status, out)
			completed, err := a.Operations.RunExports(ctx, 10)
			if err != nil || len(completed) != 1 || completed[0].State != "ready" {
				t.Fatal("export failed", completed, err)
			}
			_, rows := readAcceptanceCSV(t, s.f, base+"/"+out["id"].(string)+"/download", "zh")
			if operator != s.channel {
				if len(rows) != 0 {
					t.Fatal("historical transactions attributed to receiving or parent operator", operator, rows)
				}
				continue
			}
			if len(rows) != 3 {
				t.Fatal("original channel lost historical orders", rows)
			}
			for _, row := range rows {
				if row["实际经营方"] != "星河渠道" || row["当前服务方"] != "星河品牌" || row["客户"] != "接管测试客户" {
					t.Fatal("historical parties not readable", row)
				}
			}
		}
		s.f.login(t, "customer", s.email, "customer-password-test")
		status, order, _ := s.f.call(t, "GET", "/v1/me/orders/"+s.order, nil, "")
		mustOK(t, status, order)
		_, rows := readAcceptanceCSV(t, s.f, "/v1/me/statement?from="+from.UTC().Format(time.RFC3339)+"&to="+to.UTC().Format(time.RFC3339), "zh")
		for _, row := range rows {
			if row["账户类型"] == "客户 ATG 权益" && row["套餐名称"] == "" {
				t.Fatal("takeover lost the statement's original package identity", row)
			}
		}
	})
}

func TestPostgresCustomerStatementIdentifiesEachBalanceAndLocalizesKinds(t *testing.T) {
	f, customer, _ := pooledInvoiceFixture(t)
	ctx, op := t.Context(), f.a.Installation.OperatorID
	accounts := map[string]ledger.Account{}
	for _, purpose := range []string{"commission:frozen", "commission:available", "control:commissions"} {
		account, err := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: purpose, Unit: ledger.MoneyUnit, AllowNegative: strings.HasPrefix(purpose, "control:")})
		if err != nil {
			t.Fatal(err)
		}
		accounts[purpose] = account
	}
	for _, posting := range []ledger.Posting{
		{OperatorID: op, BusinessKey: uuid.NewString(), Kind: "commission_accrual", SourceID: uuid.NewString(), ActorID: "system:fixture", Reason: "synthetic commission", Lines: []ledger.Line{{AccountID: accounts["commission:frozen"].ID, Amount: ledger.FromInt64(7)}, {AccountID: accounts["control:commissions"].ID, Amount: ledger.FromInt64(-7)}}},
		{OperatorID: op, BusinessKey: uuid.NewString(), Kind: "commission_release", SourceID: uuid.NewString(), ActorID: "system:fixture", Reason: "synthetic release", Lines: []ledger.Line{{AccountID: accounts["commission:frozen"].ID, Amount: ledger.FromInt64(-7)}, {AccountID: accounts["commission:available"].ID, Amount: ledger.FromInt64(7)}}},
	} {
		if _, err := f.a.Ledger.Post(ctx, nil, posting); err != nil {
			t.Fatal(err)
		}
	}
	f.login(t, "customer", "pooled-invoice@example.test", "customer-password-test")
	path := "/v1/me/statement?from=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + "&to=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		tr := func(s string) string { return httpx.LocalizedText(locale, s) }
		head, rows := readAcceptanceCSV(t, f, path, locale)
		for _, column := range head {
			if column == tr("变动后余额") {
				t.Fatal("statement still implies an aggregate balance", head)
			}
		}
		grantAccounts, types := map[string]bool{}, map[string]bool{}
		for _, row := range rows {
			account, kind := row[tr("账户类型")], row[tr("账务事项")]
			types[account] = true
			if kind == "grant" || kind == "commission_release" || kind == "commission_accrual" {
				t.Fatal("raw system kind in customer statement", locale, row)
			}
			if account == tr("客户 ATG 权益") {
				if row[tr("该账户变动后余额")] != "50" || kind != tr("权益发放") || (row[tr("套餐名称")] != "Purchase A" && row[tr("套餐名称")] != "Purchase B") {
					t.Fatal("grant account's amount or identity changed", locale, row)
				}
				grantAccounts[row[tr("账户编号")]] = true
			}
		}
		if len(grantAccounts) != 2 || grantAccounts[""] || !types[tr("支付余额")] || !types[tr("可用佣金")] || !types[tr("冻结佣金")] {
			t.Fatal("independent balances are not distinguishable", locale, grantAccounts, types)
		}
	}
	f.registered(t, "other-statement@example.test")
	f.login(t, "customer", "other-statement@example.test", "customer-password-test")
	_, rows := readAcceptanceCSV(t, f, path, "zh")
	if len(rows) != 0 {
		t.Fatal("another customer read the statement", rows)
	}
}
