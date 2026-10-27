package cooperation

import (
	"math/big"
	"testing"
	"time"
)

func TestCoverageObservationKeepsKnownRatesWithoutCachingFutureOrHistoricalPrices(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	apply := func(rate *big.Rat, complete bool, at time.Time, want string) {
		t.Helper()
		got, err := observedCoverageRate(f.db, f.platform, f.channel, rate, complete, at, now)
		if err != nil || got.RatString() != want {
			t.Fatal(got, err, want)
		}
	}
	apply(big.NewRat(3, 1), true, now.Add(-time.Second), "3")
	apply(big.NewRat(1, 1), true, now.Add(time.Hour), "1")
	apply(nil, false, now, "3")
	apply(big.NewRat(1, 1), true, now.Add(-time.Hour), "1")
	apply(nil, false, now, "3")
	apply(big.NewRat(4, 1), false, now, "4")
	apply(nil, false, now, "4")
	apply(big.NewRat(2, 3), true, now, "2/3")
	apply(nil, false, now, "1")
	var row coverageRateObservation
	if err := f.db.Where("operator_id=? AND beneficiary_id=?", f.platform, f.channel).First(&row).Error; err != nil || row.Numerator != "2" || row.Denominator != "3" {
		t.Fatal(row, err)
	}
}
