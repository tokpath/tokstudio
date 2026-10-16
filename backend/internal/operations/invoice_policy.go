package operations

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

type InvoicePolicy struct {
	OperatorID     string         `gorm:"primaryKey" json:"operator_id"`
	PartyKind      string         `gorm:"primaryKey" json:"party_kind"`
	Enabled        bool           `json:"enabled"`
	Stage          string         `json:"stage"`
	MaximumRatio   ledger.Decimal `json:"maximum_ratio"`
	RulesReference string         `json:"rules_reference"`
	Version        int64          `json:"version"`
	UpdatedBy      string         `json:"updated_by"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (InvoicePolicy) TableName() string { return "operations_invoice_policies" }
func (s *Service) SaveInvoicePolicy(ctx context.Context, actor string, p InvoicePolicy, expected int64) (InvoicePolicy, error) {
	if p.PartyKind == "" {
		p.PartyKind = "customer"
	}
	if p.PartyKind != "customer" && p.PartyKind != "procurement" {
		return p, ErrInvalid
	}
	if (p.Stage != "receipt" && p.Stage != "top_up" && p.Stage != "purchase" && p.Stage != "consumption") || p.RulesReference == "" || p.MaximumRatio.Sign() < 0 || p.MaximumRatio.Cmp(ledger.FromInt64(1)) > 0 || !p.MaximumRatio.Valid() {
		return p, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, p.OperatorID, "invoices.manage"); e != nil {
			return e
		}
		if e := lock(t, "invoice-policy:"+p.OperatorID+":"+p.PartyKind); e != nil {
			return e
		}
		var old InvoicePolicy
		q := t.Where("operator_id=? AND party_kind=?", p.OperatorID, p.PartyKind).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if (q.RowsAffected == 0 && expected != 0) || (q.RowsAffected > 0 && old.Version != expected) {
			return ErrConflict
		}
		p.Version = expected + 1
		p.UpdatedBy = actor
		if e := t.Raw("SELECT clock_timestamp()").Row().Scan(&p.UpdatedAt); e != nil {
			return e
		}
		snapshot, e := json.Marshal(p)
		if e != nil {
			return e
		}
		if e = t.Table("operations_invoice_policy_versions").Create(map[string]any{"operator_id": p.OperatorID, "party_kind": p.PartyKind, "version": p.Version, "snapshot": json.RawMessage(snapshot), "created_at": p.UpdatedAt}).Error; e != nil {
			return e
		}
		return t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "party_kind"}}, UpdateAll: true}).Create(&p).Error
	})
	return p, e
}

func (s *Service) ListInvoicePolicyVersions(ctx context.Context, actor, issuer, partyKind string) ([]InvoicePolicy, error) {
	if partyKind != "customer" && partyKind != "procurement" {
		return nil, ErrInvalid
	}
	out := []InvoicePolicy{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, issuer, "invoices.manage"); e != nil {
			return e
		}
		var rows []struct{ Snapshot json.RawMessage }
		if e := tx.Table("operations_invoice_policy_versions").Where("operator_id=? AND party_kind=?", issuer, partyKind).Order("version DESC").Limit(100).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			var p InvoicePolicy
			if e := json.Unmarshal(row.Snapshot, &p); e != nil {
				return e
			}
			out = append(out, p)
		}
		return nil
	})
	return out, e
}
func (s *Service) GetInvoicePolicy(ctx context.Context, actor, operator string, customer bool) (InvoicePolicy, error) {
	out := InvoicePolicy{OperatorID: operator, PartyKind: "customer", Enabled: false}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if customer {
			if _, e := s.self(ctx, t, actor, operator); e != nil {
				return e
			}
		} else {
			if e := s.require(ctx, t, actor, operator, "invoices.manage"); e != nil {
				return e
			}
		}
		return t.Where("operator_id=? AND party_kind='customer'", operator).Limit(1).Find(&out).Error
	})
	return out, e
}
func (s *Service) ListInvoicePolicies(ctx context.Context, actor, operator string) ([]InvoicePolicy, error) {
	out := []InvoicePolicy{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		return tx.Where("operator_id=?", operator).Order("party_kind").Find(&out).Error
	})
	return out, e
}
func (s *Service) GetIssuerInvoicePolicy(ctx context.Context, actor, issuer, partyKind string) (InvoicePolicy, error) {
	out := InvoicePolicy{OperatorID: issuer, PartyKind: partyKind}
	if partyKind != "customer" && partyKind != "procurement" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, issuer, "invoices.manage"); e != nil {
			return e
		}
		return tx.Where("operator_id=? AND party_kind=?", issuer, partyKind).Limit(1).Find(&out).Error
	})
	return out, e
}
func eligibleInvoiceAmount(b InvoiceBasis, stage string, ratio ledger.Decimal) (ledger.Decimal, error) {
	value := b.EligibleUSD
	switch stage {
	case "receipt":
		if b.ReceiptUSD != nil {
			value = *b.ReceiptUSD
		}
	case "top_up":
		value = b.TopUpUSD
	case "purchase":
		value = b.PurchaseUSD
	case "consumption":
		value = b.ConsumptionUSD
	default:
		return ledger.Zero(), ErrInvalid
	}
	if value.Sign() < 0 {
		return ledger.Zero(), ErrInvalid
	}
	out, _ := ledger.Quantize(new(big.Rat).Mul(value.Rat(), ratio.Rat()))
	return out, nil
}
