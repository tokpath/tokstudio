package cooperation

import (
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"math/big"
)

// GiftPercent uses percentage points: "1" means 1%. Each band applies only
// to the part of this order above FromUSD, up to the next band's threshold.
type PurchaseGiftTier struct {
	FromUSD     ledger.Decimal `json:"from_usd"`
	GiftPercent ledger.Decimal `json:"gift_percent"`
}
type PurchasePricing struct {
	GiftTiers []PurchaseGiftTier `json:"gift_tiers"`
	PaidATG   ledger.Decimal     `json:"paid_atg"`
	GiftATG   ledger.Decimal     `json:"gift_atg"`
	// Keep sub-atom calculation evidence; grant the displayed 18-place quantity.
	GiftRemainder string `json:"gift_remainder"`
}

func ValidatePurchaseTiers(tiers []PurchaseGiftTier) error {
	if len(tiers) == 0 || len(tiers) > 100 || !tiers[0].FromUSD.IsZero() {
		return ledger.ErrAmount
	}
	for i, t := range tiers {
		if !t.FromUSD.Valid() || !t.GiftPercent.Valid() || t.FromUSD.Sign() < 0 || t.GiftPercent.Sign() < 0 {
			return ledger.ErrAmount
		}
		if i > 0 && t.FromUSD.Cmp(tiers[i-1].FromUSD) <= 0 {
			return ledger.ErrAmount
		}
	}
	return nil
}
func CalculatePurchase(tiers []PurchaseGiftTier, amount ledger.Decimal) (PurchasePricing, error) {
	out := PurchasePricing{GiftTiers: tiers, PaidATG: amount}
	if err := ValidatePurchaseTiers(tiers); err != nil {
		return out, err
	}
	if !amount.Valid() || amount.Sign() <= 0 {
		return out, ledger.ErrAmount
	}
	gift := new(big.Rat)
	for i, t := range tiers {
		if amount.Cmp(t.FromUSD) <= 0 {
			break
		}
		end := amount
		if i+1 < len(tiers) && end.Cmp(tiers[i+1].FromUSD) > 0 {
			end = tiers[i+1].FromUSD
		}
		part := new(big.Rat).Mul(end.Sub(t.FromUSD).Rat(), t.GiftPercent.Rat())
		gift.Add(gift, part.Quo(part, big.NewRat(100, 1)))
	}
	var rest *big.Rat
	out.GiftATG, rest = ledger.Quantize(gift)
	out.GiftRemainder = rest.RatString()
	if !out.GiftATG.Valid() || !out.PaidATG.Add(out.GiftATG).Valid() {
		return out, ledger.ErrAmount
	}
	return out, nil
}
