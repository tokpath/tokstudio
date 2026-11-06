package operations

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
)

func TestInvoiceInstructionsOptionalAndVersioned(t *testing.T) {
	s, a, _, op, customer := opsFixture(t)
	ctx := context.Background()
	if _, err := s.SaveInvoiceInstructions(ctx, a.actor, op, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveInvoiceInstructions(ctx, a.actor, op, "Within three working days", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveInvoiceInstructions(ctx, a.actor, op, "stale", 1); err != ErrConflict {
		t.Fatal(err)
	}
	if _, err := s.SaveInvoiceInstructions(ctx, customer, op, "unauthorized", 2); err == nil {
		t.Fatal("customer edited instructions")
	}
	if _, err := s.SaveInvoiceInstructions(ctx, a.actor, op, "", 2); err != nil {
		t.Fatal("cannot clear optional instructions", err)
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
	var policies []struct {
		Stage        string
		MaximumRatio ledger.Decimal
		Version      int64
		Enabled      bool
	}
	if err = db.Table("operations_invoice_policies").Find(&policies).Error; err != nil || len(policies) != 3 {
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

func TestInvoiceMigrationRemovesClosedGatesWithoutRemovingInvoices(t *testing.T) {
	db := operationsDBBefore(t, "0011_invoice_instructions.sql")
	op, actor, customer, id := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := db.Exec("INSERT INTO operations_invoice_policies(operator_id,party_kind,enabled,stage,maximum_ratio,rules_reference,version,updated_by,updated_at) VALUES(?,'customer',false,'purchase',1,'old-code',1,?,clock_timestamp())", op, actor).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO operations_invoices(id,operator_id,customer_id,business_key,fingerprint,amount_money,state,details_ciphertext,created_at,policy_stage,policy_version,policy_ratio) VALUES(?,?,?,'preserved','preserved',10,'requested','encrypted',clock_timestamp(),'purchase',1,1)`, id, op, customer).Error; err != nil {
		t.Fatal(err)
	}
	sql, err := migrations.ReadFile("migrate/0011_invoice_instructions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	var row Invoice
	if err = db.First(&row, "id=?", id).Error; err != nil || row.AmountMoney.String() != "10" || row.DetailsCiphertext != "encrypted" {
		t.Fatal(row, err)
	}
	if db.Migrator().HasTable("operations_invoice_policies") {
		t.Fatal("obsolete gate survived")
	}
}
