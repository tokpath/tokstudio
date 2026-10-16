package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerTransfer struct {
	FromOperatorID, ToOperatorID, TakeoverID, CustomerID, ActorID, Evidence string
	At                                                                      time.Time
}
type TransferMapping struct {
	AccountIDs map[string]string `json:"account_ids"`
	HoldIDs    map[string]string `json:"hold_ids"`
}
type transferRow struct {
	ID, TakeoverID, FromOperatorID, ToOperatorID, CustomerID, Fingerprint string
	AccountIDs, HoldIDs                                                   json.RawMessage `gorm:"type:jsonb"`
	ActorID, Evidence                                                     string
	CreatedAt                                                             time.Time
}

func (transferRow) TableName() string { return "ledger_customer_transfers" }

// TransferCustomerAssets requires the coordinator's exclusive organization
// ownership fence and a single encompassing transaction. Every old journal and
// account survives. Two balanced journals record each transferred asset class;
// active reservations are re-created against the destination, never dropped.
func (s *Service) TransferCustomerAssets(ctx context.Context, t *gorm.DB, in CustomerTransfer) (TransferMapping, error) {
	out := TransferMapping{AccountIDs: map[string]string{}, HoldIDs: map[string]string{}}
	if t == nil || in.FromOperatorID == in.ToOperatorID || in.TakeoverID == "" || in.CustomerID == "" || in.ActorID == "" || in.Evidence == "" || in.At.IsZero() {
		return out, ErrScope
	}
	fp := fingerprint([]string{in.FromOperatorID, in.ToOperatorID, in.TakeoverID, in.CustomerID, in.ActorID, in.Evidence})
	var old transferRow
	e := t.First(&old, "takeover_id=? AND customer_id=?", in.TakeoverID, in.CustomerID).Error
	if e == nil {
		if old.Fingerprint != fp {
			return out, ErrConflict
		}
		if e = json.Unmarshal(old.AccountIDs, &out.AccountIDs); e != nil {
			return out, e
		}
		e = json.Unmarshal(old.HoldIDs, &out.HoldIDs)
		return out, e
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	ops := []string{in.FromOperatorID, in.ToOperatorID}
	sort.Strings(ops)
	for _, op := range ops {
		if e = lockCustomer(t, op, in.CustomerID); e != nil {
			return out, e
		}
	}
	var accounts []Account
	if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND owner_id=?", in.FromOperatorID, in.CustomerID).Order("id").Find(&accounts).Error; e != nil {
		return out, e
	}
	ids := []string{}
	originals := map[string]Account{}
	for _, a := range accounts {
		ids = append(ids, a.ID)
		originals[a.ID] = a
		if a.AllowNegative && !a.Balance.IsZero() {
			return out, ErrScope
		}
		dest, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.ToOperatorID, OwnerID: in.CustomerID, Purpose: a.Purpose, Unit: a.Unit, AllowNegative: a.AllowNegative})
		if e != nil {
			return out, e
		}
		out.AccountIDs[a.ID] = dest.ID
	}
	var holds []Hold
	if len(ids) > 0 {
		if e = t.Table("ledger_holds h").Select("DISTINCT h.*").Joins("JOIN ledger_hold_allocations a ON a.hold_id=h.id").Where("h.operator_id=? AND h.status='active' AND a.account_id IN ?", in.FromOperatorID, ids).Order("h.id").Scan(&holds).Error; e != nil {
			return out, e
		}
	}
	reservations := []Reservation{}
	for _, h := range holds {
		_, allocs, e := s.HoldDetails(ctx, t, in.FromOperatorID, h.ID)
		if e != nil {
			return out, e
		}
		r := Reservation{OperatorID: in.ToOperatorID, BusinessKey: "takeover:" + in.TakeoverID + ":" + h.ID, Purpose: h.Purpose, ExpiresAt: h.ExpiresAt}
		if !r.ExpiresAt.After(time.Now().UTC()) {
			return out, ErrHoldState
		}
		for _, a := range allocs {
			id, ok := out.AccountIDs[a.AccountID]
			if !ok {
				return out, ErrScope
			}
			r.Amounts = append(r.Amounts, Line{id, a.Amount})
		}
		if _, e = s.Release(ctx, t, in.FromOperatorID, h.ID, "takeover:"+in.TakeoverID); e != nil {
			return out, e
		}
		reservations = append(reservations, r)
	}
	for _, unit := range []string{"USD", "ATG"} {
		fromLines, toLines := []Line{}, []Line{}
		total := Zero()
		for _, a := range accounts {
			if a.Unit != unit || a.Balance.IsZero() {
				continue
			}
			fromLines = append(fromLines, Line{a.ID, a.Balance.Neg()})
			toLines = append(toLines, Line{out.AccountIDs[a.ID], a.Balance})
			total = total.Add(a.Balance)
		}
		if total.IsZero() {
			continue
		}
		source, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.FromOperatorID, OwnerID: in.FromOperatorID, Purpose: "control:takeover", Unit: unit, AllowNegative: true})
		if e != nil {
			return out, e
		}
		destination, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.ToOperatorID, OwnerID: in.ToOperatorID, Purpose: "control:takeover", Unit: unit, AllowNegative: true})
		if e != nil {
			return out, e
		}
		fromLines = append(fromLines, Line{source.ID, total})
		toLines = append(toLines, Line{destination.ID, total.Neg()})
		key := "takeover:" + in.TakeoverID + ":" + in.CustomerID + ":" + unit
		if _, e = s.Post(ctx, t, Posting{OperatorID: in.FromOperatorID, BusinessKey: key, Kind: "customer_responsibility_out", SourceID: in.TakeoverID, ActorID: in.ActorID, Reason: in.Evidence, Lines: fromLines}); e != nil {
			return out, e
		}
		if _, e = s.Post(ctx, t, Posting{OperatorID: in.ToOperatorID, BusinessKey: key, Kind: "customer_responsibility_in", SourceID: in.TakeoverID, ActorID: in.ActorID, Reason: in.Evidence, Lines: toLines}); e != nil {
			return out, e
		}
	}
	for i, r := range reservations {
		h, e := s.Reserve(ctx, t, r)
		if e != nil {
			return out, e
		}
		out.HoldIDs[holds[i].ID] = h.ID
	}
	for oldID, newID := range out.AccountIDs {
		if e = t.Model(&Grant{}).Where("account_id=? AND operator_id=? AND customer_id=?", oldID, in.FromOperatorID, in.CustomerID).Updates(map[string]any{"operator_id": in.ToOperatorID, "account_id": newID}).Error; e != nil {
			return out, e
		}
		// Rounding state is an asset provenance projection too; carry fractions must
		// not vanish or survive twice after ownership changes.
		var carries []struct{ SourceKey, Numerator, Denominator string }
		if e = t.Table("ledger_carries").Where("account_id=?", oldID).Find(&carries).Error; e != nil {
			return out, e
		}
		for _, carry := range carries {
			var existing struct{ Numerator, Denominator string }
			q := t.Table("ledger_carries").Where("account_id=? AND source_key=?", newID, carry.SourceKey).Find(&existing)
			if q.Error != nil {
				return out, q.Error
			}
			a, ok := new(big.Rat).SetString(carry.Numerator + "/" + carry.Denominator)
			if !ok {
				return out, ErrAmount
			}
			if q.RowsAffected > 0 {
				b, ok := new(big.Rat).SetString(existing.Numerator + "/" + existing.Denominator)
				if !ok {
					return out, ErrAmount
				}
				a.Add(a, b)
			}
			if e = t.Exec("INSERT INTO ledger_carries(account_id,source_key,numerator,denominator,version) VALUES(?,?,?,?,1) ON CONFLICT(account_id,source_key) DO UPDATE SET numerator=EXCLUDED.numerator,denominator=EXCLUDED.denominator,version=ledger_carries.version+1", newID, carry.SourceKey, a.Num().String(), a.Denom().String()).Error; e != nil {
				return out, e
			}
			if e = t.Exec("UPDATE ledger_carries SET numerator='0',denominator='1',version=version+1 WHERE account_id=? AND source_key=?", oldID, carry.SourceKey).Error; e != nil {
				return out, e
			}
		}
	}
	a, _ := json.Marshal(out.AccountIDs)
	h, _ := json.Marshal(out.HoldIDs)
	row := transferRow{ID: uuid.NewString(), TakeoverID: in.TakeoverID, FromOperatorID: in.FromOperatorID, ToOperatorID: in.ToOperatorID, CustomerID: in.CustomerID, Fingerprint: fp, AccountIDs: a, HoldIDs: h, ActorID: in.ActorID, Evidence: in.Evidence, CreatedAt: in.At}
	return out, t.Create(&row).Error
}
