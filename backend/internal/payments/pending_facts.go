package payments

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PendingReceiptObligation = ReceiptValuationView

func (s *Service) RecordedReceiptLinks(ctx context.Context, tx *gorm.DB, operator string, orderIDs []string) ([]ReceiptLink, error) {
	out := []ReceiptLink{}
	if tx == nil || operator == "" || len(orderIDs) > 1000 {
		return out, ErrInvalid
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, ErrInvalid
	}
	if orderIDs != nil && len(orderIDs) == 0 {
		return out, nil
	}
	q := tx.WithContext(ctx).Table("payments_verified_facts").Select("merchant_id,order_id,receipt_id,id AS fact_id").Where("operator_id=? AND kind='receipt' AND COALESCE(receipt_id,'')<>''", operator)
	if orderIDs != nil {
		q = q.Where("order_id IN ?", orderIDs)
	}
	e := q.Order("id").Scan(&out).Error
	return out, e
}

// UnsettledReceiptFunds delegates to the configured receipt domain's public
// projection. Payments never interprets or reads the commerce funding tables.
func (s *Service) UnsettledReceiptFunds(ctx context.Context, tx *gorm.DB, operator string, receiptIDs []string) ([]commerce.ReceiptFundObligation, error) {
	source, ok := s.Sink.(interface {
		UnsettledReceiptFunds(context.Context, *gorm.DB, string, []string) ([]commerce.ReceiptFundObligation, error)
	})
	if !ok {
		return nil, ErrNotConfigured
	}
	return source.UnsettledReceiptFunds(ctx, tx, operator, receiptIDs)
}

// PendingReceiptObligations is an internal exact-operator projection for
// lifecycle gates. Nil orderIDs means all of this operator; an empty non-nil
// slice means no orders. It deliberately ignores commercial order status.
func (s *Service) PendingReceiptObligations(ctx context.Context, tx *gorm.DB, operator string, orderIDs []string) ([]PendingReceiptObligation, error) {
	out := []PendingReceiptObligation{}
	if tx == nil || operator == "" {
		return out, ErrInvalid
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, ErrInvalid
	}
	if orderIDs != nil && len(orderIDs) == 0 {
		return out, nil
	}
	if len(orderIDs) > 1000 {
		return out, ErrInvalid
	}
	q := tx.WithContext(ctx).Table("payments_verified_facts").Where("operator_id=? AND kind='receipt' AND COALESCE(receipt_id,'')='' AND state IN ('awaiting_settlement','verified_pending_receipt','verified')", operator)
	if orderIDs != nil {
		q = q.Where("order_id IN ?", orderIDs)
	}
	var rows []receiptFactRow
	if e := q.Order("id").Find(&rows).Error; e != nil {
		return out, e
	}
	for _, row := range rows {
		var f Fact
		if json.Unmarshal(row.Fact, &f) != nil {
			return out, ErrInvalid
		}
		out = append(out, PendingReceiptObligation{FactID: f.ID, MerchantID: row.MerchantID, OrderID: row.OrderID, Driver: f.Driver, Money: f.Money, PaidAt: f.OccurredAt, ObservedAt: f.ObservedAt, State: row.State, Reason: row.ValuationReason})
	}
	return out, nil
}

// RunPendingFacts only repeats original lookups or replays already verified
// facts. It never creates a checkout, charges a wallet, or supplies a guessed FX
// rate. Claim scheduling uses the same database clock as initial fact storage.
func (s *Service) RunPendingFacts(ctx context.Context, limit int) ([]InboxAttempt, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []receiptFactRow
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Table("payments_verified_facts").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("kind='receipt' AND COALESCE(receipt_id,'')='' AND state IN ('awaiting_settlement','verified_pending_receipt','verified') AND fact_recovery_after<=clock_timestamp()").Order("fact_recovery_after,id").Limit(limit).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			if e := tx.Table("payments_verified_facts").Where("id=?", row.ID).Update("fact_recovery_after", gorm.Expr("clock_timestamp()+interval '1 minute'")).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := []InboxAttempt{}
	for _, row := range rows {
		work, cancel := context.WithTimeout(ctx, 20*time.Second)
		m, e := s.Merchant(work, row.OperatorID, row.MerchantID)
		if e == nil {
			var o Order
			o, e = s.loadOrder(work, m, row.OrderID)
			if e == nil {
				m, e = s.historicalMerchant(work, m, o)
			}
			if e == nil {
				var fact Fact
				if json.Unmarshal(row.Fact, &fact) != nil {
					e = ErrInvalid
				} else {
					fact.proof = true
					if row.State == "awaiting_settlement" && row.ValuationReason == "payment_time_unverified" && m.Driver != "manual" {
						driver := s.Drivers[m.Driver]
						if driver == nil {
							e = ErrUnsupported
						} else {
							fact, e = driver.Lookup(work, m, o)
						}
					}
					if e == nil {
						e = s.RecordFact(work, m, o, fact)
					}
				}
			}
		}
		cancel()
		attempt := InboxAttempt{MerchantID: row.MerchantID, EventID: "fact:" + row.ID, State: "processed"}
		if e != nil {
			attempt.State = "pending"
			attempt.ErrorCode = "receipt_reconciliation_pending"
			if errors.Is(e, ErrAwaitingValuation) {
				attempt.ErrorCode = "settlement_confirmation_required"
			}
		}
		out = append(out, attempt)
	}
	return out, nil
}
