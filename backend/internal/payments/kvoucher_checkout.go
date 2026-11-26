package payments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	KVoucherFormReady       = "ready"
	KVoucherFormClaimed     = "claimed"
	KVoucherFormUnavailable = "unavailable"
	KVoucherFormPreparing   = "preparing"
)

var (
	ErrCheckoutUnavailable = errors.New("the requested KVoucher form cannot be issued")
	ErrCheckoutCartReused  = errors.New("KVoucher reused a previously issued Gkash cart ID")
)

// The opt-in flag is stored with the signed form. Older checkouts may already
// have sent their original form to the payer through a normal order response.
type storedCheckout struct {
	Checkout
	KVoucherSingleUse bool `json:"kvoucher_single_use,omitempty"`
}

func (c storedCheckout) kvoucherFormState(claimedAt *time.Time) string {
	if !c.KVoucherSingleUse || c.URL == "" || len(c.FormFields) == 0 {
		return KVoucherFormUnavailable
	}
	if claimedAt != nil {
		return KVoucherFormClaimed
	}
	return KVoucherFormReady
}

func (c Checkout) withoutForm() Checkout {
	c.URL = ""
	c.FormFields = nil
	return c
}

type kvoucherCheckoutScope func(context.Context, *gorm.DB, string) (string, string, error)

type kvoucherCheckoutState struct {
	intentRow
	order Order
	saved storedCheckout
	now   time.Time
}

func (s *Service) readKVoucherCheckout(ctx context.Context, tx *gorm.DB, id string, scope kvoucherCheckoutScope) (kvoucherCheckoutState, error) {
	var state kvoucherCheckoutState
	if err := LockOrderPayment(tx, id); err != nil {
		return state, err
	}
	operator, merchant, err := scope(ctx, tx, id)
	if err != nil {
		return state, err
	}
	q := tx.Table("payments_intents").Where("operator_id=? AND merchant_id=? AND order_id=?", operator, merchant, id)
	if err = q.Take(&state.intentRow).Error; err != nil {
		return state, err
	}
	if err = s.open(state.OrderCiphertext, &state.order); err != nil {
		return state, err
	}
	if state.order.PaymentDriver != "kvoucher" {
		return state, ErrUnsupported
	}
	if state.order.ID != id || state.order.OperatorID != operator || state.order.MerchantID != merchant {
		return state, ErrConflict
	}
	switch state.State {
	case "created":
	case "closed":
		return state, ErrPaymentClosed
	case "executing", "outcome_unknown":
		return state, ErrOutcomeUnknown
	default:
		return state, ErrNotFinal
	}
	state.now, err = databaseTime(ctx, tx)
	if err != nil {
		return state, err
	}
	if !kvoucherCheckoutOpenAt(state.order, state.now) {
		return state, ErrNotFinal
	}
	received, err := HasOrderReceipt(ctx, tx, operator, id)
	if err != nil {
		return state, err
	}
	if received {
		return state, ErrConflict
	}
	if state.CheckoutCiphertext == "" {
		return state, ErrCheckoutUnavailable
	}
	if err = s.open(state.CheckoutCiphertext, &state.saved); err != nil {
		return state, err
	}
	if state.ProviderID == "" || state.saved.ProviderID != state.ProviderID {
		return state, ErrConflict
	}
	state.order.ProviderID = state.ProviderID
	return state, nil
}

func kvoucherCheckoutOpenAt(order Order, now time.Time) bool {
	return now.Before(order.ExpiresAt) && (order.PaymentQuote == nil || now.Before(order.PaymentQuote.ExpiresAt))
}

// OpenKVoucherCheckout reserves the caller's action before asking KVoucher
// for a replacement form. Scope authorizes the payer and selected merchant
// inside both transactions. Only the action that commits can return a form.
func (s *Service) OpenKVoucherCheckout(ctx context.Context, id string, expectedGeneration int64, actionKey string, scope kvoucherCheckoutScope) (Checkout, int64, error) {
	key, err := uuid.Parse(actionKey)
	if err != nil || key == uuid.Nil || id == "" || expectedGeneration < 0 {
		return Checkout{}, 0, ErrInvalid
	}
	var state kvoucherCheckoutState
	var initial Checkout
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		state, err = s.readKVoucherCheckout(ctx, tx, id, scope)
		if err != nil {
			return err
		}
		if state.CheckoutGeneration != expectedGeneration || state.CheckoutIssueUntil != nil && state.now.Before(*state.CheckoutIssueUntil) {
			return ErrCheckoutUnavailable
		}

		action := map[string]any{"merchant_id": state.MerchantID, "order_id": id, "action_key": key}
		updates := map[string]any{"checkout_generation": expectedGeneration + 1}
		if state.saved.kvoucherFormState(state.CheckoutClaimedAt) == KVoucherFormReady {
			cartHash := gkashCartHash(state.saved.FormFields)
			if cartHash == "" || gkashField(state.saved.FormFields, "signature") == "" {
				return ErrProvider
			}
			action["cart_hash"] = cartHash
			updates["checkout_claimed_at"] = state.now
			updates["checkout_issue_until"] = nil
			initial = state.saved.Checkout
		} else {
			if gkashCartHash(state.saved.FormFields) == "" {
				return ErrCheckoutUnavailable
			}
			updates["checkout_issue_until"] = state.now.Add(time.Minute)
		}
		claimed := tx.Table("payments_kvoucher_checkout_actions").Clauses(clause.OnConflict{DoNothing: true}).Create(action)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return ErrCheckoutUnavailable
		}
		result := tx.Table("payments_intents").
			Where("operator_id=? AND merchant_id=? AND order_id=? AND state='created' AND checkout_generation=?", state.OperatorID, state.MerchantID, id, expectedGeneration).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCheckoutUnavailable
		}
		return nil
	})
	if err != nil {
		return Checkout{}, state.CheckoutGeneration, err
	}
	generation := expectedGeneration + 1
	if initial.URL != "" {
		return initial, generation, nil
	}

	form, err := s.reissueKVoucherCheckout(ctx, state, generation, key, scope)
	if err != nil {
		// The reserved generation still fences the old action. Clear its
		// short lease so another explicit request can check the original order.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.DB.WithContext(cleanupCtx).Table("payments_intents").
			Where("operator_id=? AND merchant_id=? AND order_id=? AND checkout_generation=?", state.OperatorID, state.MerchantID, id, generation).
			Update("checkout_issue_until", nil).Error
	}
	return form, generation, err
}

