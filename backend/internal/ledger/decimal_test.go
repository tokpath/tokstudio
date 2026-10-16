package ledger

import (
	"encoding/json"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

func TestExactDecimal(t *testing.T) {
	for _, s := range []string{"0", "0.000000000000000001", "12345678901234567890.123456789012345678", "-0.1"} {
		d, e := ParseDecimal(s)
		if e != nil || d.String() != s {
			t.Fatalf("%s %v %v", s, d, e)
		}
		b, _ := json.Marshal(d)
		var round Decimal
		if e = json.Unmarshal(b, &round); e != nil || d.Cmp(round) != 0 {
			t.Fatal(e, string(b))
		}
	}
	for _, s := range []string{"1e3", "1/3", "01", "NaN", " 1", "1.0000000000000000001", "100000000000000000000"} {
		if _, e := ParseDecimal(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	d := MustDecimal("0.1")
	if d.Add(MustDecimal("0.2")).String() != "0.3" || d.String() != "0.1" {
		t.Fatal("mutation or floating error")
	}
	var d2 Decimal
	if json.Unmarshal([]byte("0.1"), &d2) == nil {
		t.Fatal("accepted JSON float")
	}
}
func TestCarryAndOrderShare(t *testing.T) {
	fee := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil))
	carry := new(big.Rat)
	posted := Zero()
	for i := 0; i < 100; i++ {
		d, c := Accumulate(fee, carry)
		posted = posted.Add(d)
		carry = c
	}
	if posted.String() != "0.000000000000000001" || carry.Sign() != 0 {
		t.Fatal(posted, carry)
	}
	share, _, e := CumulativeShare(FromInt64(80), FromInt64(110), FromInt64(22), Zero())
	if e != nil || share.String() != "16" {
		t.Fatal(share, e)
	}
	all := Zero()
	for q := int64(1); q <= 110; q++ {
		d, _, e := CumulativeShare(FromInt64(80), FromInt64(110), FromInt64(q), all)
		if e != nil {
			t.Fatal(e)
		}
		all = all.Add(d)
	}
	if all.String() != "80" {
		t.Fatal(all)
	}
}
func TestProportionProperties(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for n := 0; n < 500; n++ {
		weights := []Decimal{}
		for i := 0; i < 1+r.Intn(12); i++ {
			weights = append(weights, FromInt64(int64(1+r.Intn(1000))))
		}
		total := fromAtoms(big.NewInt(int64(r.Intn(1000000))))
		out, e := Proportional(total, weights)
		if e != nil {
			t.Fatal(e)
		}
		sum := Zero()
		for _, v := range out {
			if v.Sign() < 0 {
				t.Fatal("negative")
			}
			sum = sum.Add(v)
		}
		if sum.Cmp(total) != 0 {
			t.Fatal("lost atoms")
		}
	}
}
func TestCostAndCoverage(t *testing.T) {
	p := Position{}
	p, e := p.Receive(FromInt64(100), FromInt64(80))
	if e != nil {
		t.Fatal(e)
	}
	p, c, _, e := p.Remove(FromInt64(25))
	if e != nil || c.String() != "20" {
		t.Fatal(p, c, e)
	}
	p, e = p.Receive(FromInt64(125), FromInt64(90))
	if e != nil {
		t.Fatal(e)
	}
	p, c, _, e = p.Remove(FromInt64(40))
	if e != nil || c.String() != "30" || p.Cost.String() != "120" {
		t.Fatal(p, c, e)
	}
	n, total, e := (Coverage{FromInt64(200), FromInt64(20), FromInt64(30), FromInt64(60), FromInt64(90), FromInt64(10)}).Space()
	if e != nil || n.String() != "20" || total.String() != "50" {
		t.Fatal(n, total, e)
	}
}
func TestCalendarClampsAndNaturalWindows(t *testing.T) {
	start := time.Date(2024, 1, 31, 10, 0, 0, 0, time.UTC)
	end, e := PeriodEnd(start, Month, "UTC")
	if e != nil || end.Day() != 29 || end.Month() != 2 {
		t.Fatal(end, e)
	}
	accepted := time.Date(2026, 10, 31, 16, 1, 0, 0, time.UTC)
	w, e := BudgetWindow(accepted, start, Month, "Asia/Shanghai")
	if e != nil || !w.Start.Equal(time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)) {
		t.Fatal(w, e)
	}
	if !accepted.Before(*w.End) {
		t.Fatal("window excludes request")
	}
}
