package portal

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

// Opt-in browser fixture: an actual application and isolated PostgreSQL schema,
// using the separately deployed model/payment protocol service. No production
// database, real provider credential or external money transfer is involved.
// Delete the ready file to stop, or let the bounded lifetime expire. The normal
// regression does not claim this manually driven server as an acceptance test.
func TestExternalFrontendAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_FRONTEND_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("requires explicit frontend acceptance ready file")
	}
	lab := os.Getenv("TOKENHUB_MODEL_MOCK_URL")
	if lab == "" {
		t.Fatal("independent protocol service required")
	}
	f := setupPortal(t)
	f.server.Close()
	// The local Next proxy sends the original browser Host. Trust only its
	// loopback source, exactly as a configured ingress is trusted in deployment.
	f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	listener, err := net.Listen("tcp", "0.0.0.0:18081")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	server.URL = "http://127.0.0.1:18081"
	t.Cleanup(server.Close)
	f.server = server
	f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	f.a.Config.PublicBaseURL = "http://127.0.0.1:13003"
	f.openRegistration(t)
	f.registered(t, "customer@frontend.test")
	model := f.configureExternalCatalog(t, lab)
	ctx := context.Background()
	op := f.a.Installation.OperatorID
	owner := f.a.Installation.OwnerStaffID
	manual, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "合成测试收款账户", "instructions": "仅限隔离联调：登记合成到账事实后交付，不要向任何真实账户付款。"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	stripe, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "stripe", Enabled: true, APIBaseURL: lab, Credentials: map[string]string{"secret_key": "sk_test_frontend_" + uuid.NewString(), "publishable_key": "pk_test_frontend", "webhook_secret": "whsec_integration_test", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range []organization.BusinessFeature{organization.NewPurchase, organization.ModelUsage} {
		state, e := f.a.Organization.GetBusinessStatus(ctx, nil, op)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: feature, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "isolated browser acceptance against independent protocol service"}, f.a); e != nil {
			t.Fatal(e)
		}
	}
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "联调渠道", LegalName: "合成渠道主体"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(ready), 0700); err != nil {
		t.Fatal(err)
	}
	var personas map[string]any
	simulation := os.Getenv("TOKENHUB_SIMULATION") == "1"
	if simulation {
		personas = map[string]any{}
		personas["customers"] = f.seedCustomerSimulation(t, lab)
		personas["operators"] = f.seedOperatorSimulation(t, lab)
		personas["platform"] = f.seedPlatformSimulation(t, lab)
		personas["finance"] = f.seedFinanceSimulation(t)
	}
	mailboxFile := filepath.Join(filepath.Dir(ready), "mailbox.json")
	writeMailbox := func() {
		f.box.mu.Lock()
		raw, err := json.Marshal(f.box.messages)
		f.box.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(mailboxFile+".tmp", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(mailboxFile+".tmp", mailboxFile); err != nil {
			t.Fatal(err)
		}
	}
	writeMailbox()
	data, _ := json.MarshalIndent(map[string]any{"api_url": server.URL, "web_url": f.a.Config.WebOrigin, "operator_id": op, "model_id": model, "manual_method_id": manual.ID, "stripe_method_id": stripe.ID, "channel_id": child.Operator.ID, "owner_email": "owner@example.test", "owner_password": "owner-password-test", "customer_email": "customer@frontend.test", "customer_password": "customer-password-test", "started_at": time.Now(), "personas": personas, "mailbox_file": mailboxFile}, "", "  ")
	if err = os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Isolated acceptance application ready; remove the explicitly supplied ready file to finish")
	if simulation {
		workerCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			f.a.RunWorker(workerCtx, 2, func(kind string, err error) { t.Log("simulation worker", kind, err) })
		}()
		defer func() { cancel(); <-done }()
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	deadline := time.NewTimer(90 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			return
		case <-tick.C:
			if _, e := os.Stat(ready); os.IsNotExist(e) {
				return
			}
			writeMailbox()
			if !simulation {
				if e := f.a.RunDeliveryJobs(ctx, "frontend-acceptance", 20); e != nil {
					t.Log("delivery worker:", e)
				}
			}
		}
	}
}
