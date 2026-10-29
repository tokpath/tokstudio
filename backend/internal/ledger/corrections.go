package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GrantReversal struct {
	ID, OperatorID, OriginalEventID, ReversalEventID, BusinessKey, Fingerprint string
	Amount, PaidMoney                                                          Decimal
	Sources                                                                    json.RawMessage `gorm:"type:jsonb"`
	CreatedAt                                                                  time.Time
}

func (GrantReversal) TableName() string { return "ledger_grant_reversals" }

type ReverseUsageInput struct {
	OperatorID, CustomerID, OriginalEventID, BusinessKey, ActorID, Reason string
	Amount                                                                Decimal
	At                                                                    time.Time
}
type ReversalResult struct {
	Reversal     GrantReversal
	Event        GrantEvent
	ExpiryEvents []GrantEvent
}

// ReverseUsage restores the exact original source composition, with cumulative
// bounds against the original fact. Expired grants are immediately classified
// as expiry again; the transaction never creates an extended usable balance.
func (s *Service) ReverseUsage(ctx context.Context, tx *gorm.DB, in ReverseUsageInput) (ReversalResult, error) {
	var out ReversalResult
	if in.Amount.Sign() <= 0 || in.BusinessKey == "" || in.ActorID == "" || in.Reason == "" || in.At.IsZero() {
		return out, ErrAmount
	}
	fp := fingerprint(struct{ Operator, Customer, Event, Key, Actor, Reason, Amount string }{in.OperatorID, in.CustomerID, in.OriginalEventID, in.BusinessKey, in.ActorID, in.Reason, in.Amount.String()})
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, in.OperatorID, in.CustomerID); e != nil {
			return e
		}
		e := t.First(&out.Reversal, "operator_id=? AND business_key=?", in.OperatorID, in.BusinessKey).Error
		if e == nil {
			if out.Reversal.Fingerprint != fp {
				return ErrConflict
			}
			return t.First(&out.Event, "id=?", out.Reversal.ReversalEventID).Error
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var original GrantEvent
		if e = t.First(&original, "id=?", in.OriginalEventID).Error; e != nil {
			return e
		}
		if original.Kind != "usage" || original.JournalID == nil {
			return ErrScope
		}
		var grant Grant
		if e = t.First(&grant, "id=? AND operator_id=? AND customer_id=?", original.GrantID, in.OperatorID, in.CustomerID).Error; e != nil {
			return e
		}
		var prior []GrantReversal
		if e = t.Where("original_event_id=?", original.ID).Find(&prior).Error; e != nil {
			return e
		}
		reversed, paidReversed := Zero(), Zero()
		already := map[string]Decimal{}
		for _, r := range prior {
			reversed = reversed.Add(r.Amount)
			paidReversed = paidReversed.Add(r.PaidMoney)
			var parts []GrantSource
			if e = json.Unmarshal(r.Sources, &parts); e != nil {
				return e
			}
			for _, p := range parts {
				already[p.Kind] = already[p.Kind].Add(p.Amount)
			}
		}
		if in.Amount.Cmp(original.Amount.Sub(reversed)) > 0 {
			return ErrAmount
		}
		var originalSources []GrantSource
		if e = json.Unmarshal(original.Sources, &originalSources); e != nil {
			return e
		}
		weights := []Decimal{}
		for _, source := range originalSources {
			weights = append(weights, source.Amount.Sub(already[source.Kind]))
		}
		parts, e := Proportional(in.Amount, weights)
		if e != nil {
			return e
		}
		paidTarget, _ := Quantize(new(big.Rat).Mul(original.PaidMoney.Rat(), new(big.Rat).Quo(reversed.Add(in.Amount).Rat(), original.Amount.Rat())))
		paid := paidTarget.Sub(paidReversed)
		control, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.OperatorID, OwnerID: in.OperatorID, Purpose: "control:credits", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		// A takeover preserves the original journal in its old operator. The
		// grant reversal still links to that event, while this new journal
		// belongs to the current responsibility and cannot cross scopes.
		originalJournal := original.JournalID
		var originalScope string
		if e = t.Model(&Journal{}).Select("operator_id").Where("id=?", *original.JournalID).Row().Scan(&originalScope); e != nil {
			return e
		}
		if originalScope != in.OperatorID {
			originalJournal = nil
		}
		journal, e := s.Post(ctx, t, Posting{OperatorID: in.OperatorID, BusinessKey: "correction:" + in.BusinessKey, Kind: "usage_reversal", SourceID: original.ID, ActorID: in.ActorID, Reason: in.Reason, OriginalID: originalJournal, Lines: []Line{{AccountID: grant.AccountID, Amount: in.Amount}, {AccountID: control.ID, Amount: in.Amount.Neg()}}})
		if e != nil {
			return e
		}
		restored := []GrantSource{}
		for i, source := range originalSources {
			if parts[i].IsZero() {
				continue
			}
			var row grantSourceRow
			if e = t.First(&row, "grant_id=? AND kind=?", grant.ID, source.Kind).Error; e != nil {
				return e
			}
			if parts[i].Cmp(row.Consumed) > 0 {
				return ErrAmount
			}
			if e = t.Model(&grantSourceRow{}).Where("grant_id=? AND kind=?", grant.ID, source.Kind).Update("consumed", row.Consumed.Sub(parts[i])).Error; e != nil {
				return e
			}
			restored = append(restored, GrantSource{Kind: source.Kind, Amount: parts[i]})
		}
		if grant.Consumed.Cmp(in.Amount) < 0 || grant.ConsumedPaidMoney.Cmp(paid) < 0 {
			return ErrAmount
		}
		if e = t.Model(&Grant{}).Where("id=?", grant.ID).Updates(map[string]any{"consumed": grant.Consumed.Sub(in.Amount), "consumed_paid_money": grant.ConsumedPaidMoney.Sub(paid)}).Error; e != nil {
			return e
		}
		encoded, _ := json.Marshal(restored)
		out.Event = GrantEvent{ID: uuid.NewString(), GrantID: grant.ID, BusinessKey: "correction:" + in.BusinessKey, Kind: "reversal", Amount: in.Amount.Neg(), PaidMoney: paid.Neg(), Sources: encoded, JournalID: &journal.ID, CreatedAt: in.At}
		if e = t.Create(&out.Event).Error; e != nil {
			return e
		}
		out.Reversal = GrantReversal{ID: uuid.NewString(), OperatorID: in.OperatorID, OriginalEventID: original.ID, ReversalEventID: out.Event.ID, BusinessKey: in.BusinessKey, Fingerprint: fp, Amount: in.Amount, PaidMoney: paid, Sources: encoded, CreatedAt: in.At}
		if e = t.Create(&out.Reversal).Error; e != nil {
			return e
		}
		out.ExpiryEvents, e = s.ExpireCredits(ctx, t, in.OperatorID, in.CustomerID, in.At)
		return e
	})
	return out, e
}

