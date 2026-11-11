package payments

import (
	"context"
	"log/slog"
)

const CheckoutAmountMismatch = "amount_mismatch"

// Log only normalized money and our own order identity. Never log the provider
// body, signed form, credentials, headers or customer details.
func checkoutMismatch(ctx context.Context, driver string, o Order, providerID string, actual *Money) (Checkout, error) {
	currency, amount := "missing", "missing"
	if actual != nil {
		currency = "unsupported"
		if _, ok := fiatMinorDigits(actual.Currency); ok {
			currency = actual.Currency
		}
		amount = actual.Amount.String()
	}
	slog.WarnContext(ctx, "payment_checkout_mismatch", "driver", driver, "order_id", o.ID,
		"expected_currency", o.Money.Currency, "expected_amount", o.Money.Amount.String(),
		"reported_currency", currency, "reported_amount", amount)
	return Checkout{ProviderID: providerID, Problem: CheckoutAmountMismatch}, ErrCheckoutMismatch
}
