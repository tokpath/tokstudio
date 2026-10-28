package operations

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
	"time"
)

type Finding struct {
	Domain    string         `json:"domain"`
	Kind      string         `json:"kind"`
	Reference string         `json:"reference"`
	Unit      string         `json:"unit"`
	Expected  ledger.Decimal `json:"expected"`
	Actual    ledger.Decimal `json:"actual"`
}
type ExportQuery struct {
	Kind string    `json:"kind"`
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}
type Source interface {
	Reconcile(context.Context, *gorm.DB, string) ([]Finding, error)
	Metrics(context.Context, *gorm.DB, string, time.Time) (map[string]ledger.Decimal, error)
	Export(context.Context, *gorm.DB, string, ExportQuery) ([]json.RawMessage, error)
	HasAdjustment(context.Context, *gorm.DB, string, string) (bool, error)
}
type ResultRetention interface {
	PurgeExpiredResults(context.Context, time.Time, int) (int64, error)
}

// DomainSource uses only public domain reads. Missing dependencies fail the
// report; unknown costs are explicit metrics rather than zero cost estimates.
type DomainSource struct {
	Ledger      *ledger.Service
	Cooperation *cooperation.Service
	Usage       *usage.Service
	Commerce    *commerce.Service
	Refunds     RefundFinancialSource
	Payments    PendingPaymentSource
}

func (d DomainSource) ready() error {
	if d.Ledger == nil || d.Cooperation == nil || d.Usage == nil {
		return ErrNotReady
	}
	return nil
}
func (d DomainSource) Reconcile(ctx context.Context, tx *gorm.DB, operator string) ([]Finding, error) {
	if e := d.ready(); e != nil {
		return nil, e
	}
	out := []Finding{}
	l, e := d.Ledger.OperationalIntegrity(ctx, tx, operator)
	if e != nil {
		return nil, e
	}
	for _, r := range l {
		out = append(out, Finding{"ledger", r.Kind, r.Reference, r.Unit, r.Expected, r.Actual})
	}
	c, e := d.Cooperation.OperationalIntegrity(ctx, tx, operator)
	if e != nil {
		return nil, e
	}
	for _, r := range c {
		out = append(out, Finding{"cooperation", r.Kind, r.Reference, "ATG", r.Expected, r.Actual})
	}
	return out, nil
}
func (d DomainSource) Metrics(ctx context.Context, tx *gorm.DB, operator string, at time.Time) (map[string]ledger.Decimal, error) {
	if e := d.ready(); e != nil {
		return nil, e
	}
	if d.Payments == nil {
		return nil, ErrNotReady
	}
	funds, e := d.Payments.PendingReceiptObligations(ctx, tx, operator, nil)
	if e != nil {
		return nil, e
	}
	valuationCount := int64(0)
	for _, fact := range funds {
		if fact.State == "awaiting_valuation" {
			valuationCount++
		}
	}
	coverage, e := d.Cooperation.Coverage(ctx, tx, operator, at)
	if e != nil {
		return nil, e
	}
	pending, unverified, e := d.Usage.OperationalPending(ctx, tx, operator)
	if e != nil {
		return nil, e
	}
	oversellUsed := coverage.Required.Add(coverage.Pending).Sub(coverage.ProcurementAvailable)
	if oversellUsed.Sign() < 0 {
		oversellUsed = ledger.Zero()
	}
	return map[string]ledger.Decimal{"sales_space_atg": coverage.Space, "procurement_available_atg": coverage.ProcurementAvailable, "oversell_used_atg": oversellUsed, "pending_usage": ledger.FromInt64(pending), "unverified_costs": ledger.FromInt64(unverified), "pending_receipt_facts": ledger.FromInt64(int64(len(funds))), "awaiting_valuation_receipts": ledger.FromInt64(valuationCount)}, nil
}
func jsonRows[T any](rows []T) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		b, e := json.Marshal(r)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, nil
}
func (d DomainSource) Export(ctx context.Context, tx *gorm.DB, operator string, q ExportQuery) ([]json.RawMessage, error) {
	if e := d.ready(); e != nil {
		return nil, e
	}
	switch q.Kind {
	case "ledger":
		rows, e := d.Ledger.OperationalEntries(ctx, tx, operator, "", q.From, q.To, 100001)
		if e != nil {
			return nil, e
		}
		return jsonRows(rows)
	case "requests":
		rows, e := d.Usage.OperationalRequests(ctx, tx, operator, "", q.From, q.To, 100001)
		if e != nil {
			return nil, e
		}
		return jsonRows(rows)
	case "procurement":
		rows, e := d.Cooperation.OperationalPurchaseOrders(ctx, tx, operator, "", q.From, q.To, 100001)
		if e != nil {
			return nil, e
		}
		return jsonRows(rows)
	case "financial":
		report, e := d.FinancialReport(ctx, tx, operator, q.From, q.To)
		if e != nil {
			return nil, e
		}
		return financialExportRows(report)
	case "orders", "payments":
		return d.salesExport(ctx, tx, operator, q)
	default:
		return nil, ErrInvalid
	}
}
func (d DomainSource) HasAdjustment(ctx context.Context, tx *gorm.DB, operator, id string) (bool, error) {
	if d.Ledger == nil {
		return false, ErrNotReady
	}
	return d.Ledger.HasAdjustment(ctx, tx, operator, id)
}
