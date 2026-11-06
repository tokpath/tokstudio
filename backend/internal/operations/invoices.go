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

// InvoiceBasis belongs to one completed purchase, regardless of its payment mix.
type InvoiceBasis struct {
	OrderID        string         `json:"order_id"`
	EligibleMoney  ledger.Decimal `json:"eligible_money"`
	ReviewRequired bool           `json:"review_required"`
	Evidence       string         `json:"evidence"`
}
type InvoiceSource interface {
	InvoiceBasis(context.Context, *gorm.DB, string, string, string) (InvoiceBasis, error)
}
type InvoiceLine struct {
	OrderID string `json:"order_id"`
	// Amount is a server-owned snapshot. Clients only select orders.
	AmountMoney ledger.Decimal `json:"-"`
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
	AmountMoney       ledger.Decimal `json:"amount_money"`
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

// RequestInvoice invoices each completed order once at its actual paid value.
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
	lines = append([]InvoiceLine(nil), lines...)
	sort.Slice(lines, func(i, j int) bool { return lines[i].OrderID < lines[j].OrderID })
	ids := make([]string, len(lines))
	for i, l := range lines {
		if !validID(l.OrderID) || i > 0 && lines[i-1].OrderID == l.OrderID {
			return out, ErrInvalid
		}
		ids[i] = l.OrderID
	}
	fp := hash(struct {
		Orders  []string
		Details InvoiceDetails
	}{ids, details})
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
		amount := ledger.Zero()
		for i, l := range lines {
			if e := lock(t, "invoice-order:"+operator+":"+partyKind+":"+l.OrderID); e != nil {
				return e
			}
			basis, e := s.invoiceBasis(ctx, t, Invoice{PartyKind: partyKind, CustomerID: party, BuyerOperatorID: &party, OperatorID: operator}, l.OrderID)
			if e != nil {
				return e
			}
			if basis.OrderID != l.OrderID || basis.ReviewRequired || basis.EligibleMoney.Sign() <= 0 || !basis.EligibleMoney.Valid() {
				return ErrConflict
			}
			var count int64
			if e = t.Table("operations_invoice_orders src").Joins("JOIN operations_invoices i ON i.id=src.invoice_id").Where("i.operator_id=? AND i.party_kind=? AND src.order_id=? AND i.state NOT IN ('rejected','voided')", operator, partyKind, l.OrderID).Count(&count).Error; e != nil {
				return e
			}
			if count > 0 {
				return ErrInvoiceOrderUsed
			}
			lines[i].AmountMoney = basis.EligibleMoney
			amount = amount.Add(basis.EligibleMoney)
		}
		if !amount.Valid() {
			return ErrInvalid
		}
		cipher, e := s.seal(details)
		if e != nil {
			return e
		}
		out = Invoice{ID: uuid.NewString(), OperatorID: operator, PartyKind: partyKind, BusinessKey: key, Fingerprint: fp, AmountMoney: amount, State: "requested", PolicyStage: "purchase", PolicyVersion: policy.Version, PolicyRatio: ledger.FromInt64(1), DetailsCiphertext: cipher, CreatedAt: time.Now().UTC()}
		if partyKind == "procurement" {
			out.BuyerOperatorID = &party
		} else {
			out.CustomerID = party
		}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		for _, l := range lines {
			if e = t.Table("operations_invoice_orders").Create(map[string]any{"invoice_id": out.ID, "order_id": l.OrderID, "amount_money": l.AmountMoney.String()}).Error; e != nil {
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
				return ErrInvoiceDocument
			}
			if valid, e := s.invoiceOrdersValid(ctx, t, out); e != nil {
				return e
			} else if !valid {
				return ErrConflict
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
			valid, e := s.invoiceOrdersValid(ctx, t, r)
			if e != nil {
				return e
			}
			review := !valid
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

func (s *Service) invoiceOrdersValid(ctx context.Context, tx *gorm.DB, i Invoice) (bool, error) {
	var lines []InvoiceLine
	if e := tx.Table("operations_invoice_orders").Where("invoice_id=?", i.ID).Order("order_id").Find(&lines).Error; e != nil {
		return false, e
	}
	if len(lines) == 0 {
		return false, nil
	}
	for _, l := range lines {
		var duplicate int64
		if err := tx.Table("operations_invoice_orders s").Joins("JOIN operations_invoices other ON other.id=s.invoice_id").Where("s.order_id=? AND other.id<>? AND other.operator_id=? AND other.party_kind=? AND other.state NOT IN ('rejected','voided')", l.OrderID, i.ID, i.OperatorID, i.PartyKind).Count(&duplicate).Error; err != nil {
			return false, err
		}
		if duplicate > 0 {
			return false, nil
		}
		basis, e := s.invoiceBasis(ctx, tx, i, l.OrderID)
		if e != nil {
			return false, e
		}
		if basis.ReviewRequired || basis.OrderID != l.OrderID || basis.EligibleMoney.Cmp(l.AmountMoney) != 0 {
			return false, nil
		}
	}
	return true, nil
}
