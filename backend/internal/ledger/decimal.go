// Package ledger owns exact asset quantities, holds and immutable postings.
// Values have a unit at the account boundary; USD and ATG never balance each other.
package ledger

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const Scale = 18

var scale = new(big.Int).Exp(big.NewInt(10), big.NewInt(Scale), nil)
var maxAtoms = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(10), big.NewInt(38), nil), big.NewInt(1))
var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
var ErrAmount = errors.New("invalid exact decimal amount")

// Decimal is immutable. Its zero value is zero; methods never mutate an operand.
type Decimal struct{ atoms *big.Int }

func Zero() Decimal                { return Decimal{} }
func FromInt64(n int64) Decimal    { return Decimal{new(big.Int).Mul(big.NewInt(n), scale)} }
func fromAtoms(n *big.Int) Decimal { return Decimal{new(big.Int).Set(n)} }
func (d Decimal) integer() *big.Int {
	if d.atoms == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(d.atoms)
}
func ParseDecimal(s string) (Decimal, error) {
	if len(s) > 60 || !decimalPattern.MatchString(s) {
		return Zero(), ErrAmount
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.SplitN(s, ".", 2)
	fractional := ""
	if len(parts) == 2 {
		fractional = parts[1]
	}
	if len(fractional) > Scale {
		return Zero(), fmt.Errorf("%w: more than %d fractional digits", ErrAmount, Scale)
	}
	n, ok := new(big.Int).SetString(parts[0]+fractional+strings.Repeat("0", Scale-len(fractional)), 10)
	if !ok || n.Cmp(maxAtoms) > 0 {
		return Zero(), ErrAmount
	}
	if negative {
		n.Neg(n)
	}
	return fromAtoms(n), nil
}
func MustDecimal(s string) Decimal {
	d, e := ParseDecimal(s)
	if e != nil {
		panic(e)
	}
	return d
}
func (d Decimal) Valid() bool           { return new(big.Int).Abs(d.integer()).Cmp(maxAtoms) <= 0 }
func (d Decimal) Add(o Decimal) Decimal { return fromAtoms(new(big.Int).Add(d.integer(), o.integer())) }
func (d Decimal) Sub(o Decimal) Decimal { return fromAtoms(new(big.Int).Sub(d.integer(), o.integer())) }
func (d Decimal) Neg() Decimal          { return fromAtoms(new(big.Int).Neg(d.integer())) }
func (d Decimal) Cmp(o Decimal) int     { return d.integer().Cmp(o.integer()) }
func (d Decimal) Sign() int             { return d.integer().Sign() }
func (d Decimal) IsZero() bool          { return d.Sign() == 0 }
func (d Decimal) Rat() *big.Rat         { return new(big.Rat).SetFrac(d.integer(), new(big.Int).Set(scale)) }
func (d Decimal) String() string {
	n := d.integer()
	negative := n.Sign() < 0
	n.Abs(n)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, scale, r)
	s := q.String()
	if r.Sign() != 0 {
		s += "." + strings.TrimRight(strings.Repeat("0", Scale-len(r.String()))+r.String(), "0")
	}
	if negative {
		s = "-" + s
	}
	return s
}
func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("decimal must be a string: %w", err)
	}
	x, err := ParseDecimal(s)
	if err == nil {
		*d = x
	}
	return err
}
func (d Decimal) Value() (driver.Value, error) {
	if !d.Valid() {
		return nil, ErrAmount
	}
	return d.String(), nil
}
func (d *Decimal) Scan(v any) error {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	case int64:
		s = fmt.Sprint(x)
	default:
		return fmt.Errorf("decimal: unsupported SQL value %T", v)
	}
	x, err := ParseDecimal(s)
	if err == nil {
		*d = x
	}
	return err
}
func (Decimal) GormDataType() string { return "numeric(38,18)" }

// Quantize truncates toward zero and returns the exact unposted remainder.
// The caller must persist/carry the remainder instead of silently discarding it.
func Quantize(value *big.Rat) (Decimal, *big.Rat) {
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(scale))
	n := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	d := fromAtoms(n)
	return d, new(big.Rat).Sub(value, d.Rat())
}

// Accumulate posts every representable atom without repeatedly rounding micro fees away.
func Accumulate(value, carry *big.Rat) (Decimal, *big.Rat) {
	x := new(big.Rat).Set(value)
	if carry != nil {
		x.Add(x, carry)
	}
	return Quantize(x)
}

// Proportional allocates an amount without creating or losing atoms. Stable
// largest-remainder ordering only resolves the final indivisible atoms.
func Proportional(total Decimal, weights []Decimal) ([]Decimal, error) {
	if total.Sign() < 0 || !total.Valid() {
		return nil, ErrAmount
	}
	sum := Zero()
	for _, w := range weights {
		if w.Sign() < 0 || !w.Valid() {
			return nil, ErrAmount
		}
		sum = sum.Add(w)
	}
	if sum.IsZero() {
		if total.IsZero() {
			return make([]Decimal, len(weights)), nil
		}
		return nil, ErrAmount
	}
	out := make([]Decimal, len(weights))
	rems := make([]*big.Rat, len(weights))
	used := Zero()
	for i, w := range weights {
		exact := new(big.Rat).Mul(total.Rat(), new(big.Rat).Quo(w.Rat(), sum.Rat()))
		out[i], rems[i] = Quantize(exact)
		used = used.Add(out[i])
	}
	left := total.Sub(used).integer()
	for left.Sign() > 0 {
		best := -1
		for i, r := range rems {
			if r.Sign() > 0 && (best < 0 || r.Cmp(rems[best]) > 0) {
				best = i
			}
		}
		if best < 0 {
			return nil, ErrAmount
		}
		out[best] = out[best].Add(fromAtoms(big.NewInt(1)))
		rems[best] = new(big.Rat)
		left.Sub(left, big.NewInt(1))
	}
	return out, nil
}

// CumulativeShare avoids drift when an order is consumed in many tiny pieces.
func CumulativeShare(value, totalQuantity, consumedQuantity, alreadyPosted Decimal) (Decimal, *big.Rat, error) {
	if value.Sign() < 0 || totalQuantity.Sign() <= 0 || consumedQuantity.Sign() < 0 || consumedQuantity.Cmp(totalQuantity) > 0 || alreadyPosted.Sign() < 0 {
		return Zero(), nil, ErrAmount
	}
	exact := new(big.Rat).Mul(value.Rat(), new(big.Rat).Quo(consumedQuantity.Rat(), totalQuantity.Rat()))
	target, rem := Quantize(exact)
	delta := target.Sub(alreadyPosted)
	if delta.Sign() < 0 {
		return Zero(), nil, ErrAmount
	}
	return delta, rem, nil
}
