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
)

type GrantSource struct {
	Kind   string  `json:"kind"`
	Amount Decimal `json:"amount"`
}
type GrantInput struct {
	OperatorID  string        `json:"operator_id"`
	CustomerID  string        `json:"customer_id"`
	OrderID     string        `json:"order_id"`
	BusinessKey string        `json:"business_key"`
	ActorID     string        `json:"actor_id"`
	Reason      string        `json:"reason"`
	PaidMoney   Decimal       `json:"paid_money"`
	Sources     []GrantSource `json:"sources"`
	EffectiveAt time.Time     `json:"effective_at"`
	ExpiresAt   *time.Time    `json:"expires_at,omitempty"`
}
type Grant struct {
	ID                 string     `json:"id"`
	OperatorID         string     `json:"operator_id"`
	OriginalOperatorID string     `json:"original_operator_id"`
	CustomerID         string     `json:"customer_id"`
	AccountID          string     `json:"account_id"`
	OrderID            string     `json:"order_id"`
	BusinessKey        string     `json:"business_key"`
	Fingerprint        string     `json:"-"`
	PaidMoney          Decimal    `json:"paid_money"`
	Total              Decimal    `json:"total"`
	Consumed           Decimal    `json:"consumed"`
	Expired            Decimal    `json:"expired"`
	Cancelled          Decimal    `json:"cancelled"`
	ConsumedPaidMoney  Decimal    `json:"consumed_paid_money"`
	ExpiredPaidMoney   Decimal    `json:"expired_paid_money"`
	CancelledPaidMoney Decimal    `json:"cancelled_paid_money"`
	EffectiveAt        time.Time  `json:"effective_at"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

func (Grant) TableName() string { return "ledger_grants" }

type grantSourceRow struct {
	GrantID   string
	Kind      string
	Quantity  Decimal
	Consumed  Decimal
	Expired   Decimal
	Cancelled Decimal
}

func (grantSourceRow) TableName() string { return "ledger_grant_sources" }

type GrantEvent struct {
	ID          string          `json:"id"`
	GrantID     string          `json:"grant_id"`
	BusinessKey string          `json:"business_key"`
	Kind        string          `json:"kind"`
	Amount      Decimal         `json:"amount"`
	PaidMoney   Decimal         `json:"paid_money"`
	Sources     json.RawMessage `json:"sources" gorm:"type:jsonb"`
	JournalID   *string         `json:"journal_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (GrantEvent) TableName() string { return "ledger_grant_events" }

type CreditResult struct {
	Requested Decimal      `json:"requested"`
	Debited   Decimal      `json:"debited"`
	Shortfall Decimal      `json:"shortfall"`
	PaidMoney Decimal      `json:"paid_money"`
	JournalID *string      `json:"journal_id,omitempty"`
	Events    []GrantEvent `json:"events"`
}
type creditResultRow struct {
	HoldID      string
	Fingerprint string
	Result      json.RawMessage `gorm:"type:jsonb"`
	CreatedAt   time.Time
}

func (creditResultRow) TableName() string { return "ledger_credit_results" }

func lockCustomer(t *gorm.DB, operator, customer string) error {
	return t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", operator+":credits:"+customer).Error
}

func (s *Service) IssueGrant(ctx context.Context, tx *gorm.DB, in GrantInput) (Grant, error) {
	if in.OperatorID == "" || in.CustomerID == "" || in.BusinessKey == "" || in.ActorID == "" || in.Reason == "" || in.PaidMoney.Sign() < 0 || in.EffectiveAt.IsZero() || (in.ExpiresAt != nil && !in.ExpiresAt.After(in.EffectiveAt)) {
		return Grant{}, ErrAmount
	}
	allowed := map[string]bool{"paid": true, "package_gift": true, "merchant_gift": true, "kol_gift": true, "registration_gift": true, "manual_gift": true}
	quantities := map[string]Decimal{}
	total := Zero()
	for _, source := range in.Sources {
		if !allowed[source.Kind] || source.Amount.Sign() <= 0 || !source.Amount.Valid() {
			return Grant{}, ErrAmount
		}
		quantities[source.Kind] = quantities[source.Kind].Add(source.Amount)
		total = total.Add(source.Amount)
	}
	if total.Sign() <= 0 || !total.Valid() || !in.PaidMoney.Valid() || (in.PaidMoney.Sign() > 0 && quantities["paid"].Sign() <= 0) {
		return Grant{}, ErrAmount
	}
	in.Sources = nil
	for k, v := range quantities {
		in.Sources = append(in.Sources, GrantSource{k, v})
	}
	sort.Slice(in.Sources, func(i, j int) bool { return in.Sources[i].Kind < in.Sources[j].Kind })
	var result Grant
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, in.OperatorID, in.CustomerID); e != nil {
			return e
		}
		e := t.Where("operator_id=? AND business_key=?", in.OperatorID, in.BusinessKey).First(&result).Error
		if e == nil {
			if result.Fingerprint != fingerprint(in) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		g := Grant{ID: uuid.NewString(), OperatorID: in.OperatorID, OriginalOperatorID: in.OperatorID, CustomerID: in.CustomerID, OrderID: in.OrderID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), PaidMoney: in.PaidMoney, Total: total, EffectiveAt: in.EffectiveAt.UTC(), ExpiresAt: in.ExpiresAt, CreatedAt: time.Now().UTC()}
		a, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.OperatorID, OwnerID: in.CustomerID, Purpose: "customer:atg:" + g.ID, Unit: "ATG"})
		if e != nil {
			return e
		}
		g.AccountID = a.ID
		control, e := s.EnsureAccount(ctx, t, Account{OperatorID: in.OperatorID, OwnerID: in.OperatorID, Purpose: "control:credits", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		if e = t.Create(&g).Error; e != nil {
			return e
		}
		for _, source := range in.Sources {
			if e = t.Create(&grantSourceRow{GrantID: g.ID, Kind: source.Kind, Quantity: source.Amount}).Error; e != nil {
				return e
			}
		}
		_, e = s.Post(ctx, t, Posting{OperatorID: in.OperatorID, BusinessKey: "grant:" + in.BusinessKey, Kind: "grant", SourceID: g.ID, ActorID: in.ActorID, Reason: in.Reason, Lines: []Line{{a.ID, total}, {control.ID, total.Neg()}}})
		if e != nil {
			return e
		}
		result = g
		return nil
	})
	return result, err
}

