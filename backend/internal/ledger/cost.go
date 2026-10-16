package ledger

import "math/big"

// Position separates procurement quantity from its historical USD book cost.
type Position struct {
	Quantity Decimal `json:"quantity"`
	Cost     Decimal `json:"cost"`
}

func (p Position) Receive(quantity, cost Decimal) (Position, error) {
	if quantity.Sign() <= 0 || cost.Sign() < 0 || p.Quantity.Sign() < 0 || p.Cost.Sign() < 0 {
		return p, ErrAmount
	}
	n := Position{p.Quantity.Add(quantity), p.Cost.Add(cost)}
	if !n.Quantity.Valid() || !n.Cost.Valid() {
		return p, ErrAmount
	}
	return n, nil
}
func (p Position) Remove(quantity Decimal) (Position, Decimal, *big.Rat, error) {
	if quantity.Sign() < 0 || quantity.Cmp(p.Quantity) > 0 || p.Quantity.Sign() <= 0 || p.Cost.Sign() < 0 {
		return p, Zero(), nil, ErrAmount
	}
	exact := new(big.Rat).Mul(p.Cost.Rat(), new(big.Rat).Quo(quantity.Rat(), p.Quantity.Rat()))
	cost, rem := Quantize(exact)
	return Position{p.Quantity.Sub(quantity), p.Cost.Sub(cost)}, cost, rem, nil
}

// Coverage is in the upstream procurement ATG unit, never USD or retail ATG.
type Coverage struct {
	Procurement         Decimal
	ProcurementHeld     Decimal
	Oversell            Decimal
	DirectLiability     Decimal
	DownstreamLiability Decimal
	PendingLiability    Decimal
}

func (c Coverage) Space() (normal, total Decimal, err error) {
	for _, v := range []Decimal{c.Procurement, c.ProcurementHeld, c.Oversell, c.DirectLiability, c.DownstreamLiability, c.PendingLiability} {
		if v.Sign() < 0 || !v.Valid() {
			return Zero(), Zero(), ErrAmount
		}
	}
	if c.ProcurementHeld.Cmp(c.Procurement) > 0 {
		return Zero(), Zero(), ErrAmount
	}
	normal = c.Procurement.Sub(c.ProcurementHeld).Sub(c.DirectLiability).Sub(c.DownstreamLiability).Sub(c.PendingLiability)
	return normal, normal.Add(c.Oversell), nil
}
