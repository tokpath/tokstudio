package organization

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrFrozen = errors.New("organization: requested action is frozen in this scope")

type CustomerLifecycleInput struct {
	CustomerID, OperatorID, ActorID, Evidence, WaiverEvidence string
	ExpectedVersion                                           int64
	EffectiveAt                                               time.Time
}

// CustomerLifecycleHandler joins real Key/subscription/ledger/incentive state
// in the caller's transaction. Missing handlers never report a successful stop
// or closure while leaving future billing or obligations unresolved.
type CustomerLifecycleHandler interface {
	Stop(context.Context, *gorm.DB, CustomerLifecycleInput) error
	BeginClose(context.Context, *gorm.DB, CustomerLifecycleInput) error
	CheckClose(context.Context, *gorm.DB, CustomerLifecycleInput) ([]Requirement, error)
	Close(context.Context, *gorm.DB, CustomerLifecycleInput) error
}

var CustomerClosureRequirements = []string{"in_flight", "pending_usage", "pending_receipts", "money_balance", "commission_frozen", "commission_available", "payouts", "pending_offsets", "disputes", "remaining_paid_grants", "remaining_gifts"}

func customerCanAuthenticate(status string) bool {
	return status == "active" || status == "stopped" || status == "closing"
}
func (s *Service) customerLifecycle(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput, target string, handler CustomerLifecycleHandler) (out Customer, err error) {
	if !validID(in.CustomerID) || !validID(in.OperatorID) || in.ActorID == "" || strings.TrimSpace(in.Evidence) == "" || in.ExpectedVersion < 1 {
		return out, ErrInvalidInput
	}
	if handler == nil {
		return out, ErrReadiness
	}
	if target == "closing" && in.ActorID != in.CustomerID {
		return out, ErrForbidden
	}
	err = s.withTx(ctx, tx, func(t *gorm.DB) error {
		own, e := s.CurrentOwnership(ctx, t, in.CustomerID)
		if e != nil {
			return e
		}
		if own.OperatorID != in.OperatorID {
			return ErrForbidden
		}
		if _, e = s.LockOwnership(ctx, t, own.CustomerID, own.OperatorID, own.Version); e != nil {
			return e
		}
		if in.ActorID != in.CustomerID {
			if e = s.RequirePermission(ctx, t, in.ActorID, in.OperatorID, "security.manage"); e != nil {
				return e
			}
		}
		var c customerRow
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, "id=?", in.CustomerID).Error; e != nil {
			return e
		}
		if c.Status == target {
			out = Customer(c)
			return nil
		}
		if c.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if e = t.Raw(`SELECT clock_timestamp()`).Scan(&in.EffectiveAt).Error; e != nil {
			return e
		}
		in.EffectiveAt = in.EffectiveAt.UTC()
		switch target {
		case "stopped":
			if c.Status != "active" {
				return ErrConflict
			}
			if e = handler.Stop(ctx, t, in); e != nil {
				return e
			}
		case "active":
			if c.Status != "stopped" {
				return ErrConflict
			} // Keys and renewal require explicit re-enablement.
		case "closing":
			if c.Status != "active" && c.Status != "stopped" {
				return ErrConflict
			}
			if e = handler.BeginClose(ctx, t, in); e != nil {
				return e
			}
			if e = t.Model(&codeRow{}).Where("customer_id=? AND kind='user'", in.CustomerID).Update("enabled", false).Error; e != nil {
				return e
			}
		case "closed":
			if c.Status != "closing" {
				return ErrConflict
			}
			checks, e := handler.CheckClose(ctx, t, in)
			if e != nil {
				return e
			}
			byCode := map[string]Requirement{}
			for _, check := range checks {
				byCode[check.Code] = check
			}
			missing := []Requirement{}
			for _, code := range CustomerClosureRequirements {
				r, ok := byCode[code]
				if !ok {
					r = Requirement{Code: code, Description: "closure dependency did not report its status"}
				}
				if !r.Ready {
					missing = append(missing, r)
				}
			}
			if len(missing) > 0 {
				return &ReadinessError{Missing: missing}
			}
			if e = handler.Close(ctx, t, in); e != nil {
				return e
			}
			if e = t.Model(&accessTokenRow{}).Where("customer_id=? AND revoked_at IS NULL", in.CustomerID).Update("revoked_at", in.EffectiveAt).Error; e != nil {
				return e
			}
		default:
			return ErrInvalidInput
		}
		if e = t.Model(&customerRow{}).Where("id=?", in.CustomerID).Updates(map[string]any{"status": target, "version": c.Version + 1}).Error; e != nil {
			return e
		}
		if e = t.Exec(`INSERT INTO organization_lifecycle_events(id,customer_id,operator_id,action,actor_id,evidence,waiver_evidence,effective_at) VALUES(?,?,?,?,?,?,?,?)`, newID(), in.CustomerID, in.OperatorID, target, in.ActorID, in.Evidence, in.WaiverEvidence, in.EffectiveAt).Error; e != nil {
			return e
		}
		c.Status = target
		c.Version++
		out = Customer(c)
		return s.event(t, in.OperatorID, in.ActorID, "customer.lifecycle_changed", in.Evidence, map[string]any{"customer_id": in.CustomerID, "status": target, "effective_at": in.EffectiveAt})
	})
	return
}
func (s *Service) StopUsing(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput, h CustomerLifecycleHandler) (Customer, error) {
	return s.customerLifecycle(ctx, tx, in, "stopped", h)
}
func (s *Service) ResumeUsing(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput, h CustomerLifecycleHandler) (Customer, error) {
	return s.customerLifecycle(ctx, tx, in, "active", h)
}
func (s *Service) BeginCustomerClosure(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput, h CustomerLifecycleHandler) (Customer, error) {
	return s.customerLifecycle(ctx, tx, in, "closing", h)
}
func (s *Service) CompleteCustomerClosure(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput, h CustomerLifecycleHandler) (Customer, error) {
	return s.customerLifecycle(ctx, tx, in, "closed", h)
}