func (s *Service) reissueKVoucherCheckout(ctx context.Context, state kvoucherCheckoutState, generation int64, key uuid.UUID, scope kvoucherCheckoutScope) (Checkout, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	k, ok := s.Drivers["kvoucher"].(KVoucher)
	if !ok {
		return Checkout{}, ErrUnsupported
	}
	merchant, err := s.Merchant(ctx, state.OperatorID, state.MerchantID)
	if err != nil {
		return Checkout{}, err
	}
	if err = ready(merchant, k); err != nil {
		return Checkout{}, err
	}
	if err = k.checkRetryOrder(ctx, merchant, state.order); err != nil {
		return Checkout{}, err
	}

	validate := func(tx *gorm.DB) (kvoucherCheckoutState, error) {
		current, err := s.readKVoucherCheckout(ctx, tx, state.OrderID, scope)
		if err != nil {
			return current, err
		}
		if current.MerchantID != state.MerchantID || current.ProviderID != state.ProviderID || current.CheckoutGeneration != generation ||
			current.CheckoutIssueUntil == nil || !current.now.Before(*current.CheckoutIssueUntil) {
			return current, ErrCheckoutUnavailable
		}
		var row merchantRow
		err = tx.Table("payments_merchants").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id=? AND operator_id=? AND version=? AND enabled", merchant.ID, merchant.OperatorID, merchant.Version).
			Take(&row).Error
		return current, err
	}
	if err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := validate(tx)
		return err
	}); err != nil {
		return Checkout{}, err
	}
	form, err := k.gkashCheckout(ctx, merchant, state.ProviderID)
	if err != nil {
		return Checkout{}, err
	}

	var checkout Checkout
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := validate(tx)
		if err != nil {
			return err
		}
		prior := current.saved.PayMoney
		if form.Money == nil || prior == nil || form.Money.Currency != "MYR" || form.Money.Validate() != nil ||
			prior.Currency != form.Money.Currency || prior.Amount.Cmp(form.Money.Amount) != 0 {
			return ErrCheckoutMismatch
		}
		cartHash := gkashCartHash(form.Fields)
		if cartHash == "" || gkashField(form.Fields, "signature") == "" {
			return ErrProvider
		}
		if cartHash == gkashCartHash(current.saved.FormFields) {
			return ErrCheckoutCartReused
		}
		var used bool
		if err = tx.Raw("SELECT EXISTS (SELECT 1 FROM payments_kvoucher_checkout_actions WHERE merchant_id=? AND order_id=? AND cart_hash=?)", state.MerchantID, state.OrderID, cartHash).
			Scan(&used).Error; err != nil {
			return err
		}
		if used {
			return ErrCheckoutCartReused
		}
		result := tx.Table("payments_kvoucher_checkout_actions").
			Where("merchant_id=? AND order_id=? AND action_key=? AND cart_hash IS NULL", state.MerchantID, state.OrderID, key).
			Update("cart_hash", cartHash)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCheckoutUnavailable
		}
		result = tx.Table("payments_intents").
			Where("operator_id=? AND merchant_id=? AND order_id=? AND checkout_generation=?", state.OperatorID, state.MerchantID, state.OrderID, generation).
			Updates(map[string]any{"checkout_claimed_at": current.now, "checkout_issue_until": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCheckoutUnavailable
		}
		checkout = current.saved.Checkout
		checkout.URL = form.Action
		checkout.FormFields = form.Fields
		return nil
	})
	if err != nil {
		return Checkout{}, err
	}
	return checkout, nil
}

func gkashCartHash(fields map[string]string) string {
	cart := gkashField(fields, "v_cartid")
	if cart == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToUpper(cart)))
	return hex.EncodeToString(sum[:])
}

func gkashField(fields map[string]string, name string) string {
	var found string
	var seen bool
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			if seen {
				return ""
			}
			seen = true
			found = strings.TrimSpace(value)
		}
	}
	return found
}
