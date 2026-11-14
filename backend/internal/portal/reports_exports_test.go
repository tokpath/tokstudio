package portal

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
)

func TestSimpleReportsAndCSVExports(t *testing.T) {
	s := stockSetup(t)
	status, order := s.buy(t, 1000)
	mustOK(t, status, order)
	s.fund(t, order["id"].(string), 1000)
	base := "/v1/operators/" + s.oem
	from, to := time.Now().Add(-time.Hour).UTC(), time.Now().Add(time.Minute).UTC()
	status, report, _ := s.f.call(t, "GET", base+"/reports/financial?from="+from.Format(time.RFC3339Nano)+"&to="+to.Format(time.RFC3339Nano), nil, "")
	mustOK(t, status, report)
	if report["procurement"].(map[string]any)["wholesale_sales_money"] != "1000" {
		t.Fatal(report)
	}
	if cash := report["cash"].(map[string]any); cash["recorded_in_money"] != "1000" || cash["closing_recorded_net_money"] != "1000" {
		t.Fatalf("same-currency receipt missing from report: %v", cash)
	}
	var first string
	for _, kind := range []string{"financial", "orders", "payments", "requests", "procurement", "ledger"} {
		input := map[string]any{"kind": kind, "from": from, "to": to}
		key := uuid.NewString()
		status, out, _ := s.f.call(t, "POST", base+"/exports", input, key)
		mustOK(t, status, out)
		status, repeat, _ := s.f.call(t, "POST", base+"/exports", input, key)
		mustOK(t, status, repeat)
		if repeat["id"] != out["id"] {
			t.Fatal("retry duplicated export", repeat, out)
		}
		id := out["id"].(string)
		if first == "" {
			first = id
		}
		jobs, err := s.f.a.Operations.RunExports(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 || jobs[0].State != "ready" {
			t.Fatalf("%s export: %#v", kind, jobs)
		}
		resp, err := s.f.client.Get(s.f.server.URL + base + "/exports/" + id + "/download")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(resp.Header.Get("Content-Disposition"), ".csv") {
			t.Fatal(resp.StatusCode, resp.Header, string(raw))
		}
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff"))).ReadAll()
		if err != nil || len(rows)-1 != jobs[0].RowCount {
			t.Fatal(kind, rows, err)
		}
		if kind == "payments" && (!strings.Contains(string(raw), "OEM3-1 channel") || strings.Contains(string(raw), "channel bank")) {
			t.Fatal("receipt must show name without bank proof", string(raw))
		}
		if kind == "procurement" && (!strings.Contains(string(raw), "Third OEM") || !strings.Contains(string(raw), "OEM3-1 channel")) {
			t.Fatal(string(raw))
		}
		if kind == "financial" && (!strings.Contains(string(raw), "批发销售额,1000,USD") || !strings.Contains(string(raw), "资金收支,已核实流入,1000,USD")) {
			t.Fatal(string(raw))
		}
		if kind == "orders" && len(rows) != 1 {
			t.Fatal("wholesale must remain separate from retail orders", rows)
		}
	}
	var count int64
	s.f.a.DB.Model(&operations.Export{}).Where("operator_id=?", s.oem).Count(&count)
	if count != 6 {
		t.Fatal("duplicate export jobs", count)
	}
	for _, input := range []map[string]any{{"kind": "usage", "from": from, "to": to}, {"kind": "ledger", "from": from, "to": to, "owner_id": s.channel}} {
		status, out, _ := s.f.call(t, "POST", base+"/exports", input, "")
		if status != 400 {
			t.Fatal(status, out)
		}
	}
	status, out, _ := s.f.call(t, "GET", "/v1/operators/"+s.channel+"/exports/"+first+"/download", nil, "")
	if status != 404 {
		t.Fatal("cross-operator export leaked", status, out)
	}
	if err := s.f.a.DB.Model(&operations.Export{}).Where("id=?", first).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	status, list, _ := s.f.call(t, "GET", base+"/exports", nil, "")
	mustOK(t, status, list)
	for _, raw := range list["items"].([]any) {
		row := raw.(map[string]any)
		if row["id"] == first {
			if row["status"] != "expired" || row["values"].(map[string]any)["download_url"] != nil {
				t.Fatal("expired file offered download", row)
			}
		}
	}
}

func TestProcurementReportingAndReconciliationUseTheSamePayment(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, t.Context()
	owner := f.a.Installation.OwnerStaffID
	status, purchase := s.buy(t, 1000)
	mustOK(t, status, purchase)
	id := purchase["id"].(string)
	now := time.Now().UTC()
	paidAt := now.Add(-time.Hour)
	split := now.Add(-time.Minute)
	gross := ledger.FromInt64(1000)
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: gross, FeeStatus: "pending", Currency: "USD", OriginalAmount: "1000", ValuationEvidence: "verified fixture", PaidAt: &paidAt, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	status, order, _ := f.call(t, "POST", "/v1/operators/"+s.oem+"/procurement/"+id+"/fund", map[string]string{"receipt_id": receipt.ID}, "")
	mustOK(t, status, order)
	seller, err := f.a.Operations.FinancialReport(ctx, owner, s.oem, paidAt.Add(-time.Minute), split)
	if err != nil || seller.Cash.RecordedInMoney.Cmp(gross) != 0 {
		t.Fatalf("seller cash at payment: %+v %v", seller.Cash, err)
	}
	buyer, err := f.a.Operations.FinancialReport(ctx, owner, s.channel, paidAt.Add(-time.Minute), split)
	if err != nil || buyer.Cash.ProcurementPaymentsMoney.Cmp(gross) != 0 {
		t.Fatalf("buyer cash at payment: %+v %v", buyer.Cash, err)
	}
	buyer, err = f.a.Operations.FinancialReport(ctx, owner, s.channel, split, now.Add(time.Minute))
	if err != nil || !buyer.Cash.ProcurementPaymentsMoney.IsZero() {
		t.Fatalf("late posting counted twice: %+v %v", buyer.Cash, err)
	}
	for _, operator := range []string{s.oem, s.channel} {
		clean, err := f.a.Operations.RunReconciliation(ctx, owner, operator)
		if err != nil || clean.FindingCount != 0 {
			t.Fatalf("clean supply pair: %+v %v", clean, err)
		}
	}
	if err = f.a.DB.Exec("UPDATE cooperation_purchase_orders SET allocated_money=0 WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	discrepancies := make(map[string]string)
	for _, operator := range []string{s.oem, s.channel} {
		for range 2 {
			run, err := f.a.Operations.RunReconciliation(ctx, owner, operator)
			if err != nil || run.FindingCount != 1 {
				t.Fatalf("unreported supply mismatch: %+v %v", run, err)
			}
		}
		rows, next, err := f.a.Operations.ListDiscrepancies(ctx, owner, operator, "open", "", 1)
		if err != nil || len(rows) != 1 || next != "" || rows[0].Reference != id || rows[0].Expected.Cmp(gross) != 0 || !rows[0].Actual.IsZero() {
			t.Fatalf("wrong supply mismatch or duplicate: %+v %s %v", rows, next, err)
		}
		discrepancies[operator] = rows[0].ID
		if err := f.a.Operations.ResolveDiscrepancy(ctx, owner, operator, rows[0].ID, receipt.ID, "verified receipt"); !errors.Is(err, operations.ErrConflict) {
			t.Fatalf("a receipt resolved a still-open mismatch: %v", err)
		}
	}
	if err = f.a.DB.Exec("UPDATE cooperation_purchase_orders SET allocated_money=? WHERE id=?", gross, id).Error; err != nil {
		t.Fatal(err)
	}
	for operator, discrepancy := range discrepancies {
		if err = f.a.Operations.ResolveDiscrepancy(ctx, owner, operator, discrepancy, uuid.NewString(), "unknown record"); !errors.Is(err, operations.ErrInvalid) {
			t.Fatalf("unknown reference accepted: %v", err)
		}
		if err = f.a.Operations.ResolveDiscrepancy(ctx, owner, operator, discrepancy, receipt.ID, "checked the verified receipt after repair"); err != nil {
			t.Fatal(err)
		}
		rows, _, err := f.a.Operations.ListDiscrepancies(ctx, owner, operator, "resolved", "", 1)
		if err != nil || len(rows) != 1 || rows[0].ID != discrepancy || rows[0].AdjustmentReference == nil || *rows[0].AdjustmentReference != receipt.ID {
			t.Fatalf("resolution did not preserve the evidence: %+v %v", rows, err)
		}
	}
}

func TestExternalReportsAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_REPORTS_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("explicit isolated report browser acceptance only")
	}
	s := stockSetup(t)
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	s.fund(t, out["id"].(string), 1000)
	s.f.server.Close()
	s.f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	s.f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	s.f.a.Config.PublicBaseURL = s.f.a.Config.WebOrigin
	listener, err := net.Listen("tcp", "0.0.0.0:18081")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(s.f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	s.f.server = server
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]string{"url": server.URL, "operator_id": s.oem, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if err = os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(ready); os.IsNotExist(err) {
			return
		}
		if _, err = s.f.a.Operations.RunExports(context.Background(), 5); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
