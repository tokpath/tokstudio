package portal

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in real API/browser fixture. Only synthetic failures are created in the
// isolated test schema; recovery runs the normal worker and HTTP command paths.
func TestExternalTaskExceptionsAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_TASK_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated task acceptance server")
	}
	f := setupPortal(t)
	roles := f.seedPaymentOperatorRoles(t)
	op := f.a.Installation.OperatorID
	payload := fmt.Sprintf(`{"operator_id":%q}`, roles[0]["operator"])
	at := time.Now().Add(-time.Hour)
	f.taskFixture(t, "portal-maintenance", "invoice_review", "invoice_review:demo:1", "dead", true, at, payload)
	latest := f.taskFixture(t, "portal-maintenance", "invoice_review", "invoice_review:demo:2", "dead", true, at.Add(10*time.Minute), payload)
	f.taskScheduleFixture(t, "demo", latest, payload)
	f.taskFixture(t, "portal-maintenance", "retention", "cleanup-once", "dead", true, at.Add(20*time.Minute), `{}`)
	f.taskFixture(t, "portal-maintenance", "retention", "retention:recovered:1", "dead", true, at, `{}`)
	done := f.taskFixture(t, "portal-maintenance", "retention", "retention:recovered:2", "completed", false, at.Add(time.Minute), `{}`)
	f.taskScheduleFixture(t, "recovered", done, `{}`)
	f.server.Close()
	f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	f.a.Config.PublicBaseURL = f.a.Config.WebOrigin
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	raw, _ := json.Marshal(map[string]any{"operator": op, "roles": roles, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if err = os.WriteFile(ready, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Hour); time.Now().Before(deadline); {
		if _, err = os.Stat(ready); os.IsNotExist(err) {
			return
		}
		if err = f.a.RunMaintenanceJobs(t.Context(), "task-browser-acceptance", 1); err != nil {
			t.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
