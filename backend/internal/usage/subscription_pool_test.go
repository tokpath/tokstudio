package usage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

type subscriptionTestRuntime func(context.Context, modelruntime.Attempt, modelruntime.Request, modelruntime.Emit) (modelruntime.Result, error)

func (f subscriptionTestRuntime) Execute(ctx context.Context, a modelruntime.Attempt, r modelruntime.Request, emit modelruntime.Emit) (modelruntime.Result, error) {
	return f(ctx, a, r, emit)
}

func TestPostgresSubscriptionPoolRetriesAndSettlesOnce(t *testing.T) {
	for _, test := range []struct {
		name       string
		fallback   bool
		rateLimit  bool
		authFailed bool
	}{
		{name: "rotate within pool"},
		{name: "fall back to metered supplier", fallback: true, rateLimit: true},
		{name: "release when all accounts are rate limited", rateLimit: true},
		{name: "release when every authorization fails before dispatch", authFailed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := setup(t, "100", nil)
			pool, err := f.cat.SaveSubscriptionPool(f.ctx, f.operator, catalog.SubscriptionPoolInput{
				Name: "Claude pool", Kind: subscriptionauth.Claude, Enabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{"sk-ant-oat-mock-one", "sk-ant-oat-mock-two"} {
				if _, err = f.cat.SaveSubscriptionAccount(f.ctx, f.operator, catalog.SubscriptionAccountInput{
					PoolID: pool, Name: "Test account", Credential: token, Enabled: true,
				}); err != nil {
					t.Fatal(err)
				}
			}
			upstream := catalog.UpstreamModel{
				ID: "subscription", PublicModelID: "public-model", Identity: "vendor/test@1",
				ProviderID: pool, Provider: "anthropic", Model: "test-chat", CredentialID: pool,
				SubscriptionKind: subscriptionauth.Claude, SubscriptionInputLimit: 10000, SubscriptionOutputLimit: 2000,
				Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}, Enabled: true, Healthy: true, Priority: -1,
			}
			if err = f.cat.SaveUpstream(f.ctx, f.operator, upstream, 0); err != nil {
				t.Fatal(err)
			}
			if !test.fallback {
				if err = f.db.Exec(`UPDATE tokenhub_upstreams SET definition=jsonb_set(definition,'{enabled}','false') WHERE id='upstream'`).Error; err != nil {
					t.Fatal(err)
				}
			}
			var calls []modelruntime.Attempt
			retryAt := time.Now().Add(2 * time.Minute)
			f.s.Runtime = subscriptionTestRuntime(func(ctx context.Context, a modelruntime.Attempt, r modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
				calls = append(calls, a)
				if a.SubscriptionKind != "" {
					if _, err := f.cat.ResolveSubscriptionCredential(ctx, a, r.Operation); err != nil {
						t.Fatal("leased subscription credential could not be resolved", err)
					}
					if a.CostSnapshot != nil {
						t.Fatal("subscription used a per-token supplier cost snapshot")
					}
					if test.authFailed {
						return modelruntime.Result{State: "not_dispatched"}, modelruntime.ErrSubscriptionReauth
					}
					if len(calls) == 1 || test.rateLimit {
						return modelruntime.Result{State: "failed", ProviderStatus: 429, SubscriptionRetryAt: &retryAt}, &modelruntime.UpstreamFailure{Status: 429}
					}
					return modelruntime.Result{State: "completed", Usage: measured("12", "5")}, nil
				}
				return modelruntime.Result{State: "completed", Usage: measured("12", "5"), Cost: &modelruntime.CostObservation{
					Source: "provider_reported", ReportedCost: &modelruntime.Money{Currency: "USD", Amount: "0.02"},
				}}, nil
			})
			ex, err := f.s.Execute(f.ctx, f.key.Key, "subscription-retry", request(t, false), nil)
			wantStatus, wantCharge, wantBalance := "completed", "0.017", "99.983"
			if !test.fallback && (test.rateLimit || test.authFailed) {
				wantStatus, wantCharge, wantBalance = "failed", "0", "100"
				if test.authFailed && !errors.Is(err, modelruntime.ErrSubscriptionReauth) || test.rateLimit && err == nil {
					t.Fatalf("expected the supplier rejection, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if ex.Request.Status != wantStatus || ex.Request.ChargedATG.String() != wantCharge {
				t.Fatalf("settlement state=%s charge=%s err=%v", ex.Request.Status, ex.Request.ChargedATG, err)
			}
			want := 2
			if test.fallback {
				want = 3
			}
			if len(calls) != want || calls[0].SubscriptionAccountID == "" || calls[1].SubscriptionAccountID == "" || calls[0].SubscriptionAccountID == calls[1].SubscriptionAccountID {
				t.Fatalf("expected distinct accounts and the configured fallback in %d attempts", want)
			}
			if test.fallback && calls[2].SubscriptionKind != "" {
				t.Fatal("failed to fall back to the metered supplier")
			}
			var attempts []AttemptRecord
			if err = f.db.Where("request_id=?", ex.Request.ID).Order("started_at,id").Find(&attempts).Error; err != nil || len(attempts) != want {
				t.Fatal("missing attempt records", err)
			}
			for _, row := range attempts[:2] {
				var a modelruntime.Attempt
				if json.Unmarshal(row.Definition, &a) != nil || a.SubscriptionAccountID == "" || row.CostStatus != "subscription" || row.ReferenceCostAmount != nil || row.ReportedCostAmount != nil {
					t.Fatal("subscription attempt was counted as a per-token supplier cost")
				}
			}
			if test.fallback && (attempts[2].CostStatus != "confirmed" || attempts[2].ReportedCostAmount == nil || *attempts[2].ReportedCostAmount != "0.02") {
				t.Fatal("metered fallback cost was lost")
			}
			pools, err := f.cat.SubscriptionPools(f.ctx, f.operator)
			wantAccountStatus := "cooldown"
			if test.authFailed {
				wantAccountStatus = "reauth_required"
			}
			if err != nil || len(pools) != 1 || pools[0].Accounts[0].Status != wantAccountStatus && pools[0].Accounts[1].Status != wantAccountStatus {
				t.Fatal("rejected account status was not updated", err)
			}
			grants, err := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
			if err != nil || len(grants) != 1 {
				t.Fatal("missing test grant", err)
			}
			account, err := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
			if err != nil || account.Balance.String() != wantBalance || !account.Held.IsZero() {
				t.Fatal("duplicate charge or unreleased hold", err)
			}
		})
	}
}

func TestPostgresSubscriptionTextBridgesReuseRouteAndSettleOnce(t *testing.T) {
	f := setup(t, "100", nil, modelruntime.DefaultBilling(modelruntime.Responses).Capability(modelruntime.Responses, false))
	f.s.ResultStore = imageTestStore{}
	connection := subscriptionauth.Connection{InferenceURL: "https://approved-grok.example/v1/responses"}
	f.cat.SubscriptionConnections = subscriptionauth.Connections{subscriptionauth.Grok: connection}
	f.s.Catalog = f.cat
	pool, err := f.cat.SaveSubscriptionPool(f.ctx, f.operator, catalog.SubscriptionPoolInput{
		Name: "Grok pool", Kind: subscriptionauth.Grok, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.cat.SaveAuthorizedSubscriptionAccount(f.ctx, f.operator, subscriptionauth.Grok,
		catalog.SubscriptionAccountInput{PoolID: pool, Enabled: true},
		subscriptionauth.Credential{AccountID: "native-grok-account", AccessToken: "test-token", Connection: connection.Fingerprint(), ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.SaveAuthorizedSubscriptionAccount(f.ctx, f.operator, subscriptionauth.Grok,
		catalog.SubscriptionAccountInput{PoolID: pool, Enabled: true},
		subscriptionauth.Credential{AccountID: "older-account", AccessToken: "older-token", Connection: connection.Fingerprint(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`UPDATE tokenhub_subscription_accounts
		SET discovered_models='["verified-grok-model"]'::jsonb,last_selected_at=clock_timestamp() WHERE id=?`, account).Error; err != nil {
		t.Fatal(err)
	}
	upstream := catalog.UpstreamModel{
		ID: "grok-responses", PublicModelID: "public-model", Identity: "vendor/test@1",
		ProviderID: pool, Provider: "openai", Model: "verified-grok-model", CredentialID: pool,
		SubscriptionKind: subscriptionauth.Grok, SubscriptionInputLimit: 4096, SubscriptionOutputLimit: 30,
		Capabilities: []modelruntime.Capability{{Operation: modelruntime.Responses}}, Enabled: true, Healthy: true, Priority: 10,
	}
	if err := f.cat.SaveUpstream(f.ctx, f.operator, upstream, 0); err != nil {
		t.Fatal(err)
	}
	var calls int
	var selected modelruntime.Operation
	f.s.Runtime = subscriptionTestRuntime(func(ctx context.Context, a modelruntime.Attempt, r modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
		calls++
		auth, err := f.cat.ResolveSubscriptionCredential(ctx, a, r.Operation)
		if err != nil || auth.Endpoint != connection.InferenceURL || a.SubscriptionAccountID != account || a.CostSnapshot != nil ||
			a.Capability.Operation != modelruntime.Responses || r.Operation != modelruntime.Responses || r.ClientProtocol != selected {
			t.Fatal("compatibility endpoint bypassed the original subscription Responses route", err)
		}
		return modelruntime.Result{State: "completed", Output: json.RawMessage(`{"model":"public-model"}`), Usage: measured("12", "5")}, nil
	})
	for _, sample := range []struct {
		protocol modelruntime.Operation
		body     string
	}{
		{modelruntime.Chat, `{"model":"public-model","max_tokens":20,"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi"},{"role":"user","content":"Bye"}]}`},
		{modelruntime.Messages, `{"model":"public-model","max_tokens":20,"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi"},{"role":"user","content":"Bye"}]}`},
	} {
		selected = sample.protocol
		input, err := modelruntime.DecodeProtocol("incoming", sample.protocol, []byte(sample.body))
		if err != nil {
			t.Fatal(err)
		}
		key := string(sample.protocol)
		first, err := f.s.Execute(f.ctx, f.key.Key, key, input, nil)
		if err != nil || first.Request.Status != "completed" || first.Request.Operation != modelruntime.Responses || first.Request.ChargedATG.String() != "0.017" {
			t.Fatalf("bridge %s state=%s charge=%s err=%v", sample.protocol, first.Request.Status, first.Request.ChargedATG, err)
		}
		retry, err := modelruntime.DecodeProtocol("retry", sample.protocol, []byte(sample.body))
		if err != nil {
			t.Fatal(err)
		}
		replay, err := f.s.Execute(f.ctx, f.key.Key, key, retry, nil)
		if err != nil || !replay.Replay || replay.Request.ID != first.Request.ID {
			t.Fatalf("replay changed the admitted request %s: %v", sample.protocol, err)
		}
	}
	if calls != 2 {
		t.Fatalf("expected exactly two paid requests, got %d", calls)
	}
}
