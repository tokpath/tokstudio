package incentives

import (
	"io/fs"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresDefaultCommissionIsZeroUntilPublishedAndKeepsOrderSnapshot(t *testing.T) {
	f := setupOnDB(t, integrationDB(t))
	baseline, err := f.s.CurrentPolicy(f.ctx, nil, f.operator)
	if err != nil || baseline.Version != 0 || baseline.QualificationThreshold() != nil || !baseline.DirectRate.IsZero() || !baseline.IndirectRate.IsZero() {
		t.Fatal("unexpected default policy", baseline, err)
	}
	upstream := f.register(t, "indirect-default@example.test", "")
	inviter := f.register(t, "direct-default@example.test", upstream.InviteCode)
	oldBuyer := f.register(t, "old-default@example.test", inviter.InviteCode)
	newBuyer := f.register(t, "new-default@example.test", inviter.InviteCode)
	oldOrder := f.buy(t, oldBuyer.Customer.ID, "default-rate-purchase", "100", ActivitySelection{})
	var snapshot snapshotRow
	if err = f.db.Take(&snapshot, "order_id=?", oldOrder.ID).Error; err != nil || snapshot.PolicyID != baseline.ID || !snapshot.DirectRate.IsZero() || !snapshot.IndirectRate.IsZero() {
		t.Fatal("default order did not lock the zero rates", snapshot, err)
	}
	for _, customer := range []string{inviter.Customer.ID, upstream.Customer.ID} {
		q, err := f.s.GetQualification(f.ctx, nil, f.operator, customer)
		if err != nil || q.AcquiredAt != nil {
			t.Fatal("default rule granted KOL qualification", q, err)
		}
	}
	policy, err := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 0, DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 86400})
	if err != nil || policy.Version != 1 {
		t.Fatal(policy, err)
	}
	newOrder := f.buy(t, newBuyer.Customer.ID, "published-rate-purchase", "100", ActivitySelection{})
	f.consume(t, oldBuyer.Customer.ID, "80")
	f.consume(t, newBuyer.Customer.ID, "80")
	for _, expected := range []struct{ customer, amount string }{{inviter.Customer.ID, "12"}, {upstream.Customer.ID, "4"}} {
		entries, err := f.s.CommissionEntries(f.ctx, nil, f.operator, expected.customer)
		if err != nil || len(entries) != 1 || entries[0].OrderID != newOrder.ID || entries[0].AmountMoney.Cmp(d(expected.amount)) != 0 {
			t.Fatal("expected one commission from the later order", entries, err)
		}
	}
}

func TestPostgresDefaultCommissionMigrationOnlyFillsMissingPolicies(t *testing.T) {
	const migration = "0015_default_policy.sql"
	f := setupOnDB(t, integrationDBBefore(t, migration))
	configured, err := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ThresholdMoney: d("20"), DirectRate: d("0.12"), RewardValiditySeconds: 86400})
	if err != nil {
		t.Fatal(err)
	}
	create := func(kind organization.OperatorKind, name string) string {
		t.Helper()
		op, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.operator, Kind: kind, Name: name, BrandName: name})
		if err != nil {
			t.Fatal(err)
		}
		return op.Operator.ID
	}
	oem := create(organization.OEM, "Existing OEM")
	_, migrations := Migrations()
	body, err := fs.ReadFile(migrations, migration)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	current, err := f.s.CurrentPolicy(f.ctx, nil, f.operator)
	if err != nil || current.ID != configured.ID || current.DirectRate.Cmp(d("0.12")) != 0 {
		t.Fatal("existing policy changed", current, err)
	}
	for _, operator := range []string{oem, create(organization.Channel, "New channel")} {
		p, err := f.s.CurrentPolicy(f.ctx, nil, operator)
		if err != nil || p.Version != 0 || !p.DirectRate.IsZero() || !p.IndirectRate.IsZero() {
			t.Fatal("missing zero policy", operator, p, err)
		}
	}
}
