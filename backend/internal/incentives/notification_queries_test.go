package incentives

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPostgresNotificationAudienceUsesPersistedCustomersAndBeneficiariesWithoutSecrets(t *testing.T) {
	f := setup(t)
	a := f.register(t, "recipient-a@example.test", "")
	b := f.register(t, "recipient-b@example.test", a.InviteCode)
	f.qualify(t, a.Customer.ID, "qualify-recipient")
	f.buy(t, b.Customer.ID, "commission-source", "100", ActivitySelection{})
	consumption := f.consume(t, b.Customer.ID, "100")
	if e := f.s.ReverseEvent(f.ctx, nil, CorrectionInput{ActorStaffID: f.owner, OperatorID: f.operator, OriginalEventID: consumption[0].ID, BusinessKey: "proven-source-correction", Evidence: "private-sensitive-evidence", AmountMoney: d("10")}); e != nil {
		t.Fatal(e)
	}
	policy, e := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MinimumMoney: d("5"), FlatFeeMoney: d("1"), MethodReference: "private-bank-arrangement"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	w, e := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "notify-withdrawal", RecipientReference: "private-sensitive-bank-account", PolicyID: policy.ID, AmountMoney: d("10")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = f.s.MarkWithdrawalUnknown(f.ctx, nil, f.owner, f.operator, w.ID, "private-sensitive-bank-response"); e != nil {
			t.Fatal(e)
		}
	}
	var correction, unknown, qualification Event
	for kind, dest := range map[string]*Event{"consumption.corrected": &correction, "withdrawal.outcome_unknown": &unknown, "qualification.acquired": &qualification} {
		if e = f.db.Where("kind=?", kind).Order("created_at DESC,id DESC").First(dest).Error; e != nil {
			t.Fatal(e)
		}
	}
	var unknownCount int64
	if e = f.db.Model(&Event{}).Where("kind='withdrawal.outcome_unknown'").Count(&unknownCount).Error; e != nil || unknownCount != 1 {
		t.Fatalf("unknown replay generated duplicate %d %v", unknownCount, e)
	}
	e = f.db.Transaction(func(tx *gorm.DB) error {
		rows, e := f.s.NotificationEventAudience(f.ctx, tx, correction)
		if e != nil {
			return e
		}
		ids := map[string]bool{}
		for _, row := range rows {
			ids[row.RecipientID] = true
			if row.OperatorID != f.operator || row.Reference != correction.ID {
				t.Fatal(row)
			}
		}
		if len(ids) != 2 || !ids[a.Customer.ID] || !ids[b.Customer.ID] {
			t.Fatalf("correction omitted original beneficiary/customer %+v", rows)
		}
		for _, event := range []Event{unknown, qualification} {
			// Untrusted payload changes are ignored; the stored immutable event
			// supplies its actual aggregate and a safe amount projection.
			event.Payload = json.RawMessage(`{"recipient_id":"foreign","secret":"forged"}`)
			rows, e = f.s.NotificationEventAudience(f.ctx, tx, event)
			if e != nil {
				return e
			}
			if len(rows) != 1 {
				t.Fatal(rows)
			}
			encoded, _ := json.Marshal(rows)
			if strings.Contains(string(encoded), "private-sensitive") || strings.Contains(string(encoded), "bank-account") || strings.Contains(string(encoded), "forged") {
				t.Fatal("notification exposed private payload", string(encoded))
			}
			if event.Kind == "withdrawal.outcome_unknown" && (rows[0].RecipientID != a.Customer.ID || rows[0].AmountMoney == nil || rows[0].AmountMoney.Cmp(d("10")) != 0) {
				t.Fatal(rows)
			}
		}
		wrong := correction
		wrong.OperatorID = uuid.NewString()
		if _, e = f.s.NotificationEventAudience(f.ctx, tx, wrong); !errors.Is(e, ErrConflict) {
			t.Fatal("forged scope accepted", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
