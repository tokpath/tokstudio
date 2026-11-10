package portal

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/branding"
)

// Opt-in real browser/S3 fixture. The domain provider is deliberately isolated;
// it is not evidence of real OEM DNS or public certificate issuance.
func TestExternalBrandingAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_BRAND_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated branding acceptance")
	}
	f := setupPortal(t)
	roles := f.seedPaymentOperatorRoles(t)
	a := f.a
	a.Config.DomainCNAMETarget = "ingress.example.test"
	a.DomainVerifier = branding.DomainVerifierFunc(func(context.Context, string, string) (branding.DomainVerification, error) {
		return branding.DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "isolated-acceptance-provider"}, nil
	})
	f.server.Close()
	a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	a.Config.WebOrigin = "http://127.0.0.1:13003"
	a.Config.PublicBaseURL = a.Config.WebOrigin
	listener, e := net.Listen("tcp", "127.0.0.1:18081")
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	data, _ := json.Marshal(map[string]any{"roles": roles, "operator": a.Installation.OperatorID, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if e = os.WriteFile(ready, data, 0600); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Hour)
	for time.Now().Before(deadline) {
		if _, e = os.Stat(ready); os.IsNotExist(e) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
