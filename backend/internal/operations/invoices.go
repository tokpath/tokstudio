package operations

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strings"
	"time"
)

type InvoiceBasis struct {
	ReceiptID           string          `json:"receipt_id"`
	EligibleMoney         ledger.Decimal  `json:"eligible_money"`
	AdjustmentReference string          `json:"adjustment_reference"`
	ReviewRequired      bool            `json:"review_required"`
	Evidence            string          `json:"evidence"`
	ReceiptMoney          *ledger.Decimal `json:"receipt_money,omitempty"`
	TopUpMoney            ledger.Decimal  `json:"top_up_money"`
	PurchaseMoney         ledger.Decimal  `json:"purchase_money"`
	ConsumptionMoney      ledger.Decimal  `json:"consumption_money"`
	GlobalEligibleMoney   *ledger.Decimal `json:"global_eligible_money,omitempty"`
}
type InvoiceSource interface {
	InvoiceBasis(context.Context, *gorm.DB, string, string, string) (InvoiceBasis, error)
}
type InvoiceLine struct {
	ReceiptID string         `json:"receipt_id"`
	AmountMoney ledger.Decimal `json:"amount_money"`
}
type InvoiceDetails struct {
	Title   string `json:"title"`
	TaxID   string `json:"tax_id"`
	Email   string `json:"email"`
	Address string `json:"address,omitempty"`
}
type Invoice struct {
	ID                string         `json:"id"`
	OperatorID        string         `json:"operator_id"`
	CustomerID        string         `json:"customer_id,omitempty" gorm:"default:null"`
	PartyKind         string         `json:"party_kind" gorm:"default:customer"`
	BuyerOperatorID   *string        `json:"buyer_operator_id,omitempty"`
	ReplacesInvoiceID *string        `json:"replaces_invoice_id,omitempty"`
	BusinessKey       string         `json:"business_key"`
	Fingerprint       string         `json:"-"`
	AmountMoney         ledger.Decimal `json:"amount_money"`
	State             string         `json:"state"`
	DetailsCiphertext string         `json:"-"`
	CreatedAt         time.Time      `json:"created_at"`
	InvoiceReference  string         `json:"invoice_reference,omitempty"`
	EvidenceReference string         `json:"evidence_reference,omitempty"`
	HandledBy         *string        `json:"handled_by,omitempty"`
	HandledAt         *time.Time     `json:"handled_at,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	PolicyStage       string         `json:"policy_stage"`
	PolicyVersion     int64          `json:"policy_version"`
	PolicyRatio       ledger.Decimal `json:"policy_ratio"`
}

func (Invoice) TableName() string { return "operations_invoices" }

// RequestInvoice reserves capacity against original external receipt identities.
// Funding that later buys a package keeps the same receipt identity, so it cannot
// create a second invoice capacity at another step of the purchase lifecycle.
func (s *Service) RequestInvoice(ctx context.Context, customer, operator, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	return s.requestInvoice(ctx, "customer", customer, operator, key, lines, details, func(t *gorm.DB) error { _, e := s.self(ctx, t, customer, operator); return e })
}
func (s *Service) requestInvoice(ctx context.Context, partyKind, party, operator, key string, lines []InvoiceLine, details InvoiceDetails, authorize func(*gorm.DB) error) (Invoice, error) {
	out := Invoice{}
	if partyKind == "customer" && s.Invoices == nil || partyKind == "procurement" && s.ProcurementInvoices == nil {
		return out, ErrNotReady
	}
	if key == "" || len(lines) == 0 || len(lines) > 100 || strings.TrimSpace(details.Title) == "" || len(details.Title) > 300 || len(details.TaxID) > 100 || len(details.Address) > 1000 {
		return out, ErrInvalid
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].ReceiptID < lines[j].ReceiptID })
	amount := ledger.Zero()
	for i, l := range lines {
		if !validID(l.ReceiptID) || l.AmountMoney.Sign() <= 0 || !l.AmountMoney.Valid() || (i > 0 && lines[i-1].ReceiptID == l.ReceiptID) {
			return out, ErrInvalid
		}
		amount = amount.Add(l.AmountMoney)
	}
	if !amount.Valid() {
		return out, ErrInvalid
	}
	fp := hash(struct {
		Lines   []InvoiceLine
		Details InvoiceDetails
	}{lines, details})
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := authorize(t); e != nil {
			return e
		}
		if e := lock(t, "invoice:"+operator+partyKind+party+key); e != nil {
			return e
		}
		partyColumn := "customer_id"
		if partyKind == "procurement" {
			partyColumn = "buyer_operator_id"
		}
		q := t.Where("operator_id=? AND party_kind=? AND "+partyColumn+"=? AND business_key=?", operator, partyKind, party, key).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		var policy InvoicePolicy
		if e := t.First(&policy, "operator_id=? AND party_kind=?", operator, partyKind).Error; e != nil {
			return ErrNotReady
		}
		if !policy.Enabled {
			return ErrNotReady
		}
		for _, l := range lines {
			if e := lock(t, "invoice-source:"+operator+l.ReceiptID); e != nil {
				return e
			}
			basis, e := s.invoiceBasis(ctx, t, Invoice{PartyKind: partyKind, CustomerID: party, BuyerOperatorID: &party, OperatorID: operator}, l.ReceiptID)
			if e != nil {
				return e
			}
			if basis.ReviewRequired || basis.ReceiptID != l.ReceiptID {
				return ErrConflict
			}
			var used ledger.Decimal
			if e = t.Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, operator, l.ReceiptID).Row().Scan(&used); e != nil {
				return e
			}
			eligible, e := eligibleInvoiceAmount(basis, policy.Stage, policy.MaximumRatio)
			if e != nil {
				return e
			}
			global := eligible
			if basis.GlobalEligibleMoney != nil {
				global = *basis.GlobalEligibleMoney
			}
			var partyUsed ledger.Decimal
			if e = t.Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND i.party_kind=? AND i.`+partyColumn+`=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, operator, partyKind, party, l.ReceiptID).Row().Scan(&partyUsed); e != nil {
				return e
			}
			if used.Add(l.AmountMoney).Cmp(global) > 0 || partyUsed.Add(l.AmountMoney).Cmp(eligible) > 0 {
				return ErrConflict
			}
		}
		cipher, e := s.seal(details)
		if e != nil {
			return e
		}
		out = Invoice{ID: uuid.NewString(), OperatorID: operator, PartyKind: partyKind, BusinessKey: key, Fingerprint: fp, AmountMoney: amount, State: "requested", PolicyStage: policy.Stage, PolicyVersion: policy.Version, PolicyRatio: policy.MaximumRatio, DetailsCiphertext: cipher, CreatedAt: time.Now().UTC()}
		if partyKind == "procurement" {
			out.BuyerOperatorID = &party
		} else {
			out.CustomerID = party
		}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		for _, l := range lines {
			if e = t.Table("operations_invoice_sources").Create(map[string]any{"invoice_id": out.ID, "receipt_id": l.ReceiptID, "amount_money": l.AmountMoney.String()}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) ListInvoices(ctx context.Context, actor, operator string, customer bool, search ...string) ([]Invoice, error) {
	out := []Invoice{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		q := t.Where("operator_id=?", operator)
		if customer {
			if _, e := s.self(ctx, t, actor, operator); e != nil {
				return e
			}
			q = q.Where("party_kind='customer' AND customer_id=?", actor)
		} else {
			if e := s.require(ctx, t, actor, operator, "invoices.manage"); e != nil {
				return e
			}
		}
		return invoiceSearch(q, search).Order("created_at DESC,id DESC").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s *Service) InvoiceDetails(ctx context.Context, actor, operator, id string) (InvoiceDetails, error) {
	var out InvoiceDetails
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		var row Invoice
		if e := t.First(&row, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		return s.open(row.DetailsCiphertext, &out)
	})
	return out, e
}
func (s *Service) RegisterInvoice(ctx context.Context, actor, operator, id, state, invoiceRef, evidence, reason string) (Invoice, error) {
	out := Invoice{}
	if state != "issued" && state != "rejected" && state != "voided" {
		return out, ErrInvalid
	}
	if reason == "" || (state == "issued" && (invoiceRef == "" || evidence == "")) {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		if out.State == state && out.InvoiceReference == invoiceRef && out.EvidenceReference == evidence {
			return nil
		}
		if (state == "issued" && out.State != "requested") || (state == "rejected" && out.State != "requested") || (state == "voided" && out.State != "issued" && out.State != "review_required") {
			return ErrConflict
		}
		if state == "issued" {
			if out.PartyKind == "procurement" && s.ProcurementInvoices == nil || out.PartyKind != "procurement" && s.Invoices == nil {
				return ErrNotReady
			}
			var files int64
			if e := t.Model(&InvoiceFile{}).Where("invoice_id=? AND state='ready'", id).Count(&files).Error; e != nil {
				return e
			}
			if files == 0 {
				return ErrNotReady
			}
			lines := []InvoiceLine{}
			if e := t.Table("operations_invoice_sources").Where("invoice_id=?", id).Order("receipt_id").Find(&lines).Error; e != nil {
				return e
			}
			for _, l := range lines {
				basis, e := s.invoiceBasis(ctx, t, out, l.ReceiptID)
				if e != nil {
					return e
				}
				eligible, e := eligibleInvoiceAmount(basis, out.PolicyStage, out.PolicyRatio)
				if e != nil {
					return e
				}
				globalReserved, partyReserved, e := s.invoiceReserved(ctx, t, out, l.ReceiptID)
				if e != nil {
					return e
				}
				global := eligible
				if basis.GlobalEligibleMoney != nil {
					global = *basis.GlobalEligibleMoney
				}
				if basis.ReviewRequired || partyReserved.Cmp(eligible) > 0 || globalReserved.Cmp(global) > 0 {
					return ErrConflict
				}
			}
		}
		var now time.Time
		if e := t.Raw("SELECT clock_timestamp()").Scan(&now).Error; e != nil {
			return e
		}
		previousState := out.State
		out.State = state
		out.InvoiceReference = invoiceRef
		out.EvidenceReference = evidence
		out.Reason = reason
		out.HandledBy = &actor
		out.HandledAt = &now
		if e := t.Save(&out).Error; e != nil {
			return e
		}
		return t.Exec(`INSERT INTO operations_invoice_transitions(id,invoice_id,from_state,to_state,invoice_reference,evidence_reference,reason,actor_id,at) VALUES(?,?,?,?,?,?,?,?,?)`, uuid.NewString(), out.ID, previousState, state, invoiceRef, evidence, reason, actor, now).Error
	})
	return out, e
}

// ReviewInvoices flags external refunds/corrections without changing tax facts.
// An issued invoice remains reserved until a documented void is registered.
func (s *Service) ReviewInvoices(ctx context.Context, operator string, limit int) (int, error) {
	if s.Invoices == nil && s.ProcurementInvoices == nil {
		return 0, ErrNotReady
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	count := 0
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		rows := []Invoice{}
		if e := t.Where("operator_id=? AND state IN ('requested','issued')", operator).Order("last_reviewed_at,id").Limit(limit).Find(&rows).Error; e != nil {
			return e
		}
		for _, r := range rows {
			if e := t.Model(&Invoice{}).Where("id=?", r.ID).Update("last_reviewed_at", gorm.Expr("clock_timestamp()")).Error; e != nil {
				return e
			}
			lines := []InvoiceLine{}
			if e := t.Table("operations_invoice_sources").Where("invoice_id=?", r.ID).Find(&lines).Error; e != nil {
				return e
			}
			review := false
			for _, l := range lines {
				basis, e := s.invoiceBasis(ctx, t, r, l.ReceiptID)
				if e != nil {
					return e
				}
				globalReserved, partyReserved, e := s.invoiceReserved(ctx, t, r, l.ReceiptID)
				if e != nil {
					return e
				}
				eligible, e := eligibleInvoiceAmount(basis, r.PolicyStage, r.PolicyRatio)
				if e != nil {
					return e
				}
				global := eligible
				if basis.GlobalEligibleMoney != nil {
					global = *basis.GlobalEligibleMoney
				}
				if basis.ReviewRequired || partyReserved.Cmp(eligible) > 0 || globalReserved.Cmp(global) > 0 {
					review = true
				}
			}
			if review {
				if e := t.Model(&Invoice{}).Where("id=?", r.ID).Update("state", "review_required").Error; e != nil {
					return e
				}
				count++
			}
		}
		return nil
	})
	return count, e
}

func (s *Service) invoiceReserved(ctx context.Context, tx *gorm.DB, i Invoice, receipt string) (ledger.Decimal, ledger.Decimal, error) {
	var global, party ledger.Decimal
	if e := tx.WithContext(ctx).Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, i.OperatorID, receipt).Row().Scan(&global); e != nil {
		return global, party, e
	}
	column, owner := "customer_id", i.CustomerID
	if i.PartyKind == "procurement" {
		column = "buyer_operator_id"
		if i.BuyerOperatorID == nil {
			return global, party, ErrInvalid
		}
		owner = *i.BuyerOperatorID
	}
	e := tx.WithContext(ctx).Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND i.party_kind=? AND i.`+column+`=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, i.OperatorID, i.PartyKind, owner, receipt).Row().Scan(&party)
	return global, party, e
}
