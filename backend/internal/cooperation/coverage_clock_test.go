package cooperation

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"testing"
)

func TestPostgresSaleCoverageSeesRightsEffectiveOnDatabaseClock(t *testing.T) {
	f := setup(t)
	f.purchase(t, "database-clock-inventory", "100", "100", "0")
	e := f.db.Transaction(func(tx *gorm.DB) error {
		at, e := businessTime(f.ctx, tx)
		if e != nil {
			return e
		}
		_, e = f.l.IssueGrant(f.ctx, tx, ledger.GrantInput{OperatorID: f.channel, CustomerID: uuid.NewString(), OrderID: "newly-delivered", BusinessKey: "db-clock-rights", ActorID: f.owner, Reason: "freshly fulfilled paid rights", PaidMoney: amount("100"), Sources: []ledger.GrantSource{{Kind: "paid", Amount: amount("100")}}, EffectiveAt: at})
		if e != nil {
			return e
		}
		_, e = f.s.Reserve(f.ctx, tx, commerce.Order{ID: uuid.NewString(), OperatorID: f.channel, PaidATG: amount("1")})
		if !errors.Is(e, ErrCoverage) {
			t.Fatalf("fresh DB-clock rights omitted from sale coverage: %v", e)
		}
		_, e = f.s.Restore(f.ctx, tx, commerce.Order{ID: uuid.NewString(), OperatorID: f.channel, PaidATG: amount("1")})
		if !errors.Is(e, ErrCoverage) {
			t.Fatalf("fresh DB-clock rights omitted from restored coverage: %v", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
