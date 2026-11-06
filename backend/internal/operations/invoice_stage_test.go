package operations

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
)

func TestInvoicePolicyAlwaysUsesFullPurchase(t *testing.T) {
	s, a, _, op, _ := opsFixture(t)
	p, e := s.SaveInvoicePolicy(context.Background(), a.actor, InvoicePolicy{OperatorID: op, Enabled: true, Stage: "consumption", MaximumRatio: ledger.MustDecimal("0.25"), RulesReference: "test"}, 0)
	if e != nil || p.Stage != "purchase" || p.MaximumRatio.Cmp(ledger.FromInt64(1)) != 0 {
		t.Fatal(p, e)
	}
}

func TestInvoiceMigrationRetiresReceiptAndConsumptionPolicies(t *testing.T) {
	db := operationsDBBefore(t, "0010_order_invoices.sql")
	for _, stage := range []string{"receipt", "purchase", "consumption"} {
		op, actor := uuid.NewString(), uuid.NewString()
		if err := db.Exec("INSERT INTO operations_invoice_policies(operator_id,party_kind,enabled,stage,maximum_ratio,rules_reference,version,updated_by,updated_at) VALUES(?,'customer',true,?,0.5,'existing public terms',1,?,clock_timestamp())", op, stage, actor).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO operations_invoice_policy_versions(operator_id,party_kind,version,snapshot,created_at) SELECT operator_id,party_kind,version,to_jsonb(p),updated_at FROM operations_invoice_policies p WHERE operator_id=?", op).Error; err != nil {
			t.Fatal(err)
		}
	}
	sql, err := migrations.ReadFile("migrate/0010_order_invoices.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	var policies []InvoicePolicy
	if err = db.Find(&policies).Error; err != nil || len(policies) != 3 {
		t.Fatal(policies, err)
	}
	for _, p := range policies {
		if p.Stage != "purchase" || p.MaximumRatio.Cmp(ledger.FromInt64(1)) != 0 || p.Version != 2 || !p.Enabled {
			t.Fatal(p)
		}
	}
	var old int64
	if err = db.Table("operations_invoice_policy_versions").Where("version=1 AND snapshot->>'stage' IN ('receipt','purchase','consumption')").Count(&old).Error; err != nil || old != 3 {
		t.Fatal("policy history was changed", old, err)
	}
}
