package cooperation

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresProcurementExportKeepsActualPartyAndTimeScope(t *testing.T) {
	f, _, _, quote := replenishmentFixture(t, "http://127.0.0.1:18090")
	order, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, quote.ID, "export-included", amount("10"))
	if err != nil {
		t.Fatal(err)
	}
	older, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, quote.ID, "export-outside-range", amount("10"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&PurchaseOrder{}).Where("id=?", older.ID).Update("created_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	child, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.platform, Kind: organization.Channel, Name: "unrelated sibling"})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.org.InviteStaff(f.ctx, nil, f.owner, child.Operator.ID, "export-sibling@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, inv.Token, organization.VerifiedStaff{ID: f.owner, Email: "export-sibling@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	quote.ID, quote.BuyerID = "", child.Operator.ID
	otherQuote, err := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, child.Operator.ID, otherQuote.ID, "export-foreign", amount("10")); err != nil {
		t.Fatal(err)
	}
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Minute)
	rows, err := f.s.OperationalPurchaseOrders(f.ctx, nil, f.channel, "", from, to, 100)
	if err != nil || len(rows) != 1 || rows[0].ID != order.ID {
		t.Fatal("time/tenant export boundary", rows, err)
	}
	rows, err = f.s.OperationalPurchaseOrders(f.ctx, nil, f.platform, f.channel, from, to, 100)
	if err != nil || len(rows) != 1 || rows[0].ID != order.ID {
		t.Fatal("counterparty filter widened the supplier's own transactions", rows, err)
	}
	rows, err = f.s.OperationalPurchaseOrders(f.ctx, nil, f.channel, child.Operator.ID, from, to, 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("sibling counterparty escaped the operator boundary", rows, err)
	}
	rows, err = f.s.OperationalPurchaseOrders(f.ctx, nil, f.channel, uuid.NewString(), from, to, 100)
	if err != nil || len(rows) != 0 {
		t.Fatal("unrelated counterparty exposed procurement", rows, err)
	}
}
