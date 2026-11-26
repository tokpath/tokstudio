package usage

import (
	"encoding/json"
	"maps"
	"math/big"
	"strconv"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func reservationQuantities(q catalog.ModelQuote, r modelruntime.Request) map[string]string {
	quantities := modelruntime.EstimateUsage(q.Billing, r)
	if q.SubscriptionOutputTokens > 0 {
		for _, rule := range q.Billing.UsageRules {
			if rule.Unit == "token" && strings.HasPrefix(rule.Component, "output") {
				quantities[rule.Component] = strconv.FormatInt(q.SubscriptionOutputTokens, 10)
			}
		}
	}
	return quantities
}

func estimateSupplyReservations(quote catalog.ModelQuote, r modelruntime.Request) ([]SupplyHold, error) {
	holds := make([]SupplyHold, 0, len(quote.Supply))
	quantities := reservationQuantities(quote, r)
	for _, price := range quote.Supply {
		exact, err := reservationCharge(price, quote, r, quantities)
		if err != nil {
			return nil, err
		}
		maximum, err := ceiling(exact)
		if err != nil {
			return nil, err
		}
		holds = append(holds, SupplyHold{BuyerID: price.Scope.BuyerID, Maximum: maximum})
	}
	return holds, nil
}

func reservationCharge(price catalog.ResolvedPrice, q catalog.ModelQuote, r modelruntime.Request, quantities map[string]string) (*big.Rat, error) {
	total, err := price.Charge(quantities)
	if err != nil || q.Billing.Billing == nil || !r.Operation.IsImage() && r.Operation != modelruntime.Video {
		return total, err
	}
	var params map[string]json.RawMessage
	_ = json.Unmarshal(r.Params, &params)
	var size, quality string
	_ = json.Unmarshal(params["size"], &size)
	_ = json.Unmarshal(params["quality"], &quality)
	knownSize, knownQuality := size != "" && size != "auto", quality != "" && quality != "auto"
	if knownSize && knownQuality {
		return total, nil
	}
	// A provider choosing its default size can produce a separately priced
	// result. Reserve the most expensive possible specification once.
	for _, item := range q.Billing.Billing.Output {
		for _, variant := range item.Variants {
			if knownSize && variant.Size != "" && variant.Size != size || knownQuality && variant.Quality != "" && variant.Quality != quality {
				continue
			}
			selector := make(map[string]json.RawMessage, len(params)+2)
			maps.Copy(selector, params)
			if !knownSize && variant.Size != "" {
				selector["size"], _ = json.Marshal(variant.Size)
			}
			if !knownQuality && variant.Quality != "" {
				selector["quality"], _ = json.Marshal(variant.Quality)
			}
			candidate := r
			candidate.Params, _ = json.Marshal(selector)
			amount, err := price.Charge(reservationQuantities(q, candidate))
			if err != nil {
				return nil, err
			}
			if amount.Cmp(total) > 0 {
				total = amount
			}
		}
	}
	return total, nil
}