func (s *Service) CustomerGrants(ctx context.Context, tx *gorm.DB, operator, customer string) ([]Grant, error) {
	var rows []Grant
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("operator_id=? AND customer_id=?", operator, customer).Order("expires_at ASC NULLS LAST,effective_at ASC,id ASC").Find(&rows).Error
	})
	return rows, err
}

type CreditReservation struct {
	OperatorID, CustomerID, BusinessKey string
	Estimate                            Decimal
	At, ExpiresAt                       time.Time
}

// The customer advisory lock is held by the caller. Fetch only the FEFO prefix
// needed for this debit; a long purchase history must not lock every grant on
// every model request. Availability is checked again under account row locks.
func spendableGrants(t *gorm.DB, operator, customer string, at time.Time, required Decimal) ([]Grant, error) {
	rows := []Grant{}
	if required.Sign() <= 0 {
		return rows, nil
	}
	total := Zero()
	for offset := 0; ; offset += 64 {
		var page []struct {
			Grant     `gorm:"embedded"`
			Available Decimal
		}
		err := t.Table("ledger_grants g").Select("g.*, (a.balance-a.held)::text AS available").Joins("JOIN ledger_accounts a ON a.id=g.account_id").Where("g.operator_id=? AND g.customer_id=? AND g.effective_at<=? AND (g.expires_at IS NULL OR g.expires_at>?) AND a.balance>a.held", operator, customer, at, at).Order("g.expires_at ASC NULLS LAST,g.effective_at ASC,g.id ASC").Limit(64).Offset(offset).Scan(&page).Error
		if err != nil {
			return nil, err
		}
		for _, v := range page {
			rows = append(rows, v.Grant)
			total = total.Add(v.Available)
			if total.Cmp(required) >= 0 {
				return rows, nil
			}
		}
		if len(page) < 64 {
			return rows, nil
		}
	}
}

