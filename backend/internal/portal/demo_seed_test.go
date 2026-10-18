package portal

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"testing"
)

func TestDemoTargetRefusesNormalDeployments(t *testing.T) {
	for _, in := range []DemoOptions{{}, {Host: "test.tokpath.com"}, {Confirmation: "TEST_DATA_ONLY", Host: "tokenhub.com"}, {Confirmation: "TEST_DATA_ONLY", Host: "//test.tokpath.com"}} {
		if ValidateDemoTarget(Config{Config: &config.Config{Env: "production"}, OfficialHost: "tokenhub.com"}, in) == nil {
			t.Fatal("unapproved fixture target accepted")
		}
	}
	if ValidateDemoTarget(Config{Config: &config.Config{Env: "production"}, OfficialHost: "test.tokpath.com"}, DemoOptions{Confirmation: "TEST_DATA_ONLY", Host: "test.tokpath.com"}) != nil {
		t.Fatal("explicit test site rejected")
	}
}
func TestPostgresDemoFixtureIsAtomicRepeatableAndScoped(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	original := f.a.Config.OfficialHost
	f.a.Config.OfficialHost = "localhost"
	in := DemoOptions{Confirmation: "TEST_DATA_ONLY", Host: "localhost", ModelBaseURL: "http://127.0.0.1:18091", ModelKey: "synthetic-integration-key-long-enough"}
	out, e := f.a.SeedDemo(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	var counts [2]int64
	f.a.DB.Table("commerce_orders").Count(&counts[0])
	again, e := f.a.SeedDemo(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	f.a.DB.Table("commerce_orders").Count(&counts[1])
	one, _ := json.Marshal(out)
	two, _ := json.Marshal(again)
	if string(one) != string(two) || counts[0] != counts[1] || counts[0] < 6 {
		t.Fatalf("retry changed fixture %v", counts)
	}
	f.a.Config.OfficialHost = original
	for _, account := range out.Accounts {
		if account.Kind == "staff" {
			f.login(t, "staff", account.Email, "password1")
		} else {
			f.login(t, "customer", account.Email, "password1")
		}
	}
	var platformCustomer, channelCustomer string
	for _, email := range []string{"customer@tokenhub.local", "channel-customer@tokenhub.local"} {
		var customer string
		if e = f.a.DB.Table("organization_customer_identities").Where("email_normalized=?", email).Pluck("customer_id", &customer).Error; e != nil {
			t.Fatal(e)
		}
		if email == "customer@tokenhub.local" {
			platformCustomer = customer
		} else {
			channelCustomer = customer
		}
	}
	a, _ := f.a.Organization.CurrentOwnership(ctx, nil, platformCustomer)
	b, _ := f.a.Organization.CurrentOwnership(ctx, nil, channelCustomer)
	if a.OperatorID == b.OperatorID {
		t.Fatal("different test customer scopes merged")
	}
	if e = f.a.Organization.RequirePermission(ctx, nil, f.a.Installation.OwnerStaffID, b.OperatorID, "customers.read"); e != organization.ErrForbidden {
		t.Fatal("platform owner inherited child customer access", e)
	}
	state, e := f.a.Organization.GetBusinessStatus(ctx, nil, a.OperatorID)
	if e != nil || state.Registration {
		t.Fatal("fixture improperly opened public registration without mail", state, e)
	}
}