// CancelReservedGrant removes only the specifically reserved unfulfilled rights.
// Payment completion and incentive corrections belong to the outer workflow.
func (s *Service) CancelReservedGrant(ctx context.Context, tx *gorm.DB, operator, customer, grantID, holdID, key, actor, reason string, at time.Time) (GrantEvent, error) {
	var out GrantEvent
	if key == "" || actor == "" || reason == "" || at.IsZero() {
		return out, ErrAmount
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, operator, customer); e != nil {
			return e
		}
		var g Grant
		if e := t.First(&g, "id=? AND operator_id=? AND customer_id=?", grantID, operator, customer).Error; e != nil {
			return e
		}
		e := t.First(&out, "grant_id=? AND business_key=?", grantID, "cancel:"+key).Error
		if e == nil {
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		hold, allocations, e := s.HoldDetails(ctx, t, operator, holdID)
		if e != nil {
			return e
		}
		if hold.Status != "active" || (hold.Purpose != "refund" && hold.Purpose != "chargeback" && hold.Purpose != "waiver") || len(allocations) != 1 || allocations[0].AccountID != g.AccountID {
			return ErrHoldState
		}
		n := allocations[0].Amount
		control, e := s.EnsureAccount(ctx, t, Account{OperatorID: operator, OwnerID: operator, Purpose: "control:cancelled", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		j, e := s.Capture(ctx, t, holdID, Posting{OperatorID: operator, BusinessKey: "grant-cancel:" + key, Kind: "rights_cancellation", SourceID: grantID, ActorID: actor, Reason: reason, Lines: []Line{{AccountID: g.AccountID, Amount: n.Neg()}, {AccountID: control.ID, Amount: n}}}, nil)
		if e != nil {
			return e
		}
		out, e = s.attributeGrant(t, g, n, "cancellation", "cancel:"+key, &j.ID, at)
		return e
	})
	return out, e
}