func sortGrantsFEFO(grants []Grant) {
	sort.Slice(grants, func(i, j int) bool {
		a, b := grants[i], grants[j]
		if a.ExpiresAt == nil && b.ExpiresAt != nil {
			return false
		}
		if a.ExpiresAt != nil && b.ExpiresAt == nil {
			return true
		}
		if a.ExpiresAt != nil && !a.ExpiresAt.Equal(*b.ExpiresAt) {
			return a.ExpiresAt.Before(*b.ExpiresAt)
		}
		if !a.EffectiveAt.Equal(b.EffectiveAt) {
			return a.EffectiveAt.Before(b.EffectiveAt)
		}
		return a.ID < b.ID
	})
}

func (s *Service) ReserveCredits(ctx context.Context, tx *gorm.DB, in CreditReservation) (Hold, error) {
	if in.Estimate.Sign() <= 0 {
		return Hold{}, ErrAmount
	}
	var result Hold
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, in.OperatorID, in.CustomerID); e != nil {
			return e
		}
		var old Hold
		e := t.Where("operator_id=? AND business_key=?", in.OperatorID, in.BusinessKey).First(&old).Error
		if e == nil {
			var parts []HoldAllocation
			if e = t.Where("hold_id=?", old.ID).Find(&parts).Error; e != nil {
				return e
			}
			sum := Zero()
			for _, part := range parts {
				var g Grant
				if e = t.Where("account_id=? AND customer_id=? AND operator_id=?", part.AccountID, in.CustomerID, in.OperatorID).First(&g).Error; e != nil {
					return ErrScope
				}
				sum = sum.Add(part.Amount)
			}
			if sum.Cmp(in.Estimate) != 0 || !old.ExpiresAt.Equal(in.ExpiresAt) {
				return ErrConflict
			}
			result = old
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		grants, e := spendableGrants(t, in.OperatorID, in.CustomerID, in.At, in.Estimate)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, g := range grants {
			ids = append(ids, g.AccountID)
		}
		accounts, e := lockAccounts(t, ids)
		if e != nil {
			return e
		}
		remaining := in.Estimate
		amounts := []Line{}
		for _, g := range grants {
			if g.EffectiveAt.After(in.At) || (g.ExpiresAt != nil && !g.ExpiresAt.After(in.At)) {
				continue
			}
			available := accounts[g.AccountID].Available()
			take := minimum(available, remaining)
			if take.Sign() > 0 {
				amounts = append(amounts, Line{g.AccountID, take})
				remaining = remaining.Sub(take)
			}
			if remaining.IsZero() {
				break
			}
		}
		if remaining.Sign() > 0 {
			return ErrInsufficient
		}
		result, e = s.Reserve(ctx, t, Reservation{in.OperatorID, in.BusinessKey, "model", in.ExpiresAt, amounts})
		return e
	})
	return result, err
}
func minimum(a, b Decimal) Decimal {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

// SettleCredits returns the billable paid basis and merchant shortfall. It never
// borrows another request's held amount and honors expired grants only up to this hold.
func (s *Service) SettleCredits(ctx context.Context, tx *gorm.DB, operator, customer, holdID, businessKey string, actual Decimal, at time.Time) (CreditResult, error) {
	if actual.Sign() < 0 || !actual.Valid() {
		return CreditResult{}, ErrAmount
	}
	var result CreditResult
	identity := struct{ Operator, Customer, Hold, Key, Actual string }{operator, customer, holdID, businessKey, actual.String()}
	fp := fingerprint(identity)
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, operator, customer); e != nil {
			return e
		}
		var existing creditResultRow
		e := t.First(&existing, "hold_id=?", holdID).Error
		if e == nil {
			if existing.Fingerprint != fp {
				return ErrConflict
			}
			return json.Unmarshal(existing.Result, &result)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var hold Hold
		if e = t.Where("id=? AND operator_id=?", holdID, operator).First(&hold).Error; e != nil {
			return e
		}
		if hold.Status != "active" {
			return ErrHoldState
		}
		var held []HoldAllocation
		if e = t.Where("hold_id=?", holdID).Find(&held).Error; e != nil {
			return e
		}
		heldIDs := []string{}
		heldTotal := Zero()
		for _, h := range held {
			heldIDs = append(heldIDs, h.AccountID)
			heldTotal = heldTotal.Add(h.Amount)
		}
		var grants []Grant
		if len(heldIDs) > 0 {
			if e = t.Where("operator_id=? AND customer_id=? AND account_id IN ?", operator, customer, heldIDs).Find(&grants).Error; e != nil {
				return e
			}
		}
		extraGrants, e := spendableGrants(t, operator, customer, at, actual.Sub(heldTotal))
		if e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, g := range grants {
			seen[g.ID] = true
		}
		for _, g := range extraGrants {
			if !seen[g.ID] {
				grants = append(grants, g)
				seen[g.ID] = true
			}
		}
		sortGrantsFEFO(grants)
		ids := []string{}
		byAccount := map[string]Grant{}
		for _, g := range grants {
			ids = append(ids, g.AccountID)
			byAccount[g.AccountID] = g
		}
		accounts, e := lockAccounts(t, ids)
		if e != nil {
			return e
		}
		heldAmounts := map[string]Decimal{}
		for _, h := range held {
			if _, ok := byAccount[h.AccountID]; !ok {
				return ErrScope
			}
			heldAmounts[h.AccountID] = h.Amount
		}
		remaining := actual
		selected := map[string]Decimal{}
		for _, g := range grants {
			take := minimum(heldAmounts[g.AccountID], remaining)
			if take.Sign() > 0 {
				selected[g.ID] = take
				remaining = remaining.Sub(take)
			}
		}
		for _, g := range grants {
			if remaining.IsZero() {
				break
			}
			if g.EffectiveAt.After(at) || (g.ExpiresAt != nil && !g.ExpiresAt.After(at)) {
				continue
			}
			take := minimum(accounts[g.AccountID].Available(), remaining)
			if take.Sign() > 0 {
				selected[g.ID] = selected[g.ID].Add(take)
				remaining = remaining.Sub(take)
			}
		}
		result = CreditResult{Requested: actual, Debited: actual.Sub(remaining), Shortfall: remaining, Events: []GrantEvent{}}
		if result.Debited.IsZero() {
			_, e = s.Release(ctx, t, operator, holdID, businessKey)
			if e != nil {
				return e
			}
		} else {
			control, e := s.EnsureAccount(ctx, t, Account{OperatorID: operator, OwnerID: operator, Purpose: "control:credits", Unit: "ATG", AllowNegative: true})
			if e != nil {
				return e
			}
			p := Posting{OperatorID: operator, BusinessKey: businessKey, Kind: "usage", SourceID: holdID, ActorID: "system:metering", Reason: "confirmed model usage", Lines: []Line{{control.ID, result.Debited}}}
			extra := map[string]bool{}
			for _, g := range grants {
				if n := selected[g.ID]; n.Sign() > 0 {
					p.Lines = append(p.Lines, Line{g.AccountID, n.Neg()})
					if g.ExpiresAt == nil || g.ExpiresAt.After(at) {
						extra[g.AccountID] = true
					}
				}
			}
			j, e := s.Capture(ctx, t, holdID, p, extra)
			if e != nil {
				return e
			}
			result.JournalID = &j.ID
			for _, g := range grants {
				if n := selected[g.ID]; n.Sign() > 0 {
					event, e := s.attributeGrant(t, g, n, "usage", businessKey, &j.ID, at)
					if e != nil {
						return e
					}
					result.Events = append(result.Events, event)
					result.PaidMoney = result.PaidMoney.Add(event.PaidMoney)
				}
			}
		}
		encoded, e := json.Marshal(result)
		if e != nil {
			return e
		}
		return t.Create(&creditResultRow{HoldID: holdID, Fingerprint: fp, Result: encoded, CreatedAt: at}).Error
	})
	return result, err
}

