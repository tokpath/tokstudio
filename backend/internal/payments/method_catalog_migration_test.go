package payments

import (
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"gorm.io/gorm"
	"testing"
)

func TestPaymentMethodCatalogUpgradeKeepsHistoricalFeeVersions(t *testing.T) {
	db := paymentDB(t)
	ctx := t.Context()
	svc := NewService(db, "test-payment-key", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	m := Merchant{ID: "upgrade-merchant", OperatorID: "upgrade-operator", Driver: "alipay", Credentials: map[string]string{}}
	if _, e := svc.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	original, e := paymentmethods.Current(ctx, db, m.OperatorID, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	var legacy paymentmethods.Rule
	e = db.Transaction(func(tx *gorm.DB) error {
		var err error
		legacy, err = paymentmethods.Save(ctx, tx, paymentmethods.Rule{MethodID: "legacy-payout", OperatorID: m.OperatorID, Name: "Original wallet payout", PayoutEnabled: true, PayoutKind: "alipay", FeeRate: ledger.MustDecimal("0.05")}, 0)
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec("ALTER TABLE payments_merchants DROP COLUMN payment_method_id").Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(methodCatalogSchema).Error }); e != nil {
		t.Fatal(e)
	}
	var identity paymentmethods.Identity
	if e = db.Where("operator_id=? AND kind='alipay'", m.OperatorID).First(&identity).Error; e != nil || identity.MethodID != m.ID || identity.MerchantID != m.ID {
		t.Fatal(identity, e)
	}
	prior, e := paymentmethods.Version(ctx, db, m.OperatorID, legacy.ID)
	if e != nil || prior.FeeRate.String() != "0.05" || !prior.PayoutEnabled || prior.Paused {
		t.Fatal("historical snapshot changed", prior, e)
	}
	retired, e := paymentmethods.Current(ctx, db, m.OperatorID, legacy.MethodID)
	if e != nil || !retired.Paused {
		t.Fatal(retired, e)
	}
	current, e := paymentmethods.Current(ctx, db, m.OperatorID, m.ID)
	if e != nil || current.Kind != "alipay" || current.Version != original.Version+1 {
		t.Fatal(current, e)
	}
}

func TestMethodCapabilitiesMigrationKeepsSnapshotsAndFoldsCash(t *testing.T) {
	db := paymentDB(t)
	ctx := t.Context()
	versions := map[string]string{}
	for _, row := range []struct{ id, op, kind string }{{"old-card", "one", "bank_card"}, {"old-offline", "one", "offline"}, {"old-cash", "one", "cash"}, {"cash-only", "two", "cash"}} {
		versions[row.id] = uuid.NewString()
		if e := db.Exec(`INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,method_kind,payout_kind,payout_enabled,collection,flat_fee_money,fee_rate,created_by) VALUES(?,?,?,1,?,?,'bank_cn',true,true,2,0,'test')`, versions[row.id], row.id, row.op, row.kind, row.kind).Error; e != nil {
			t.Fatal(e)
		}
		if e := db.Create(&paymentmethods.Identity{OperatorID: row.op, MethodID: row.id, Kind: row.kind}).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e := db.Transaction(func(tx *gorm.DB) error { return tx.Exec(methodCapabilitiesSchema).Error }); e != nil {
		t.Fatal(e)
	}
	card, e := paymentmethods.Current(ctx, db, "one", "old-card")
	if e != nil || card.Kind != "stripe" || card.PayoutEnabled || card.PayoutKind != "" || !card.Collection || card.FlatFeeMoney.String() != "2" {
		t.Fatal(card, e)
	}
	old, e := paymentmethods.Version(ctx, db, "one", versions["old-card"])
	if e != nil || old.Kind != "bank_card" || !old.PayoutEnabled {
		t.Fatal("historical version changed", old, e)
	}
	cash, e := paymentmethods.Current(ctx, db, "one", "old-cash")
	if e != nil || !cash.Paused || cash.PayoutEnabled {
		t.Fatal(cash, e)
	}
	cash, e = paymentmethods.Current(ctx, db, "two", "cash-only")
	if e != nil || cash.Kind != "offline" || cash.PayoutKind != "offline" || !cash.PayoutEnabled {
		t.Fatal(cash, e)
	}
}

func TestSimpleWithdrawalMigrationRestoresOriginalRulesAndRespectsOperatorEdits(t *testing.T) {
	db := paymentDB(t)
	ctx := t.Context()
	for _, kind := range []string{"wechat", "alipay", "usdt"} {
		if e := db.Exec(`INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,method_kind,payout_kind,payout_enabled,fee_rate) VALUES(?,?, 'simple',1,?,?,?,true,0.02)`, uuid.NewString(), kind, kind, kind, kind).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e := db.Transaction(func(tx *gorm.DB) error { return tx.Exec(methodCapabilitiesSchema).Error }); e != nil {
		t.Fatal(e)
	}
	// This operator subsequently chose to keep Alipay disabled. Do not undo that choice.
	if e := db.Exec(`INSERT INTO payment_method_versions(id,method_id,operator_id,version,name,method_kind,payout_kind,payout_enabled,fee_rate,created_by,payout_currency,payout_rate) VALUES(?,'alipay','simple',3,'支付宝','alipay','alipay',false,0.03,'operator','CNY',7)`, uuid.NewString()).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(simpleWithdrawalsSchema).Error; e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"wechat", "alipay", "usdt"} {
		rule, e := paymentmethods.Current(ctx, db, "simple", kind)
		if e != nil || rule.PayoutEnabled != (kind != "alipay") {
			t.Fatal(kind, rule, e)
		}
		want := "0.02"
		if kind == "alipay" {
			want = "0.03"
		}
		if rule.FeeRate.String() != want {
			t.Fatal(rule)
		}
		var details struct {
			PayoutCurrency, PayoutNote string
			PayoutRate                 ledger.Decimal
		}
		if e := db.Table("payment_method_versions").Where("id=?", rule.ID).Scan(&details).Error; e != nil || details.PayoutCurrency != "" || !details.PayoutRate.IsZero() || details.PayoutNote != "" {
			t.Fatal(details, e)
		}
		var original paymentmethods.Rule
		if e := db.Where("operator_id='simple' AND method_id=? AND version=1", kind).First(&original).Error; e != nil || !original.PayoutEnabled || original.FeeRate.String() != "0.02" {
			t.Fatal("accepted fee version changed", original, e)
		}
	}
}
