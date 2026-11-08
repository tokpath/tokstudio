package portal

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
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
	m, e := a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "Cash or bank transfer: pay the displayed total."}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	rule, e := paymentmethods.Current(ctx, a.DB, op, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	rule.Name = "Bank transfer"
	rule.FeeRate = ledger.MustDecimal("0.02")
	rule.PayoutEnabled = true
	rule.PayoutKind = "bank_cn"
	if e = a.DB.Transaction(func(tx *gorm.DB) error { _, e := paymentmethods.Save(ctx, tx, rule, rule.Version); return e }); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: "Fee acceptance plan", PriceMoney: ledger.FromInt64(40), PaidATG: ledger.FromInt64(40), GiftATG: ledger.FromInt64(10), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, TermsReference: "isolated fixture"}); e != nil {
		t.Fatal(e)
	}
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
	data, _ := json.Marshal(map[string]string{"operator": op, "merchant": m.ID, "customer_email": "fees@example.test", "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
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