func (s *Service) attributeGrant(t *gorm.DB, g Grant, n Decimal, kind, key string, journal *string, at time.Time) (GrantEvent, error) {
	var sources []grantSourceRow
	if e := t.Where("grant_id=?", g.ID).Order("kind ASC").Find(&sources).Error; e != nil {
		return GrantEvent{}, e
	}
	weights := []Decimal{}
	for _, source := range sources {
		weights = append(weights, source.Quantity.Sub(source.Consumed).Sub(source.Expired).Sub(source.Cancelled))
	}
	split, e := Proportional(n, weights)
	if e != nil {
		return GrantEvent{}, e
	}
	allocations := []GrantSource{}
	for i, source := range sources {
		if split[i].IsZero() {
			continue
		}
		column := "consumed"
		old := source.Consumed
		if kind == "expiry" {
			column = "expired"
			old = source.Expired
		}
		if kind == "cancellation" {
			column = "cancelled"
			old = source.Cancelled
		}
		if e = t.Model(&grantSourceRow{}).Where("grant_id=? AND kind=?", g.ID, source.Kind).Update(column, old.Add(split[i])).Error; e != nil {
			return GrantEvent{}, e
		}
		allocations = append(allocations, GrantSource{source.Kind, split[i]})
	}
	// Compute a cumulative target across both disjoint attribution classes.
	attributed := g.Consumed.Add(g.Expired).Add(g.Cancelled).Add(n)
	target, _ := Quantize(new(big.Rat).Mul(g.PaidMoney.Rat(), new(big.Rat).Quo(attributed.Rat(), g.Total.Rat())))
	paid := target.Sub(g.ConsumedPaidMoney).Sub(g.ExpiredPaidMoney).Sub(g.CancelledPaidMoney)
	updates := map[string]any{}
	if kind == "usage" {
		updates["consumed"] = g.Consumed.Add(n)
		updates["consumed_paid_money"] = g.ConsumedPaidMoney.Add(paid)
	} else if kind == "expiry" {
		updates["expired"] = g.Expired.Add(n)
		updates["expired_paid_money"] = g.ExpiredPaidMoney.Add(paid)
	} else if kind == "cancellation" {
		updates["cancelled"] = g.Cancelled.Add(n)
		updates["cancelled_paid_money"] = g.CancelledPaidMoney.Add(paid)
	} else {
		return GrantEvent{}, ErrAmount
	}
	if e = t.Model(&Grant{}).Where("id=?", g.ID).Updates(updates).Error; e != nil {
		return GrantEvent{}, e
	}
	encoded, _ := json.Marshal(allocations)
	event := GrantEvent{ID: uuid.NewString(), GrantID: g.ID, BusinessKey: key, Kind: kind, Amount: n, PaidMoney: paid, Sources: encoded, JournalID: journal, CreatedAt: at}
	e = t.Create(&event).Error
	return event, e
}