type ScopeFreeze struct {
	ScopeKind   string     `json:"scope_kind" gorm:"primaryKey"`
	ScopeID     string     `json:"scope_id" gorm:"primaryKey"`
	Action      string     `json:"action" gorm:"primaryKey"`
	Enabled     bool       `json:"enabled"`
	Evidence    string     `json:"evidence"`
	Explanation string     `json:"explanation"`
	ActorID     string     `json:"actor_id"`
	ReviewDueAt *time.Time `json:"review_due_at"`
	Version     int64      `json:"version"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (ScopeFreeze) TableName() string { return "organization_scope_freezes" }

type ScopeFreezeInput struct {
	ActorStaffID, OperatorID, ScopeKind, ScopeID, Action, Evidence, Explanation string
	Enabled                                                                     bool
	ExpectedVersion                                                             int64
	ReviewDueAt                                                                 *time.Time
}

func freezeAction(action string) bool {
	switch action {
	case "registration", "new_purchase", "renewal", "model_usage", "activity", "withdrawal", "wallet_refund", "customer_login", "all_new_business":
		return true
	}
	return false
}
func (s *Service) SetScopeFreeze(ctx context.Context, tx *gorm.DB, in ScopeFreezeInput) (out ScopeFreeze, err error) {
	if !validID(in.OperatorID) || !validID(in.ScopeID) || !freezeAction(in.Action) || in.ExpectedVersion < 0 || in.Evidence == "" || in.Explanation == "" || (in.Enabled && (in.ReviewDueAt == nil || !in.ReviewDueAt.After(s.now()))) {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(t *gorm.DB) error {
		if e := s.RequirePermission(ctx, t, in.ActorStaffID, in.OperatorID, "security.manage"); e != nil {
			return e
		}
		op, e := s.GetOperator(ctx, t, in.OperatorID)
		if e != nil {
			return e
		}
		switch in.ScopeKind {
		case "operator":
			if in.ScopeID != op.ID {
				return ErrForbidden
			}
		case "brand":
			b, e := s.GetBrand(ctx, t, in.ScopeID)
			if e != nil {
				return e
			}
			if b.OwnerOperatorID != op.ID {
				return ErrForbidden
			}
		case "customer":
			own, e := s.CurrentOwnership(ctx, t, in.ScopeID)
			if e != nil {
				return e
			}
			if own.OperatorID != op.ID {
				return ErrForbidden
			}
		default:
			return ErrInvalidInput
		}
		if e = t.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "organization:freeze:"+in.ScopeKind+":"+in.ScopeID+":"+in.Action).Error; e != nil {
			return e
		}
		var old ScopeFreeze
		q := t.Where("scope_kind=? AND scope_id=? AND action=?", in.ScopeKind, in.ScopeID, in.Action).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if old.Version != in.ExpectedVersion {
			return ErrConflict
		}
		out = ScopeFreeze{ScopeKind: in.ScopeKind, ScopeID: in.ScopeID, Action: in.Action, Enabled: in.Enabled, Evidence: in.Evidence, Explanation: in.Explanation, ActorID: in.ActorStaffID, ReviewDueAt: in.ReviewDueAt, Version: old.Version + 1, UpdatedAt: s.now()}
		if q.RowsAffected == 0 {
			e = t.Create(&out).Error
		} else {
			e = t.Save(&out).Error
		}
		if e != nil {
			return e
		}
		return s.event(t, in.OperatorID, in.ActorStaffID, "scope.freeze_changed", in.Evidence, out)
	})
	return
}
func (s *Service) RequireUnfrozen(ctx context.Context, tx *gorm.DB, brand, operator, customer, action string) error {
	if action == "settlement" {
		return nil
	}
	if !freezeAction(action) || !validID(brand) || !validID(operator) || (customer != "" && !validID(customer)) {
		return ErrInvalidInput
	}
	actions := []string{action}
	switch action {
	case "registration", "new_purchase", "renewal", "model_usage", "activity":
		actions = append(actions, "all_new_business")
	}
	q := s.dbFor(ctx, tx).Model(&ScopeFreeze{}).Where("enabled=true AND action IN ?", actions)
	if customer == "" {
		q = q.Where("(scope_kind='brand' AND scope_id=?) OR (scope_kind='operator' AND scope_id=?)", brand, operator)
	} else {
		q = q.Where("(scope_kind='brand' AND scope_id=?) OR (scope_kind='operator' AND scope_id=?) OR (scope_kind='customer' AND scope_id=?)", brand, operator, customer)
	}
	var count int64
	if e := q.Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return ErrFrozen
	}
	return nil
}
func (s *Service) RequireCustomerAction(ctx context.Context, tx *gorm.DB, customer, operator, action string) error {
	c, e := s.GetCustomer(ctx, tx, customer)
	if e != nil {
		return e
	}
	own, e := s.CurrentOwnership(ctx, tx, customer)
	if e != nil {
		return e
	}
	if own.OperatorID != operator {
		return ErrForbidden
	}
	switch action {
	case "new_purchase", "renewal", "model_usage", "activity":
		if c.Status != "active" {
			return ErrBusinessClosed
		}
	case "customer_login":
		if !customerCanAuthenticate(c.Status) {
			return ErrInvalidCredentials
		}
	}
	return s.RequireUnfrozen(ctx, tx, c.BrandID, operator, customer, action)
}
