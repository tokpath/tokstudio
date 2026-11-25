package usage

import (
	"encoding/json"
	"maps"
	"math/big"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func reservationQuantities(q catalog.ModelQuote, r modelruntime.Request) map[string]string {
	return modelruntime.EstimateUsage(q.Billing, r)
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
