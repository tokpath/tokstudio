package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func TestPostgresCryptoRemovalReleasesOnlyAffectedWithdrawalHolds(t *testing.T) {
	f := setupPortal(t)
	ctx, db := t.Context(), f.a.DB
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	cryptoPolicy, normalPolicy, cryptoMethod := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for id, kind := range map[string]string{cryptoPolicy: "usdt", normalPolicy: "offline"} {
		method := uuid.NewString()
		if kind == "usdt" {
			method = cryptoMethod
		}
		if e := db.Exec(`INSERT INTO payment_method_versions(id,operator_id,method_id,version,name,collection,payout_enabled,payout_kind,method_kind) VALUES(?,?,?,1,?,true,true,?,?)`, id, op, method, kind, kind, kind).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e := db.Exec(`INSERT INTO payment_method_identities(operator_id,kind,method_id) VALUES(?,'usdt',?)`, op, cryptoMethod).Error; e != nil {
		t.Fatal(e)
	}
	accounts := []ledger.Account{}
	cryptoHolds := []string{}
	for _, wallet := range []string{"balance", "commission"} {
		customer := uuid.NewString()
		account, e := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: wallet, Unit: ledger.MoneyUnit})
		if e != nil {
			t.Fatal(e)
		}
		control, e := f.a.Ledger.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: op, Purpose: "control:test-opening", Unit: ledger.MoneyUnit, AllowNegative: true})
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.a.Ledger.Post(ctx, nil, ledger.Posting{OperatorID: op, BusinessKey: uuid.NewString(), Kind: "test_opening", SourceID: customer, ActorID: "test", Reason: "isolated migration fixture", Lines: []ledger.Line{{AccountID: account.ID, Amount: ledger.FromInt64(100)}, {AccountID: control.ID, Amount: ledger.FromInt64(-100)}}})
		if e != nil {
			t.Fatal(e)
		}
		accounts = append(accounts, account)
		for _, policy := range []string{cryptoPolicy, normalPolicy} {
			hold, e := f.a.Ledger.Reserve(ctx, nil, ledger.Reservation{OperatorID: op, BusinessKey: uuid.NewString(), Purpose: "withdrawal", ExpiresAt: time.Now().Add(time.Hour), Amounts: []ledger.Line{{AccountID: account.ID, Amount: ledger.FromInt64(15)}}})
			if e != nil {
				t.Fatal(e)
			}
			if policy == cryptoPolicy {
				cryptoHolds = append(cryptoHolds, hold.ID)
			}
			id := uuid.NewString()
			if e = db.Exec(`INSERT INTO incentives_withdrawals(id,operator_id,original_operator_id,customer_id,business_key,fingerprint,policy_id,amount_money,fee_money,net_money,fee_remainder,recipient_reference,hold_id,status,created_at,wallet) VALUES(?,?,?,?,?,?,?,15,0,15,'0','test recipient',?,'requested',now(),?)`, id, op, op, customer, id, id, policy, hold.ID, wallet).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	// Existing encrypted configuration can be read but its retired driver must
	// disappear from selection and must not leave a worker retrying webhooks.
	merchant := payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "fixture"}}
	if _, e := f.a.Payments.SaveMerchant(ctx, owner, merchant, 0); e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`UPDATE payments_merchants SET driver='nowpayments',definition=jsonb_set(definition,'{driver}','"nowpayments"') WHERE id=?`, merchant.ID).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`INSERT INTO payments_webhook_inbox(merchant_id,event_id,body_digest,hint,state,received_at) VALUES(?,'obsolete','test','{}','pending',now())`, merchant.ID).Error; e != nil {
		t.Fatal(e)
	}
	sql, e := migrations.ReadFile("migrate/0007_remove_crypto_payments.sql")
	if e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(sql)).Error }); e != nil {
			t.Fatal(e)
		}
	}
	for _, original := range accounts {
		account, e := f.a.Ledger.GetAccount(ctx, original.ID)
		if e != nil || account.Balance.String() != "100" || account.Held.String() != "15" {
			t.Fatal("unrelated funds or holds changed", account, e)
		}
	}
	for _, id := range cryptoHolds {
		var hold ledger.Hold
		if e = db.First(&hold, "id=?", id).Error; e != nil || hold.Status != "released" {
			t.Fatal(hold, e)
		}
	}
	var count int64
	if e = db.Table("incentives_withdrawals").Count(&count).Error; e != nil || count != 2 {
		t.Fatal(count, e)
	}
	if e = db.Table("payments_webhook_inbox").Count(&count).Error; e != nil || count != 0 {
		t.Fatal(count, e)
	}
	r, e := paymentmethods.Current(ctx, db, op, cryptoMethod)
	if e != nil || !r.Paused || r.Collection || r.PayoutEnabled {
		t.Fatal(r, e)
	}
	for _, wallet := range []string{"balance", "commission"} {
		methods, e := f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, wallet)
		if e != nil || len(methods) != 1 || methods[0].PayoutKind != "offline" {
			t.Fatal(methods, e)
		}
	}
	methods, e := f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 0 {
		t.Fatal(methods, e)
	}
}
