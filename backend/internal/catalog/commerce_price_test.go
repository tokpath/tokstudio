package catalog

import (
	"context"
	"errors"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

type priceFacts []PriceVersion

func (p priceFacts) PriceVersions(_ context.Context, s PriceScope, m string) ([]PriceVersion, error) {
	out := []PriceVersion{}
	for _, v := range p {
		if v.Scope == s && (v.ModelID == m || v.ModelID == "*") {
			out = append(out, v)
		}
	}
	return out, nil
}
func priceAt(id string, scope PriceScope, at time.Time, rate string) PriceVersion {
	return PriceVersion{ID: id, Scope: scope, ModelID: "model", Layer: "regular", EffectiveAt: at, Published: true, Rule: PriceRule{Mode: "manual", Components: []ComponentRate{{Key: "input", Unit: "token", Per: "1000000", Rate: rate}}}}
}
func TestPromotionEndsAtCurrentRegularPriceAndFollowSnapshots(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	supply := PriceScope{Kind: "supply", SellerID: "platform", BuyerID: "oem"}
	retail := PriceScope{Kind: "retail", SellerID: "oem"}
	base := priceAt("supply1", supply, start, "2")
	newBase := priceAt("supply2", supply, start.Add(2*time.Hour), "4")
	follow := PriceVersion{ID: "follow", Scope: retail, ModelID: "*", Layer: "regular", EffectiveAt: start, Published: true, Rule: PriceRule{Mode: "follow", Follow: &supply, Multiplier: "2", ModelMultipliers: map[string]string{"model": "1.5"}}}
	promo := priceAt("promotion", retail, start.Add(time.Hour), "1")
	promo.Layer = "promotion"
	end := start.Add(3 * time.Hour)
	promo.EndsAt = &end
	r := PriceResolver{Source: priceFacts{base, newBase, follow, promo}}
	during, e := r.Resolve(context.Background(), retail, "model", start.Add(150*time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	cost, e := during.Charge(map[string]string{"input": "1000000"})
	if e != nil || cost.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("promotion cost %v %v", cost, e)
	}
	after, e := r.Resolve(context.Background(), retail, "model", end)
	if e != nil {
		t.Fatal(e)
	}
	cost, e = after.Charge(map[string]string{"input": "1000000"})
	if e != nil || cost.Cmp(big.NewRat(6, 1)) != 0 {
		t.Fatalf("restore current regular cost %v %v", cost, e)
	}
	if len(after.Lineage) != 2 || after.Lineage[1].VersionID != "supply2" {
		t.Fatalf("lineage %#v", after.Lineage)
	}
	// Accepted snapshots stay valid after the source changes.
	r.Source = priceFacts{}
	cost, e = during.Charge(map[string]string{"input": "1000000"})
	if e != nil || cost.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatal("accepted snapshot changed")
	}
}
func TestCoverageFactorBoundsEveryQuantity(t *testing.T) {
	retail := ResolvedPrice{ModelID: "model", FactorNumerator: "1", FactorDenominator: "1", Components: []ComponentRate{{Key: "tokens", Unit: "token", Per: "1000", Rate: "3"}, {Key: "request", Unit: "request", Per: "1", Rate: "0.1"}}}
	supply := ResolvedPrice{ModelID: "model", FactorNumerator: "2", FactorDenominator: "3", Components: []ComponentRate{{Key: "tokens", Unit: "token", Per: "1000", Rate: "1"}, {Key: "request", Unit: "request", Per: "1", Rate: "0.2"}}}
	k, e := CoverageFactor(retail, supply)
	if e != nil {
		t.Fatal(e)
	}
	random := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		q := map[string]string{"tokens": big.NewInt(random.Int63n(100000)).String(), "request": "1"}
		sell, e := retail.Charge(q)
		if e != nil {
			t.Fatal(e)
		}
		cost, e := supply.Charge(q)
		if e != nil {
			t.Fatal(e)
		}
		if cost.Cmp(new(big.Rat).Mul(sell, k)) > 0 {
			t.Fatalf("coverage insufficient for %v", q)
		}
	}
}
func TestUnboundedZeroPriceAndMissingUsageRejected(t *testing.T) {
	p := ResolvedPrice{ModelID: "model", FactorNumerator: "1", FactorDenominator: "1", Components: []ComponentRate{{Key: "image", Unit: "image", Per: "1", Rate: "0"}}}
	q := p
	q.Components = []ComponentRate{{Key: "image", Unit: "image", Per: "1", Rate: "0.000000000000000000000001"}}
	if _, e := CoverageFactor(p, q); !errors.Is(e, ErrUnboundedCoverage) {
		t.Fatalf("got %v", e)
	}
	if _, e := p.Charge(map[string]string{}); e == nil {
		t.Fatal("missing usage settled as zero")
	}
	if _, e := p.MaximumCharge(map[string]string{"image": "1", "hidden_fee": "1"}); e == nil {
		t.Fatal("unknown fee ignored")
	}
	if _, e := PriceDecimal("0.0000000000000000000000001"); e == nil {
		t.Fatal("25 digit price silently rounded")
	}
}
func TestFollowCycleAndOverlappingPromotionsRejected(t *testing.T) {
	at := time.Now().UTC()
	s1 := PriceScope{Kind: "retail", SellerID: "a"}
	s2 := PriceScope{Kind: "supply", SellerID: "b", BuyerID: "a"}
	p := priceAt("a", s1, at, "1")
	p.Rule = PriceRule{Mode: "follow", Follow: &s2, Multiplier: "1"}
	q := priceAt("b", s2, at, "1")
	q.Rule = PriceRule{Mode: "follow", Follow: &s1, Multiplier: "1"}
	if _, e := (PriceResolver{Source: priceFacts{p, q}}).Resolve(context.Background(), s1, "model", at); e == nil {
		t.Fatal("follow cycle accepted")
	}
	p = priceAt("promo1", s1, at, "1")
	p.Layer = "promotion"
	end := at.Add(time.Hour)
	p.EndsAt = &end
	q = p
	q.ID = "promo2"
	if _, e := (PriceResolver{Source: priceFacts{p, q}}).Resolve(context.Background(), s1, "model", at); e == nil {
		t.Fatal("overlapping promotions accepted")
	}
}
