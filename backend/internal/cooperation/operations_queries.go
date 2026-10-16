package cooperation

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type IntegrityFact struct {
	Reference string         `json:"reference"`
	Kind      string         `json:"kind"`
	Expected  ledger.Decimal `json:"expected"`
	Actual    ledger.Decimal `json:"actual"`
}

func (s *Service) OperationalIntegrity(ctx context.Context, tx *gorm.DB, operator string) ([]IntegrityFact, error) {
	if _, e := uuid.Parse(operator); e != nil {
		return nil, e
	}
	out := []IntegrityFact{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`SELECT p.buyer_id::text AS reference,'procurement_lots' AS kind,COALESCE(l.remaining,0)::text AS expected,p.quantity::text AS actual
 FROM cooperation_procurement_positions p LEFT JOIN(SELECT buyer_id,sum(remaining)remaining FROM cooperation_procurement_lots GROUP BY buyer_id)l ON l.buyer_id=p.buyer_id WHERE p.buyer_id=? AND p.quantity<>COALESCE(l.remaining,0)
 UNION ALL SELECT p.buyer_id::text,'procurement_account',a.balance::text,p.quantity::text FROM cooperation_procurement_positions p JOIN ledger_accounts a ON a.id=p.account_id WHERE p.buyer_id=? AND p.quantity<>a.balance`, operator, operator).Scan(&out).Error
	})
	return out, e
}
func (s *Service) OperationalLots(ctx context.Context, tx *gorm.DB, operator string, limit int) ([]ProcurementLot, error) {
	if _, e := uuid.Parse(operator); e != nil || limit < 1 || limit > 10001 {
		return nil, ErrConflict
	}
	out := []ProcurementLot{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("buyer_id=?", operator).Order("delivered_at,id").Limit(limit).Find(&out).Error
	})
	return out, e
}
