package usage

import (
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func reservationQuantities(q catalog.ModelQuote, r modelruntime.Request) map[string]string {
	return modelruntime.EstimateUsage(q.Billing, r)
}
