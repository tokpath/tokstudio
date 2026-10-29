package cooperation

import (
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"math/big"
)

// GiftPercent uses percentage points: "1" means 1%. Each band applies only
// to the part of this order above FromMoney, up to the next band's threshold.
type PurchaseGiftTier struct {
	FromMoney   ledger.Decimal `json:"from_money"`
	GiftPercent ledger.Decimal `json:"gift_percent"`
}
type PurchasePricing struct {
	MonetarySnapshot ledger.MonetarySnapshot `json:"monetary_snapshot"`
	GiftTiers        []PurchaseGiftTier      `json:"gift_tiers"`
	PaidATG          ledger.Decimal          `json:"paid_atg"`
	GiftATG          ledger.Decimal          `json:"gift_atg"`
	// Keep sub-atom calculation evidence; grant the displayed 18-place quantity.
	GiftRemainder string `json:"gift_remainder"`
	PaidRemainder string `json:"paid_remainder"`
}

func ValidatePurchaseTiers(tiers []PurchaseGiftTier) error {
	if len(tiers) == 0 || len(tiers) > 100 || !tiers[0].FromMoney.IsZero() {
		return ledger.ErrAmount
	}
	for i, t := range tiers {
		if !t.FromMoney.Valid() || !t.GiftPercent.Valid() || t.FromMoney.Sign() < 0 || t.GiftPercent.Sign() < 0 {
			return ledger.ErrAmount
		}
		if i > 0 && t.FromMoney.Cmp(tiers[i-1].FromMoney) <= 0 {
			return ledger.ErrAmount
		}
	}
	return nil
}
func CalculatePurchase(tiers []PurchaseGiftTier, amount ledger.Decimal, base ledger.MonetarySnapshot) (PurchasePricing, error) {
	out := PurchasePricing{GiftTiers: tiers, MonetarySnapshot: base}
	if err := ValidatePurchaseTiers(tiers); err != nil {
		return out, err
	}
	if !amount.Valid() || amount.Sign() <= 0 || !base.AtgPerUnit.Valid() || base.AtgPerUnit.Sign() <= 0 {
		return out, ledger.ErrAmount
	}
	paid, rest := ledger.Quantize(new(big.Rat).Mul(amount.Rat(), base.AtgPerUnit.Rat()))
	out.PaidATG, out.PaidRemainder = paid, rest.RatString()
	gift := new(big.Rat)
	for i, t := range tiers {
		if amount.Cmp(t.FromMoney) <= 0 {
			break
		}
		end := amount
		if i+1 < len(tiers) && end.Cmp(tiers[i+1].FromMoney) > 0 {
			end = tiers[i+1].FromMoney
		}
		part := new(big.Rat).Mul(end.Sub(t.FromMoney).Rat(), t.GiftPercent.Rat())
		gift.Add(gift, part.Quo(part, big.NewRat(100, 1)))
	}
	gift.Mul(gift, base.AtgPerUnit.Rat())
	out.GiftATG, rest = ledger.Quantize(gift)
	out.GiftRemainder = rest.RatString()
	if out.PaidATG.Sign() <= 0 || !out.PaidATG.Valid() || !out.GiftATG.Valid() || !out.PaidATG.Add(out.GiftATG).Valid() {
		return out, ledger.ErrAmount
	}
	return out, nil
}
