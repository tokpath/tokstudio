package payment

import (
	"testing"
	"time"
)

func TestManualRefundLegacyProjection(t *testing.T) {
	when := time.Date(2026, 10, 10, 2, 26, 0, 0, time.UTC)
	zero := time.Time{}
	partial := int64(2000)
	base := orderRow{Adapter: AdapterManual, Status: StatusRefunded, AmountMinor: 12345, CreditMinor: 2000000, Currency: "CNY", RefundedAt: &when, RefundRecordedBy: "finance"}
	for _, tc := range []struct {
		name   string
		change func(*orderRow)
		infer  bool
	}{
		{name: "registered legacy full refund", infer: true},
		{name: "no actor", change: func(row *orderRow) { row.RefundRecordedBy = "" }},
		{name: "no actual time", change: func(row *orderRow) { row.RefundedAt = nil }},
		{name: "zero actual time", change: func(row *orderRow) { row.RefundedAt = &zero }},
		{name: "not refunded", change: func(row *orderRow) { row.Status = StatusPaid }},
		{name: "online partial refund", change: func(row *orderRow) {
			row.Adapter = AdapterStripe
			row.RefundStatus = StatusRefundPartial
			row.RefundAmountMinor = &partial
		}},
		{name: "online missing facts", change: func(row *orderRow) { row.Adapter = AdapterStripe }},
		{name: "conflicting partial amount", change: func(row *orderRow) { row.RefundAmountMinor = &partial }},
		{name: "conflicting failed status", change: func(row *orderRow) { row.RefundStatus = StatusRefundFailed }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := base
			if tc.change != nil {
				tc.change(&row)
			}
			view := orderView(row)
			if tc.infer {
				if view.RefundAmountMinor == nil || *view.RefundAmountMinor != row.AmountMinor || view.RefundStatus != StatusRefunded || view.Currency != "CNY" || view.CreditMinor != 2000000 {
					t.Fatalf("lost original cash/currency/credit distinction: %+v", view)
				}
				if row.RefundAmountMinor != nil || row.RefundStatus != "" {
					t.Fatal("projection changed the legacy row")
				}
			} else if view.RefundAmountMinor != row.RefundAmountMinor || view.RefundStatus != row.RefundStatus {
				t.Fatalf("invented or changed refund facts: %+v", view)
			}
		})
	}
}
