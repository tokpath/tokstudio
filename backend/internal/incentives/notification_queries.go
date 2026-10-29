package incentives

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type NotificationAudience struct {
	OperatorID  string          `json:"operator_id"`
	RecipientID string          `json:"recipient_id"`
	Reference   string          `json:"reference"`
	Kind        string          `json:"kind"`
	AmountMoney *ledger.Decimal `json:"amount_money,omitempty"`
	OccurredAt  time.Time       `json:"occurred_at"`
}

// NotificationEventAudience is a trusted same-transaction outbox projection,
// never an HTTP directory. It reloads the persisted event and returns no raw
// event payload, email addresses, bank details, proofs or credentials.
func (s *Service) NotificationEventAudience(ctx context.Context, tx *gorm.DB, input Event) ([]NotificationAudience, error) {
	if tx == nil || !validID(input.ID) {
		return nil, ErrInvalidInput
	}
	var event Event
	if e := s.dbFor(ctx, tx).First(&event, "id=?", input.ID).Error; e != nil {
		return nil, e
	}
	if event.OperatorID != input.OperatorID || event.Kind != input.Kind || event.AggregateID != input.AggregateID {
		return nil, ErrConflict
	}
	ids := map[string]bool{}
	var amount *ledger.Decimal
	addStaff := func() error {
		staff, e := s.org.OperatorNotificationStaff(ctx, tx, event.OperatorID)
		if e != nil {
			return e
		}
		for _, id := range staff {
			ids[id] = true
		}
		return nil
	}
	switch {
	case event.Kind == "withdrawal.policy_changed" || event.Kind == "policy.changed":
		if e := addStaff(); e != nil {
			return nil, e
		}
	case strings.HasPrefix(event.Kind, "withdrawal."):
		var withdrawal withdrawalRow
		if e := tx.First(&withdrawal, "id=?", event.AggregateID).Error; e != nil {
			return nil, e
		}
		ids[withdrawal.CustomerID] = true
		value := withdrawal.AmountMoney
		amount = &value
	case event.Kind == "qualification.acquired" || event.Kind == "commission.function_changed" || event.Kind == "commission.pending_offset_recovered":
		ids[event.AggregateID] = true
	case event.Kind == "consumption.corrected" || event.Kind == "confirmation.dispute_changed" || event.Kind == "commission.source_restricted" || event.Kind == "commission.source_released":
		var confirmation confirmationRow
		if e := tx.First(&confirmation, "id=?", event.AggregateID).Error; e != nil {
			return nil, e
		}
		ids[confirmation.CustomerID] = true
		var beneficiaries []string
		if e := tx.Table("incentives_commission_bases").Distinct("beneficiary_id").Where("event_id=?", confirmation.ID).Pluck("beneficiary_id", &beneficiaries).Error; e != nil {
			return nil, e
		}
		for _, id := range beneficiaries {
			ids[id] = true
		}
	case event.Kind == "registration.reward_delivery_pending" || event.Kind == "registration.rewards_delivered":
		var promise promiseRow
		if e := tx.First(&promise, "intent_id=?", event.AggregateID).Error; e != nil {
			return nil, e
		}
		if promise.CustomerID != nil {
			ids[*promise.CustomerID] = true
		}
		if event.Kind == "registration.rewards_delivered" && promise.InviterID != nil && promise.ActualInviterATG.Sign() > 0 {
			ids[*promise.InviterID] = true
		}
	case event.Kind == "campaign.version_published":
		var campaign campaignRow
		if e := tx.First(&campaign, "id=?", event.AggregateID).Error; e != nil {
			return nil, e
		}
		if campaign.OwnerCustomerID != nil {
			ids[*campaign.OwnerCustomerID] = true
		}
		if e := addStaff(); e != nil {
			return nil, e
		}
	default:
		return []NotificationAudience{}, nil
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		if validID(id) {
			ordered = append(ordered, id)
		}
	}
	sort.Strings(ordered)
	out := make([]NotificationAudience, 0, len(ordered))
	for _, id := range ordered {
		out = append(out, NotificationAudience{OperatorID: event.OperatorID, RecipientID: id, Reference: event.ID, Kind: event.Kind, AmountMoney: amount, OccurredAt: event.CreatedAt})
	}
	return out, nil
}
