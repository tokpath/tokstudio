package portal

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in browser fixture. Uses synthetic KVoucher responses and an isolated
// PostgreSQL schema, never a real merchant or customer account.
func TestExternalCheckoutFailureAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_CHECKOUT_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated checkout acceptance server")
	}
	scenario := "mismatch"
	if os.Getenv("TOKENHUB_CHECKOUT_ACCEPTANCE_SCENARIO") == "success" {
		scenario = "success"
	}
	f, _, _, _ := setupCheckoutApp(t, scenario)
	roles := f.seedPaymentOperatorRoles(t)
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
	raw, _ := json.Marshal(map[string]any{"email": "checkout@example.test", "password": "customer-password-test", "roles": roles, "operator": f.a.Installation.OperatorID})
	if err = os.WriteFile(ready, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Hour); time.Now().Before(deadline); {
		if _, err = os.Stat(ready); os.IsNotExist(err) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
