package ledger

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IncreaseCreditReservation preserves the original allocations, including any
// that have expired, and adds only effective, currently available customer ATG.
// The caller holds the usage request lock shared by settlement and release.
func (s *Service) IncreaseCreditReservation(ctx context.Context, tx *gorm.DB, holdID string, in CreditReservation) error {
	if tx == nil || holdID == "" || in.Estimate.Sign() <= 0 || !in.Estimate.Valid() {
		return ErrAmount
	}
	t := tx.WithContext(ctx)
	if err := lockCustomer(t, in.OperatorID, in.CustomerID); err != nil {
		return err
	}
	var current Decimal
	if err := t.Raw(`SELECT COALESCE(sum(a.amount), 0)::text FROM ledger_hold_allocations a
JOIN ledger_grants g ON g.account_id=a.account_id AND g.operator_id=? AND g.customer_id=?
WHERE a.hold_id=?`, in.OperatorID, in.CustomerID, holdID).Row().Scan(&current); err != nil {
		return err
	}
	grants, err := spendableGrants(t, in.OperatorID, in.CustomerID, in.At, in.Estimate.Sub(current))
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(grants))
	for _, grant := range grants {
		ids = append(ids, grant.AccountID)
	}
	hold, allocations, accounts, err := lockHold(t, in.OperatorID, holdID, ids)
	if err != nil {
		return err
	}
	if hold.Status != "active" || hold.Purpose != "model" || hold.BusinessKey != in.BusinessKey || !hold.ExpiresAt.Equal(in.ExpiresAt) || !hold.ExpiresAt.After(in.At) {
		return ErrHoldState
	}
	var held Decimal
	for _, allocation := range allocations {
		account := accounts[allocation.AccountID]
		if account.OwnerID != in.CustomerID || account.Unit != "ATG" {
			return ErrScope
		}
		held = held.Add(allocation.Amount)
	}
	if held.Cmp(in.Estimate) >= 0 {
		return nil
	}
	amounts, err := allocateGrantCredits(grants, accounts, in.At, in.Estimate.Sub(held))
	if err != nil {
		return err
	}
	return increaseHoldAllocations(t, hold, allocations, accounts, amounts)
}

// IncreaseProcurementHold requires the supplier relationship lock and the usage
// request lock. The only permitted source is the buyer's procurement ATG.
func (s *Service) IncreaseProcurementHold(ctx context.Context, tx *gorm.DB, operator, holdID, businessKey, accountID string, target Decimal, expiry time.Time) error {
	if tx == nil || target.Sign() <= 0 || !target.Valid() {
		return ErrAmount
	}
	t := tx.WithContext(ctx)
	hold, allocations, accounts, err := lockHold(t, operator, holdID, []string{accountID})
	if err != nil {
		return err
	}
	account := accounts[accountID]
	if hold.Status != "active" || hold.Purpose != "procurement" || hold.BusinessKey != businessKey || !hold.ExpiresAt.Equal(expiry) ||
		len(allocations) != 1 || allocations[0].AccountID != accountID || account.OwnerID != operator || account.Purpose != "procurement:atg" || account.Unit != "ATG" {
		return ErrHoldState
	}
	delta := target.Sub(allocations[0].Amount)
	if delta.Sign() <= 0 {
		return nil
	}
	return increaseHoldAllocations(t, hold, allocations, accounts, []Line{{AccountID: accountID, Amount: delta}})
}

func increaseHoldAllocations(t *gorm.DB, hold Hold, current []HoldAllocation, accounts map[string]Account, increments []Line) error {
	totals := make(map[string]Decimal, len(current)+len(increments))
	for _, allocation := range current {
		totals[allocation.AccountID] = allocation.Amount
	}
	for _, increment := range increments {
		account := accounts[increment.AccountID]
		if account.OperatorID != hold.OperatorID || account.AllowNegative || account.Unit != "ATG" {
			return ErrScope
		}
		if account.Available().Cmp(increment.Amount) < 0 {
			return ErrInsufficient
		}
		totals[account.ID] = totals[account.ID].Add(increment.Amount)
		if err := t.Model(&Account{}).Where("id=?", account.ID).Updates(map[string]any{"held": account.Held.Add(increment.Amount), "version": gorm.Expr("version+1")}).Error; err != nil {
			return err
		}
		allocation := HoldAllocation{HoldID: hold.ID, AccountID: account.ID, Amount: totals[account.ID]}
		if err := t.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hold_id"}, {Name: "account_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"amount"}),
		}).Create(&allocation).Error; err != nil {
			return err
		}
	}
	updated := Reservation{OperatorID: hold.OperatorID, BusinessKey: hold.BusinessKey, Purpose: hold.Purpose, ExpiresAt: hold.ExpiresAt.UTC()}
	for id, amount := range totals {
		updated.Amounts = append(updated.Amounts, Line{AccountID: id, Amount: amount})
	}
	sort.Slice(updated.Amounts, func(i, j int) bool { return updated.Amounts[i].AccountID < updated.Amounts[j].AccountID })
	return t.Model(&Hold{}).Where("id=?", hold.ID).Update("fingerprint", fingerprint(updated)).Error
}
