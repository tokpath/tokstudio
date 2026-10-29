package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PurposeFlow exposes signed journal facts rather than inferring revenue from
// all USD entries. Customer wallets, cash, clearing and takeover are distinct.
type PurposeFlow struct {
	Purpose  string  `json:"purpose"`
	Kind     string  `json:"kind"`
	Unit     string  `json:"unit"`
	Increase Decimal `json:"increase"`
	Decrease Decimal `json:"decrease"`
	Net      Decimal `json:"net"`
}
type PurposeBalance struct {
	Purpose string  `json:"purpose"`
	Unit    string  `json:"unit"`
	Balance Decimal `json:"balance"`
}
type FinancialSnapshot struct {
	From                      time.Time            `json:"from"`
	To                        time.Time            `json:"to"`
	CapturedAt                time.Time            `json:"captured_at"`
	Flows                     []PurposeFlow        `json:"flows"`
	Closing                   []PurposeBalance     `json:"closing"`
	CurrentHeld               []PurposeBalance     `json:"current_held"`
	GrantEvents               []GrantFinancialFact `json:"grant_events"`
	OutstandingGrantPaidMoney Decimal              `json:"outstanding_grant_paid_money"`
	OutstandingGrantATG       Decimal              `json:"outstanding_grant_atg"`
}
type GrantFinancialFact struct {
	Kind      string  `json:"kind"`
	ATG       Decimal `json:"atg"`
	PaidMoney Decimal `json:"paid_money"`
}

// FinancialFacts is a trusted read: the caller authorizes the operator and uses
// a repeatable-read transaction when combining domains. Balances at To rebuild
// from entries; current reservations are explicitly timestamped separately.
func (s *Service) FinancialFacts(ctx context.Context, tx *gorm.DB, operator string, from, to time.Time) (FinancialSnapshot, error) {
	var out FinancialSnapshot
	if _, e := uuid.Parse(operator); e != nil {
		return out, ErrScope
	}
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, ErrScope
	}
	out = FinancialSnapshot{From: from, To: to, Flows: []PurposeFlow{}, Closing: []PurposeBalance{}, CurrentHeld: []PurposeBalance{}, GrantEvents: []GrantFinancialFact{}}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := t.Raw("SELECT clock_timestamp()").Row().Scan(&out.CapturedAt); e != nil {
			return e
		}
		purpose := `CASE WHEN a.purpose LIKE 'customer:atg:%' THEN 'customer:atg' WHEN a.purpose LIKE 'cost:supplier:%' THEN 'cost:supplier' WHEN a.purpose LIKE 'control:supplier:%' THEN 'control:supplier' WHEN a.purpose LIKE 'payable:supplier:%' THEN 'payable:supplier' ELSE a.purpose END`
		if e := t.Table("ledger_entries e").Select(purpose+` AS purpose,j.kind,e.unit,COALESCE(sum(e.amount) FILTER(WHERE e.amount>0),0)::text AS increase,COALESCE(-sum(e.amount) FILTER(WHERE e.amount<0),0)::text AS decrease,sum(e.amount)::text AS net`).Joins("JOIN ledger_journals j ON j.id=e.journal_id").Joins("JOIN ledger_accounts a ON a.id=e.account_id").Where("j.operator_id=? AND a.operator_id=? AND e.created_at>=? AND e.created_at<?", operator, operator, from, to).Group(purpose + ",j.kind,e.unit").Order("purpose,j.kind,e.unit").Scan(&out.Flows).Error; e != nil {
			return e
		}
		if e := t.Table("ledger_entries e").Select(purpose+" AS purpose,e.unit,sum(e.amount)::text AS balance").Joins("JOIN ledger_accounts a ON a.id=e.account_id").Where("a.operator_id=? AND e.created_at<?", operator, to).Group(purpose + ",e.unit").Order("purpose,e.unit").Scan(&out.Closing).Error; e != nil {
			return e
		}
		if e := t.Table("ledger_accounts a").Select(purpose+" AS purpose,a.unit,sum(a.held)::text AS balance").Where("a.operator_id=? AND a.held>0", operator).Group(purpose + ",a.unit").Order("purpose,a.unit").Scan(&out.CurrentHeld).Error; e != nil {
			return e
		}
		// Historical account entries identify the responsible operator at To.
		// Grant identity/funding stays fixed across takeover; using its current
		// operator/account pointer would rewrite both parties' historical reports.
		var outstanding struct{ PaidMoney, ATG Decimal }
		if e := t.Raw(`SELECT COALESCE(sum(g.paid_money-COALESCE(used.paid_money,0)),0)::text AS paid_money,COALESCE(sum(owned.remaining),0)::text AS atg
 FROM ledger_grants g
 JOIN ledger_accounts a ON a.purpose='customer:atg:'||g.id::text AND a.owner_id=g.customer_id AND a.operator_id=?
 JOIN (SELECT account_id,sum(amount)remaining FROM ledger_entries WHERE created_at<? GROUP BY account_id HAVING sum(amount)>0)owned ON owned.account_id=a.id
 LEFT JOIN (SELECT e.grant_id,sum(e.paid_money)paid_money FROM ledger_grant_events e JOIN ledger_journals j ON j.id=e.journal_id WHERE j.created_at<? GROUP BY e.grant_id)used ON used.grant_id=g.id`, operator, to, to).Scan(&outstanding).Error; e != nil {
			return e
		}
		out.OutstandingGrantATG = outstanding.ATG
		out.OutstandingGrantPaidMoney = outstanding.PaidMoney
		// Join the immutable journal owner, never the grant's movable current owner.
		return t.Table("ledger_grant_events e").Select("e.kind,sum(e.amount)::text AS atg,sum(e.paid_money)::text AS paid_money").Joins("JOIN ledger_journals j ON j.id=e.journal_id").Where("j.operator_id=? AND j.created_at>=? AND j.created_at<?", operator, from, to).Group("e.kind").Order("e.kind").Scan(&out.GrantEvents).Error
	})
	return out, e
}
