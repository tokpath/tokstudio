package ledger

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Hold struct {
	ID            string     `json:"id"`
	OperatorID    string     `json:"operator_id"`
	BusinessKey   string     `json:"business_key"`
	Fingerprint   string     `json:"-"`
	Purpose       string     `json:"purpose"`
	Status        string     `json:"status"`
	ExpiresAt     time.Time  `json:"expires_at"`
	ResolutionKey *string    `json:"resolution_key,omitempty"`
	JournalID     *string    `json:"journal_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
}

func (Hold) TableName() string { return "ledger_holds" }

type HoldAllocation struct {
	HoldID    string  `json:"hold_id"`
	AccountID string  `json:"account_id"`
	Amount    Decimal `json:"amount"`
}

func (HoldAllocation) TableName() string { return "ledger_hold_allocations" }

type Reservation struct {
	OperatorID  string    `json:"operator_id"`
	BusinessKey string    `json:"business_key"`
	Purpose     string    `json:"purpose"`
	ExpiresAt   time.Time `json:"expires_at"`
	Amounts     []Line    `json:"amounts"`
}

func (s *Service) Reserve(ctx context.Context, tx *gorm.DB, r Reservation) (Hold, error) {
	if r.OperatorID == "" || r.BusinessKey == "" || len(r.BusinessKey) > 200 || r.Purpose == "" || r.ExpiresAt.IsZero() || len(r.Amounts) == 0 {
		return Hold{}, ErrScope
	}
	values := map[string]Decimal{}
	for _, l := range r.Amounts {
		if l.Amount.Sign() <= 0 || !l.Amount.Valid() {
			return Hold{}, ErrAmount
		}
		values[l.AccountID] = values[l.AccountID].Add(l.Amount)
	}
	r.Amounts = nil
	for id, v := range values {
		if !v.Valid() {
			return Hold{}, ErrAmount
		}
		r.Amounts = append(r.Amounts, Line{id, v})
	}
	sort.Slice(r.Amounts, func(i, j int) bool { return r.Amounts[i].AccountID < r.Amounts[j].AccountID })
	r.ExpiresAt = r.ExpiresAt.UTC()
	h := Hold{ID: uuid.NewString(), OperatorID: r.OperatorID, BusinessKey: r.BusinessKey, Fingerprint: fingerprint(r), Purpose: r.Purpose, Status: "active", ExpiresAt: r.ExpiresAt, CreatedAt: time.Now().UTC()}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		insert := t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "business_key"}}, DoNothing: true}).Create(&h)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			var old Hold
			if e := t.Where("operator_id=? AND business_key=?", r.OperatorID, r.BusinessKey).First(&old).Error; e != nil {
				return e
			}
			if old.Fingerprint != h.Fingerprint {
				return ErrConflict
			}
			h = old
			return nil
		}
		if !r.ExpiresAt.After(h.CreatedAt) {
			return ErrHoldState
		}
		ids := []string{}
		for _, l := range r.Amounts {
			ids = append(ids, l.AccountID)
		}
		accounts, e := lockAccounts(t, ids)
		if e != nil {
			return e
		}
		for _, l := range r.Amounts {
			a := accounts[l.AccountID]
			if a.OperatorID != r.OperatorID || a.AllowNegative {
				return ErrScope
			}
			if a.Available().Cmp(l.Amount) < 0 {
				return ErrInsufficient
			}
		}
		for _, l := range r.Amounts {
			a := accounts[l.AccountID]
			if e := t.Model(&Account{}).Where("id=?", a.ID).Updates(map[string]any{"held": a.Held.Add(l.Amount), "version": gorm.Expr("version+1")}).Error; e != nil {
				return e
			}
			if e := t.Create(&HoldAllocation{h.ID, a.ID, l.Amount}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return h, err
}

// lockHold locks accounts first, then the hold, consistently across capture/release.
func lockHold(t *gorm.DB, operator, id string, extra []string) (Hold, []HoldAllocation, map[string]Account, error) {
	var allocations []HoldAllocation
	if e := t.Where("hold_id=?", id).Order("account_id").Find(&allocations).Error; e != nil {
		return Hold{}, nil, nil, e
	}
	ids := append([]string{}, extra...)
	for _, a := range allocations {
		ids = append(ids, a.AccountID)
	}
	accounts, e := lockAccounts(t, ids)
	if e != nil {
		return Hold{}, nil, nil, e
	}
	var h Hold
	if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND operator_id=?", id, operator).First(&h).Error; e != nil {
		return h, nil, nil, e
	}
	for _, a := range accounts {
		if a.OperatorID != operator {
			return h, nil, nil, ErrScope
		}
	}
	return h, allocations, accounts, nil
}

func releaseAllocations(t *gorm.DB, allocations []HoldAllocation, accounts map[string]Account) error {
	for _, l := range allocations {
		a := accounts[l.AccountID]
		if a.Held.Cmp(l.Amount) < 0 {
			return ErrHoldState
		}
		if e := t.Model(&Account{}).Where("id=?", a.ID).Updates(map[string]any{"held": a.Held.Sub(l.Amount), "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) Release(ctx context.Context, tx *gorm.DB, operator, holdID, resolutionKey string) (Hold, error) {
	if resolutionKey == "" {
		return Hold{}, ErrScope
	}
	var result Hold
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		h, a, accounts, e := lockHold(t, operator, holdID, nil)
		if e != nil {
			return e
		}
		result = h
		if h.Status != "active" {
			if h.Status == "released" && h.ResolutionKey != nil && *h.ResolutionKey == resolutionKey {
				return nil
			}
			return ErrHoldState
		}
		if e = releaseAllocations(t, a, accounts); e != nil {
			return e
		}
		now := time.Now().UTC()
		h.Status = "released"
		h.ResolutionKey = &resolutionKey
		h.ResolvedAt = &now
		if e = t.Model(&Hold{}).Where("id=?", h.ID).Updates(map[string]any{"status": h.Status, "resolution_key": resolutionKey, "resolved_at": now}).Error; e != nil {
			return e
		}
		result = h
		return nil
	})
	return result, err
}

// Capture releases the whole reservation and posts actual debits in one transaction.
// allowExtra lists ONLY customer accounts explicitly eligible for unheld overage;
// procurement accounts must never appear in it. All other held assets are bounded.
func (s *Service) Capture(ctx context.Context, tx *gorm.DB, holdID string, p Posting, allowExtra map[string]bool) (Journal, error) {
	p, e := normalizePosting(p)
	if e != nil {
		return Journal{}, e
	}
	var result Journal
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lockPosting(t, p); e != nil {
			return e
		}
		ids := []string{}
		for _, l := range p.Lines {
			ids = append(ids, l.AccountID)
		}
		h, allocations, accounts, e := lockHold(t, p.OperatorID, holdID, ids)
		if e != nil {
			return e
		}
		if h.Status != "active" {
			if h.Status == "captured" && h.JournalID != nil {
				if e = t.First(&result, "id=?", *h.JournalID).Error; e != nil {
					return e
				}
				if result.Fingerprint != fingerprint(p) {
					return ErrConflict
				}
				return nil
			}
			return ErrHoldState
		}
		reserved := map[string]Decimal{}
		for _, a := range allocations {
			reserved[a.AccountID] = a.Amount
		}
		for _, l := range p.Lines {
			a := accounts[l.AccountID]
			if l.Amount.Sign() >= 0 || a.AllowNegative {
				continue
			}
			debit := l.Amount.Neg()
			if allowExtra[l.AccountID] {
				if a.Purpose != "customer:atg" && !strings.HasPrefix(a.Purpose, "customer:atg:") {
					return ErrScope
				}
			} else if debit.Cmp(reserved[l.AccountID]) > 0 {
				return ErrInsufficient
			}
		}
		// A business key used outside this hold must not be adopted to consume it.
		var existing int64
		if e = t.Model(&Journal{}).Where("operator_id=? AND business_key=?", p.OperatorID, p.BusinessKey).Count(&existing).Error; e != nil {
			return e
		}
		if existing != 0 {
			return ErrConflict
		}
		if e = releaseAllocations(t, allocations, accounts); e != nil {
			return e
		}
		result, e = s.post(t, p)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		return t.Model(&Hold{}).Where("id=?", h.ID).Updates(map[string]any{"status": "captured", "resolution_key": p.BusinessKey, "journal_id": result.ID, "resolved_at": now}).Error
	})
	return result, err
}
