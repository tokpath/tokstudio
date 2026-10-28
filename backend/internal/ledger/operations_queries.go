package ledger

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// IntegrityFact compares materialized state to the append-only facts that own it.
// Call with the caller's repeatable-read transaction for a consistent report.
type IntegrityFact struct {
	Reference string  `json:"reference"`
	Kind      string  `json:"kind"`
	Unit      string  `json:"unit"`
	Expected  Decimal `json:"expected"`
	Actual    Decimal `json:"actual"`
}

func (s *Service) OperationalIntegrity(ctx context.Context, tx *gorm.DB, operator string) ([]IntegrityFact, error) {
	if _, e := uuid.Parse(operator); e != nil {
		return nil, ErrScope
	}
	out := []IntegrityFact{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`SELECT a.id::text AS reference,'account_balance' AS kind,a.unit,COALESCE(e.amount,0)::text AS expected,a.balance::text AS actual
   FROM ledger_accounts a LEFT JOIN (SELECT account_id,sum(amount) AS amount FROM ledger_entries GROUP BY account_id)e ON e.account_id=a.id
   WHERE a.operator_id=? AND a.balance<>COALESCE(e.amount,0)
   UNION ALL
   SELECT a.id::text,'account_held',a.unit,COALESCE(h.amount,0)::text,a.held::text
   FROM ledger_accounts a LEFT JOIN(SELECT ha.account_id,sum(ha.amount) AS amount FROM ledger_hold_allocations ha JOIN ledger_holds h ON h.id=ha.hold_id WHERE h.status='active' GROUP BY ha.account_id)h ON h.account_id=a.id
   WHERE a.operator_id=? AND a.held<>COALESCE(h.amount,0)
   UNION ALL
   SELECT j.id::text,'journal_balance',e.unit,'0',sum(e.amount)::text FROM ledger_journals j JOIN ledger_entries e ON e.journal_id=j.id
   WHERE j.operator_id=? GROUP BY j.id,e.unit HAVING sum(e.amount)<>0`, operator, operator, operator).Scan(&out).Error
	})
	return out, e
}

type ExportEntry struct {
	JournalID    string    `json:"journal_id"`
	EntryID      string    `json:"entry_id"`
	AccountID    string    `json:"account_id"`
	OwnerID      string    `json:"owner_id"`
	Kind         string    `json:"kind"`
	SourceID     string    `json:"source_id"`
	Purpose      string    `json:"purpose"`
	Unit         string    `json:"unit"`
	Amount       Decimal   `json:"amount"`
	BalanceAfter Decimal   `json:"balance_after"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Service) OperationalEntries(ctx context.Context, tx *gorm.DB, operator, owner string, from, to time.Time, limit int) ([]ExportEntry, error) {
	if !validPortalScope(operator, owner) || limit < 1 || limit > 100001 || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, ErrScope
	}
	out := []ExportEntry{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Table("ledger_entries e").Select("j.id AS journal_id,e.id AS entry_id,e.account_id,a.owner_id,j.kind,j.source_id,a.purpose,e.unit,e.amount::text,e.balance_after::text,e.created_at").Joins("JOIN ledger_journals j ON j.id=e.journal_id").Joins("JOIN ledger_accounts a ON a.id=e.account_id").Where("j.operator_id=? AND a.operator_id=? AND e.created_at>=? AND e.created_at<?", operator, operator, from, to)
		if owner != "" {
			q = q.Where("a.owner_id=?", owner)
		}
		return q.Order("e.created_at,e.id").Limit(limit).Scan(&out).Error
	})
	return out, e
}
func (s *Service) HasAdjustment(ctx context.Context, tx *gorm.DB, operator, id string) (bool, error) {
	var count int64
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Model(&Journal{}).Where("id=? AND operator_id=? AND (original_id IS NOT NULL OR kind IN ('usage_reversal','rights_cancellation','correction','reversal','adjustment'))", id, operator).Count(&count).Error
	})
	return count == 1, e
}

// PaidConsumptionByOrder is a trusted source projection. The caller must first
// prove the customer's relationship to each original funding order; a takeover
// may change the current grant operator without changing original order IDs.
func (s *Service) PaidConsumptionByOrder(ctx context.Context, tx *gorm.DB, customer string, orders []string) (map[string]Decimal, error) {
	out := map[string]Decimal{}
	if !validPortalScope("00000000-0000-0000-0000-000000000000", customer) || len(orders) > 10000 {
		return nil, ErrScope
	}
	if len(orders) == 0 {
		return out, nil
	}
	var rows []struct {
		OrderID string
		Amount  Decimal
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Model(&Grant{}).Select("order_id,COALESCE(sum(consumed_paid_usd),0)::text AS amount").Where("customer_id=? AND order_id IN ?", customer, orders).Group("order_id").Scan(&rows).Error
	})
	for _, r := range rows {
		out[r.OrderID] = r.Amount
	}
	return out, e
}
