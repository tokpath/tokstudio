package operations

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// Instructions are optional public guidance, never an eligibility gate.
type InvoiceInstructions struct {
	OperatorID   string    `gorm:"primaryKey" json:"operator_id"`
	Instructions string    `json:"instructions"`
	Version      int64     `json:"version"`
	UpdatedBy    string    `json:"updated_by"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (InvoiceInstructions) TableName() string { return "operations_invoice_instructions" }
func (s *Service) SaveInvoiceInstructions(ctx context.Context, actor, operator, instructions string, expected int64) (InvoiceInstructions, error) {
	out := InvoiceInstructions{OperatorID: operator, Instructions: strings.TrimSpace(instructions), UpdatedBy: actor}
	if len(out.Instructions) > 10000 {
		return out, ErrInvalid
	}
	err := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		if e := lock(tx, "invoice-instructions:"+operator); e != nil {
			return e
		}
		var old InvoiceInstructions
		q := tx.Where("operator_id=?", operator).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if old.Version != expected {
			return ErrConflict
		}
		out.Version = expected + 1
		if e := tx.Raw("SELECT clock_timestamp()").Row().Scan(&out.UpdatedAt); e != nil {
			return e
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}}, UpdateAll: true}).Create(&out).Error
	})
	return out, err
}
func (s *Service) GetInvoiceInstructions(ctx context.Context, actor, operator string, customer bool) (InvoiceInstructions, error) {
	out := InvoiceInstructions{OperatorID: operator}
	err := s.within(ctx, nil, func(tx *gorm.DB) error {
		if customer {
			if _, e := s.self(ctx, tx, actor, operator); e != nil {
				return e
			}
		} else if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		return tx.Where("operator_id=?", operator).Limit(1).Find(&out).Error
	})
	return out, err
}
