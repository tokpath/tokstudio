package cooperation

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

type ProcurementReversal struct {
	ID, BuyerID, RequestID, BusinessKey, Fingerprint string
	Quantity, CostUSD                                ledger.Decimal
	Sources                                          json.RawMessage `gorm:"type:jsonb"`
	JournalID, ActorID, Evidence                     string
	CreatedAt                                        time.Time
}

func (ProcurementReversal) TableName() string { return "cooperation_procurement_reversals" }

// ReverseProcurementUsage restores original deducted source quantities and the
// original settled book cost. Subsequent purchases and current average prices
// cannot change the amount of the historical correction.
func (s *Service) ReverseProcurementUsage(ctx context.Context, tx *gorm.DB, buyer, requestID, businessKey, actor, evidence string, amount ledger.Decimal, at time.Time) (ProcurementReversal, error) {
	out := ProcurementReversal{}
	if tx == nil || amount.Sign() <= 0 || !amount.Valid() || businessKey == "" || actor == "" || evidence == "" || at.IsZero() {
		return out, ErrConflict
	}
	fp := hash([]string{buyer, requestID, businessKey, actor, evidence, amount.String()})
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, buyer); e != nil {
			return e
		}
		q := t.Where("buyer_id=? AND business_key=?", buyer, businessKey).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		var original ProcurementSettlement
		if e := t.First(&original, "buyer_id=? AND request_id=?", buyer, requestID).Error; e != nil {
			return e
		}
		if original.Deducted.Sign() <= 0 {
			return ErrConflict
		}
		previous := []ProcurementReversal{}
		if e := t.Where("buyer_id=? AND request_id=?", buyer, requestID).Order("created_at,id").Find(&previous).Error; e != nil {
			return e
		}
		reversed := ledger.Zero()
		costReversed := ledger.Zero()
		restored := map[string]SourceConsumption{}
		for _, r := range previous {
			reversed = reversed.Add(r.Quantity)
			costReversed = costReversed.Add(r.CostUSD)
			var sources []SourceConsumption
			if json.Unmarshal(r.Sources, &sources) != nil {
				return ErrConflict
			}
			for _, part := range sources {
				v := restored[part.LotID]
				v.PaidATG = v.PaidATG.Add(part.PaidATG)
				v.GiftATG = v.GiftATG.Add(part.GiftATG)
				restored[part.LotID] = v
			}
		}
		cumulative := reversed.Add(amount)
		if cumulative.Cmp(original.Deducted) > 0 {
			return ErrConflict
		}
		exact := new(big.Rat).Mul(original.CostUSD.Rat(), new(big.Rat).Quo(cumulative.Rat(), original.Deducted.Rat()))
		cumulativeCost, _ := ledger.Quantize(exact)
		cost := cumulativeCost.Sub(costReversed)
		if cost.Sign() < 0 {
			return ErrConflict
		}
		var originalSources []SourceConsumption
		if json.Unmarshal(original.SourceAllocations, &originalSources) != nil {
			return ErrConflict
		}
		weights := []ledger.Decimal{}
		for _, src := range originalSources {
			before := restored[src.LotID]
			paid := src.PaidATG.Sub(before.PaidATG)
			gift := src.GiftATG.Sub(before.GiftATG)
			if paid.Sign() < 0 || gift.Sign() < 0 {
				return ErrConflict
			}
			weights = append(weights, paid, gift)
		}
		parts, e := ledger.Proportional(amount, weights)
		if e != nil {
			return e
		}
		sources := []SourceConsumption{}
		for i, src := range originalSources {
			paid, gift := parts[i*2], parts[i*2+1]
			if paid.IsZero() && gift.IsZero() {
				continue
			}
			var lot ProcurementLot
			if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lot, "id=? AND buyer_id=?", src.LotID, buyer).Error; e != nil {
				return e
			}
			if lot.PaidConsumed.Cmp(paid) < 0 || lot.GiftConsumed.Cmp(gift) < 0 {
				return ErrConflict
			}
			if e = t.Model(&lot).Updates(map[string]any{"paid_consumed": lot.PaidConsumed.Sub(paid), "gift_consumed": lot.GiftConsumed.Sub(gift), "remaining": lot.Remaining.Add(paid).Add(gift)}).Error; e != nil {
				return e
			}
			sources = append(sources, SourceConsumption{src.LotID, paid, gift})
		}
		p, e := s.Position(ctx, t, buyer)
		if e != nil {
			return e
		}
		var originalJournal ledger.Journal
		if e = t.Where("operator_id=? AND business_key=?", buyer, "procurement:usage:"+requestID).First(&originalJournal).Error; e != nil {
			return e
		}
		control, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: buyer, OwnerID: buyer, Purpose: "control:procurement", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		journal, e := s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: buyer, BusinessKey: "procurement-reversal:" + businessKey, Kind: "procurement_usage_reversal", SourceID: requestID, OriginalID: &originalJournal.ID, ActorID: actor, Reason: evidence, Lines: []ledger.Line{{AccountID: p.AccountID, Amount: amount}, {AccountID: control.ID, Amount: amount.Neg()}}})
		if e != nil {
			return e
		}
		newQuantity, newCost := p.Quantity.Add(amount), p.Cost.Add(cost)
		if !newQuantity.Valid() || !newCost.Valid() {
			return ledger.ErrAmount
		}
		if e = t.Model(&ProcurementPosition{}).Where("buyer_id=?", buyer).Updates(map[string]any{"quantity": newQuantity, "cost": newCost, "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
		raw, _ := json.Marshal(sources)
		out = ProcurementReversal{ID: uuid.NewString(), BuyerID: buyer, RequestID: requestID, BusinessKey: businessKey, Fingerprint: fp, Quantity: amount, CostUSD: cost, Sources: raw, JournalID: journal.ID, ActorID: actor, Evidence: evidence, CreatedAt: at.UTC()}
		return t.Create(&out).Error
	})
	return out, e
}
