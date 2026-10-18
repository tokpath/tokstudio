package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// These read models require an authenticated application scope. Every query is
// constrained to an exact operator and, for customer views, an exact owner.
type CustomerAssetSummary struct {
	AvailableATG, HeldATG, PurchasedRemainingATG, GiftRemainingATG, ExpiredATG Decimal
}

func validPortalScope(operator, owner string) bool {
	if _, err := uuid.Parse(operator); err != nil {
		return false
	}
	if owner != "" {
		if _, err := uuid.Parse(owner); err != nil {
			return false
		}
	}
	return true
}

func (s *Service) CustomerAssetSummary(ctx context.Context, tx *gorm.DB, operator, customer string, at time.Time) (CustomerAssetSummary, error) {
	var result CustomerAssetSummary
	if !validPortalScope(operator, customer) || customer == "" {
		return result, ErrScope
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		// Delivery and invocation use the database clock. A live balance must
		// use the same clock, even when an API host is a little ahead/behind.
		// An explicit time remains available for historical/domain queries.
		if at.IsZero() {
			if err := t.Raw("SELECT clock_timestamp()").Row().Scan(&at); err != nil {
				return err
			}
		}

		if err := t.Raw(`SELECT
			COALESCE(sum(CASE WHEN g.effective_at<=? AND (g.expires_at IS NULL OR g.expires_at>?) THEN a.balance-a.held ELSE 0 END),0)::text AS available_atg,
			COALESCE(sum(a.held),0)::text AS held_atg,
			COALESCE(sum(g.expired),0)::text AS expired_atg
			FROM ledger_grants g JOIN ledger_accounts a ON a.id=g.account_id
			WHERE g.operator_id=? AND g.customer_id=?`, at, at, operator, customer).Scan(&result).Error; err != nil {
			return err
		}
		var sources struct{ PurchasedRemainingATG, GiftRemainingATG Decimal }
		err := t.Raw(`SELECT
 COALESCE(sum(CASE WHEN src.kind='paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS purchased_remaining_atg,
			COALESCE(sum(CASE WHEN src.kind<>'paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS gift_remaining_atg
			FROM ledger_grant_sources src JOIN ledger_grants g ON g.id=src.grant_id
			WHERE g.operator_id=? AND g.customer_id=? AND g.effective_at<=?
			AND (g.expires_at IS NULL OR g.expires_at>?)`, operator, customer, at, at).Scan(&sources).Error
		result.PurchasedRemainingATG = sources.PurchasedRemainingATG
		result.GiftRemainingATG = sources.GiftRemainingATG
		return err
	})
	return result, err
}

func (s *Service) FindAccount(ctx context.Context, tx *gorm.DB, operator, owner, purpose string) (Account, error) {
	var result Account
	if !validPortalScope(operator, owner) || owner == "" || purpose == "" {
		return result, ErrScope
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("operator_id=? AND owner_id=? AND purpose=?", operator, owner, purpose).Take(&result).Error
	})
	return result, err
}
func (s *Service) ListOperatorAccounts(ctx context.Context, tx *gorm.DB, operator, owner, cursor, query string, limit int, statuses ...string) ([]Account, error) {
	if !validPortalScope(operator, owner) || limit < 1 || limit > 100 {
		return nil, ErrScope
	}
	if cursor != "" {
		if _, err := uuid.Parse(cursor); err != nil {
			return nil, ErrScope
		}
	}
	var result []Account
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Where("operator_id=?", operator).Order("id").Limit(limit)
		if len(statuses) > 0 && statuses[0] != "" && statuses[0] != "open" {
			q = q.Where("1=0")
		}
		if owner != "" {
			q = q.Where("owner_id=?", owner)
		}
		if cursor != "" {
			q = q.Where("id>?", cursor)
		}
		if query != "" {
			q = q.Where("(id::text ILIKE ? OR owner_id::text ILIKE ? OR purpose ILIKE ?)", "%"+query+"%", "%"+query+"%", "%"+query+"%")
		}
		return q.Find(&result).Error
	})
	return result, err
}
func (s *Service) ListJournals(ctx context.Context, tx *gorm.DB, operator, owner, cursor string, limit int) ([]Journal, error) {
	if !validPortalScope(operator, owner) || limit < 1 || limit > 100 {
		return nil, ErrScope
	}
	if cursor != "" {
		if _, err := uuid.Parse(cursor); err != nil {
			return nil, ErrScope
		}
	}
	var result []Journal
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Where("operator_id=?", operator).Order("id").Limit(limit)
		if cursor != "" {
			q = q.Where("id>?", cursor)
		}
		if owner != "" {
			q = q.Where(`EXISTS (SELECT 1 FROM ledger_entries e JOIN ledger_accounts a ON a.id=e.account_id WHERE e.journal_id=ledger_journals.id AND a.operator_id=? AND a.owner_id=?)`, operator, owner)
		}
		return q.Find(&result).Error
	})
	return result, err
}
func (s *Service) JournalEntries(ctx context.Context, tx *gorm.DB, operator, owner, journal string) ([]Entry, error) {
	if !validPortalScope(operator, owner) {
		return nil, ErrScope
	}
	if _, err := uuid.Parse(journal); err != nil {
		return nil, ErrScope
	}
	var result []Entry
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Table("ledger_entries e").Select("e.*").Joins("JOIN ledger_journals j ON j.id=e.journal_id").Joins("JOIN ledger_accounts a ON a.id=e.account_id").Where("j.operator_id=? AND a.operator_id=? AND j.id=?", operator, operator, journal).Order("e.id")
		if owner != "" {
			q = q.Where("a.owner_id=?", owner)
		}
		return q.Find(&result).Error
	})
	return result, err
}

type GrantRemaining struct {
	GrantID               string
	PurchasedATG, GiftATG Decimal
}

// GrantRemainders returns exact remaining source balances, including cancellation.
func (s *Service) GrantRemainders(ctx context.Context, tx *gorm.DB, operator, customer string, ids []string) ([]GrantRemaining, error) {
	if !validPortalScope(operator, customer) || customer == "" || len(ids) > 100 {
		return nil, ErrScope
	}
	for _, id := range ids {
		if _, e := uuid.Parse(id); e != nil {
			return nil, ErrScope
		}
	}
	rows := []GrantRemaining{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`SELECT src.grant_id,
 COALESCE(sum(CASE WHEN src.kind='paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS purchased_atg,
 COALESCE(sum(CASE WHEN src.kind<>'paid' THEN src.quantity-src.consumed-src.expired-src.cancelled ELSE 0 END),0)::text AS gift_atg
 FROM ledger_grant_sources src JOIN ledger_grants g ON g.id=src.grant_id
 WHERE g.operator_id=? AND g.customer_id=? AND g.id IN ? GROUP BY src.grant_id`, operator, customer, ids).Scan(&rows).Error
	})
	return rows, e
}
