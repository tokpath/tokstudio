package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
)

func TestBifrostFailureStaysPrivateInCustomerAndOperatorRecords(t *testing.T) {
	f := setup(t, "10", nil)
	const marker = "private-supplier-diagnostic"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if key != "integration-test-secret" {
			t.Error("mock did not receive the selected catalog credential")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": marker + " " + key, "code": key, "type": key}})
	}))
	defer server.Close()
	var stored struct{ Definition json.RawMessage }
	if err := f.db.Table("tokenhub_upstreams").Where("id='upstream'").Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	var upstream catalog.UpstreamModel
	if err := json.Unmarshal(stored.Definition, &upstream); err != nil {
		t.Fatal(err)
	}
	upstream.Endpoints = map[modelruntime.Operation]string{modelruntime.Chat: server.URL}
	if err := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 1); err != nil {
		t.Fatal(err)
	}
	runtime, err := modelruntime.NewBifrost(f.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, AllowTestLoopback: true, Credentials: f.cat, EndpointGuard: func(_ context.Context, target string) error {
		if target != server.URL {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	f.s.Runtime = runtime
	out, err := f.s.Execute(f.ctx, f.key.Key, "real-private-error", request(t, false), nil)
	var failure *modelruntime.UpstreamFailure
	if !errors.As(err, &failure) || failure.Status != 401 || out.Request.Reason != "upstream_authentication_failed" || !out.Request.ChargedATG.IsZero() {
		t.Fatal("supplier rejection did not retain safe diagnosis and billing state", out.Request.Reason, err)
	}
	var attempts []AttemptRecord
	if err := f.db.Where("request_id=?", out.Request.ID).Find(&attempts).Error; err != nil || len(attempts) != 1 || attempts[0].ProviderStatus != 401 {
		t.Fatal("safe supplier status was not persisted", err)
	}
	requests, err := f.s.OperationalRequests(f.ctx, nil, f.operator, f.customer, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 10)
	if err != nil || len(requests) != 1 {
		t.Fatal("operator report unavailable", err)
	}
	for _, record := range []any{out.Request, out.Result, attempts, requests} {
		raw, err := json.Marshal(record)
		if err != nil || strings.Contains(string(raw), marker) || strings.Contains(string(raw), "integration-test-secret") {
			t.Fatal("supplier content escaped into customer/operator records", fmt.Sprintf("%T", record))
		}
	}
}

type privateFailureRuntime struct{}

func (privateFailureRuntime) Execute(_ context.Context, attempt modelruntime.Attempt, _ modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
	return modelruntime.Result{AttemptID: attempt.ID, State: "failed", ErrorCode: "opaque-supplier-secret-in-error-code", ProviderStatus: 401}, errors.New("opaque-supplier-secret-in-message")
}

func TestExecutionPersistsOnlySystemFailureCodes(t *testing.T) {
	f := setup(t, "10", nil)
	f.s.Runtime = privateFailureRuntime{}
	out, err := f.s.Execute(f.ctx, f.key.Key, "private-error", request(t, false), nil)
	if err == nil || out.Request.Reason != "model_execution_failed" || out.Result.ErrorCode != "model_execution_failed" {
		t.Fatal("untrusted runtime failure was not safely classified", out.Request.Reason)
	}
	var attempts []AttemptRecord
	if err = f.db.Where("request_id=?", out.Request.ID).Find(&attempts).Error; err != nil || len(attempts) != 1 {
		t.Fatal("missing attempt", err)
	}
	if attempts[0].ErrorCode != "model_execution_failed" || attempts[0].ProviderStatus != 401 {
		t.Fatal("unsafe stored diagnostic", attempts[0].ErrorCode)
	}
	var requestReason string
	if err = f.db.Table("usage_requests").Select("reason").Where("id=?", out.Request.ID).Scan(&requestReason).Error; err != nil || strings.Contains(requestReason, "secret") {
		t.Fatal("customer-visible request retained a supplier secret", err)
	}
	// Writers cannot bypass the persistence boundary by supplying their own code.
	if err = f.s.RecordAttempt(f.ctx, out.Request.ID, attempts[0].ID, modelruntime.Result{State: "failed", ErrorCode: "another-secret", ProviderStatus: 1234567}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.First(&attempts[0], "id=?", attempts[0].ID).Error; err != nil || attempts[0].ErrorCode != "model_execution_failed" || attempts[0].ProviderStatus != 0 {
		t.Fatal("persistence accepted arbitrary diagnostic fields", err)
	}
}

func TestPrivateErrorMigrationRemovesPreviouslyRecordedBodies(t *testing.T) {
	db := dbtest.New(t)
	for _, statement := range []string{
		"CREATE TABLE usage_attempts(provider_status INT, error_code TEXT, provider_error TEXT)",
		"CREATE TABLE usage_requests(reason TEXT)",
		"INSERT INTO usage_attempts VALUES(401, 'upstream_error', 'ordinary-supplier-secret')",
		"INSERT INTO usage_requests VALUES('HTTP 401 ordinary-supplier-secret'), ('input_quantity_unavailable'), ('')",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, migrations := Migrations()
	script, err := fs.ReadFile(migrations, "0009_private_upstream_errors.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(script)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='usage_attempts' AND column_name='provider_error'").Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("raw supplier error storage remains", err)
	}
	var reasons []string
	if err = db.Table("usage_requests").Pluck("reason", &reasons).Error; err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"upstream_error": true, "input_quantity_unavailable": true, "": true}
	for _, reason := range reasons {
		if !want[reason] {
			t.Fatal("historical supplier text remains")
		}
		delete(want, reason)
	}
	if len(want) != 0 {
		t.Fatal("system-owned reasons were lost")
	}
}
