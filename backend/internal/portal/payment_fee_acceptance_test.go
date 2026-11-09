package portal

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in browser fixture: a real isolated application and database, no mocked UI responses.
func TestExternalPaymentFeeAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_FEE_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated fee acceptance server")
	}
	f := setupPortal(t)
	a := f.a
	ctx := t.Context()
	op, owner := a.Installation.OperatorID, a.Installation.OwnerStaffID
	f.openRegistration(t)
	f.registered(t, "fees@example.test")
	if e := a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; e != nil {
		t.Fatal(e)
	}
	var e error
	if _, e = a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: "Fee acceptance plan", PriceMoney: ledger.FromInt64(40), PaidATG: ledger.FromInt64(40), GiftATG: ledger.FromInt64(10), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, TermsReference: "isolated fixture"}); e != nil {
		t.Fatal(e)
	}
	roles := f.seedPaymentOperatorRoles(t)
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
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]any{"roles": roles, "operator": op, "customer_email": "fees@example.test", "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
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

// Distinct owners exercise the same settings through each real authorization scope.
func (f *testApp) seedPaymentOperatorRoles(t *testing.T) []map[string]string {
	t.Helper()
	ctx := t.Context()
	parent, actor := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	roles := []map[string]string{}
	for _, kind := range []organization.OperatorKind{organization.OEM, organization.Channel} {
		label := string(kind)
		created, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: parent, Kind: kind, Name: "Payment " + label, BrandName: "Payment test brand"})
		if e != nil {
			t.Fatal(e)
		}
		email, password := label+"-fees@example.test", "operator-password-test"
		hash, e := organization.HashPassword(password)
		if e != nil {
			t.Fatal(e)
		}
		person := Staff{ID: uuid.NewString(), Email: email, DisplayName: "Payment " + label, PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
		if e = f.a.DB.Create(&person).Error; e != nil {
			t.Fatal(e)
		}
		invite, e := f.a.Organization.InviteStaff(ctx, nil, actor, created.Operator.ID, email, "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: person.ID, Email: email, EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		roles = append(roles, map[string]string{"kind": label, "operator": created.Operator.ID, "email": email, "password": password})
		parent, actor = created.Operator.ID, person.ID
	}
	return roles
}
