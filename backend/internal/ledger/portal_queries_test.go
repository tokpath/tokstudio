package ledger

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPostgresLiveAssetSummaryUsesFulfillmentClock(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, customer := uuid.NewString(), uuid.NewString()
	var now time.Time
	if err := s.db.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
		t.Fatal(err)
	}
	for i, at := range []time.Time{now, now.Add(time.Hour)} {
		_, err := s.IssueGrant(ctx, nil, GrantInput{OperatorID: op, CustomerID: customer, OrderID: uuid.NewString(), BusinessKey: uuid.NewString(), ActorID: "test", Reason: "database-clock delivery", PaidMoney: FromInt64(10), Sources: []GrantSource{{Kind: "paid", Amount: FromInt64(10)}, {Kind: "package_gift", Amount: FromInt64(2)}}, EffectiveAt: at})
		if err != nil {
			t.Fatal(i, err)
		}
	}
	live, err := s.CustomerAssetSummary(ctx, nil, op, customer, time.Time{})
	if err != nil || live.AvailableATG.String() != "12" || live.PurchasedRemainingATG.String() != "10" || live.GiftRemainingATG.String() != "2" {
		t.Fatal("live view missed delivered rights or included future rights", live, err)
	}
	historical, err := s.CustomerAssetSummary(ctx, nil, op, customer, now.Add(-time.Minute))
	if err != nil || !historical.AvailableATG.IsZero() {
		t.Fatal("explicit historical time was replaced", historical, err)
	}
}

func TestAccountSearchUsesTheVisibleOwnerAndPurpose(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	if err := s.db.Exec(`CREATE TABLE organization_customers (id uuid PRIMARY KEY, display_name text);
CREATE TABLE organization_customer_identities (customer_id uuid, email_normalized text, is_current boolean);
CREATE TABLE organization_operators (id uuid PRIMARY KEY, name text, legal_name text)`).Error; err != nil {
		t.Fatal(err)
	}
	op, customer := uuid.NewString(), uuid.NewString()
	if err := s.db.Exec(`INSERT INTO organization_customers(id, display_name) VALUES (?, ?)`, customer, "邀请用户").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Exec(`INSERT INTO organization_customer_identities(customer_id, email_normalized, is_current) VALUES (?, ?, true)`, customer, "user@tokenhub.local").Error; err != nil {
		t.Fatal(err)
	}
	frozen, err := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: customer, Purpose: "commission:frozen", Unit: MoneyUnit})
	if err != nil {
		t.Fatal(err)
	}
	rights, err := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: customer, Purpose: "customer:atg:" + uuid.NewString(), Unit: "ATG"})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(query string) map[string]bool {
		t.Helper()
		rows, err := s.ListOperatorAccounts(ctx, nil, op, "", "", query, 20)
		if err != nil {
			t.Fatal(query, err)
		}
		out := map[string]bool{}
		for _, row := range rows {
			out[row.ID] = true
		}
		return out
	}
	byEmail := ids("user@tokenhub.local")
	if !byEmail[frozen.ID] || !byEmail[rights.ID] {
		t.Fatal("email did not find this customer's accounts", byEmail)
	}
	byName := ids("邀请用户")
	if !byName[frozen.ID] || !byName[rights.ID] {
		t.Fatal("display name did not find this customer's accounts", byName)
	}
	byPurpose := ids("冻结佣金")
	if !byPurpose[frozen.ID] || byPurpose[rights.ID] {
		t.Fatal("purpose label matched the wrong accounts", byPurpose)
	}
	byRights := ids("客户 ATG")
	if byRights[frozen.ID] || !byRights[rights.ID] {
		t.Fatal("ATG rights label did not find the grant account", byRights)
	}
	byCurrency := ids("USD")
	if !byCurrency[frozen.ID] || byCurrency[rights.ID] {
		t.Fatal("settlement currency did not stay on money accounts", byCurrency)
	}
	if page := ids("资产账户"); len(page) != 0 {
		t.Fatal("the page name is not an account field", page)
	}
}

func TestPostgresLedgerWorkspaceFilterAndChronologicalCursor(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, other := uuid.NewString(), uuid.NewString()
	var wallet, control Account
	for _, scope := range []string{op, other} {
		for _, purpose := range []string{"customer:money", "control:credits", "order:funds"} {
			row, err := s.EnsureAccount(ctx, nil, Account{OperatorID: scope, OwnerID: uuid.NewString(), Purpose: purpose, Unit: MoneyUnit, AllowNegative: strings.HasPrefix(purpose, "control:")})
			if err != nil {
				t.Fatal(err)
			}
			if scope == op && purpose == "customer:money" {
				wallet = row
			}
			if scope == op && purpose == "control:credits" {
				control = row
			}
		}
	}
	rows, err := s.ListAssetAccounts(ctx, op, "", "", 1, false)
	if err != nil || len(rows) != 1 || rows[0].ID != wallet.ID {
		t.Fatal("filter was applied after pagination", rows, err)
	}
	all, err := s.ListAssetAccounts(ctx, op, "", "", 100, true)
	if err != nil || len(all) != 3 {
		t.Fatal("scope or complete list incorrect", all, err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{"ffffffff-0000-4000-8000-000000000001", "11111111-0000-4000-8000-000000000001", "22222222-0000-4000-8000-000000000001"}
	for i, id := range ids {
		created := at
		if i == 0 {
			created = at.Add(-time.Hour)
		}
		j := Journal{ID: id, OperatorID: op, BusinessKey: id, Kind: "test_fixture", SourceID: id, Fingerprint: id, ActorID: "test", Reason: "chronology fixture", CreatedAt: created}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&j).Error; err != nil {
				return err
			}
			n := FromInt64(int64(i + 1))
			for _, line := range []Entry{{ID: uuid.NewString(), JournalID: id, AccountID: wallet.ID, Unit: MoneyUnit, Amount: FromInt64(1), BalanceAfter: n, CreatedAt: created}, {ID: uuid.NewString(), JournalID: id, AccountID: control.ID, Unit: MoneyUnit, Amount: FromInt64(-1), BalanceAfter: n.Neg(), CreatedAt: created}} {
				if err := tx.Create(&line).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Account{}).Where("id=?", wallet.ID).Update("balance", n).Error; err != nil {
				return err
			}
			return tx.Model(&Account{}).Where("id=?", control.ID).Update("balance", n.Neg()).Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListJournals(ctx, nil, op, "", "", 1)
	if err != nil || len(first) != 1 || first[0].ID != ids[2] {
		t.Fatal(first, err)
	}
	second, err := s.ListJournals(ctx, nil, op, "", first[0].ID, 1)
	if err != nil || len(second) != 1 || second[0].ID != ids[1] {
		t.Fatal(second, err)
	}
	third, err := s.ListJournals(ctx, nil, op, "", second[0].ID, 1)
	if err != nil || len(third) != 1 || third[0].ID != ids[0] {
		t.Fatal(third, err)
	}
	if _, err = s.ListJournals(ctx, nil, other, "", first[0].ID, 1); err == nil {
		t.Fatal("foreign workspace cursor accepted")
	}
}