// ExpireCredits removes only unheld quantities. Reserved units are classified
// after the original request resolves; no remaining procurement balance is added.
func (s *Service) ExpireCredits(ctx context.Context, tx *gorm.DB, operator, customer string, at time.Time) ([]GrantEvent, error) {
	events := []GrantEvent{}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockCustomer(t, operator, customer); e != nil {
			return e
		}
		var grants []Grant
		if e := t.Table("ledger_grants g").Select("g.*").Joins("JOIN ledger_accounts a ON a.id=g.account_id").Where("g.operator_id=? AND g.customer_id=? AND g.expires_at<=? AND a.balance>a.held", operator, customer, at).Order("g.expires_at ASC,g.effective_at ASC,g.id ASC").Scan(&grants).Error; e != nil {
			return e
		}
		if len(grants) == 0 {
			return nil
		}
		ids := []string{}
		for _, g := range grants {
			ids = append(ids, g.AccountID)
		}
		accounts, e := lockAccounts(t, ids)
		if e != nil {
			return e
		}
		control, e := s.EnsureAccount(ctx, t, Account{OperatorID: operator, OwnerID: operator, Purpose: "control:credits", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		for _, g := range grants {
			if g.ExpiresAt == nil || g.ExpiresAt.After(at) {
				continue
			}
			n := accounts[g.AccountID].Available()
			if n.Sign() <= 0 {
				continue
			}
			key := "expiry:" + g.ID + ":" + g.Expired.String()
			j, e := s.Post(ctx, t, Posting{OperatorID: operator, BusinessKey: key, Kind: "expiry", SourceID: g.ID, ActorID: "system:expiry", Reason: "grant term ended", Lines: []Line{{g.AccountID, n.Neg()}, {control.ID, n}}})
			if e != nil {
				return e
			}
			event, e := s.attributeGrant(t, g, n, "expiry", key, &j.ID, at)
			if e != nil {
				return e
			}
			events = append(events, event)
		}
		return nil
	})
	return events, err
}
