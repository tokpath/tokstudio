package organization

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"
	"time"
)

// This adapter exercises organization transaction fences. The application
// tests must supply actual usage/ledger/subscription/commission dependencies.
type lifecycleBoundary struct{ failStop bool }

func (h lifecycleBoundary) Stop(_ context.Context, tx *gorm.DB, _ CustomerLifecycleInput) error {
	if e := tx.Exec(`UPDATE test_lifecycle SET key_active=false,renewal_active=false`).Error; e != nil {
		return e
	}
	if h.failStop {
		return errors.New("injected dependency failure")
	}
	return nil
}
func (h lifecycleBoundary) BeginClose(ctx context.Context, tx *gorm.DB, in CustomerLifecycleInput) error {
	if e := h.Stop(ctx, tx, in); e != nil {
		return e
	}
	return tx.Exec(`UPDATE test_lifecycle SET promotion_active=false`).Error
}
func (h lifecycleBoundary) CheckClose(_ context.Context, tx *gorm.DB, in CustomerLifecycleInput) ([]Requirement, error) {
	var pending bool
	if e := tx.Raw(`SELECT pending_usage FROM test_lifecycle`).Row().Scan(&pending); e != nil {
		return nil, e
	}
	out := []Requirement{}
	for _, code := range CustomerClosureRequirements {
		ready := true
		if code == "pending_usage" {
			ready = !pending
		}
		if code == "remaining_paid_grants" {
			ready = in.WaiverEvidence != ""
		}
		out = append(out, Requirement{Code: code, Ready: ready})
	}
	return out, nil
}
func (h lifecycleBoundary) Close(_ context.Context, tx *gorm.DB, _ CustomerLifecycleInput) error {
	return tx.Exec(`UPDATE test_lifecycle SET gifts_active=false`).Error
}

func TestPostgresCustomerLifecyclePreservesSettlementAccessAndRequiresAllObligations(t *testing.T) {
	w := world(t)
	registered := w.register(t, w.platform.Brand.ID, "lifecycle@example.test", "")
	child := w.register(t, w.platform.Brand.ID, "descendant@example.test", registered.InviteCode)
	if e := w.db.Exec(`CREATE TABLE test_lifecycle(key_active BOOLEAN,renewal_active BOOLEAN,promotion_active BOOLEAN,pending_usage BOOLEAN,gifts_active BOOLEAN); INSERT INTO test_lifecycle VALUES(true,true,true,true,true)`).Error; e != nil {
		t.Fatal(e)
	}
	login, e := w.s.Login(w.ctx, nil, w.platform.Brand.ID, "lifecycle@example.test", testPassword)
	if e != nil {
		t.Fatal(e)
	}
	in := CustomerLifecycleInput{CustomerID: registered.Customer.ID, OperatorID: w.platform.Operator.ID, ActorID: registered.Customer.ID, Evidence: "explicit customer stop request", ExpectedVersion: registered.Customer.Version}
	if _, e = w.s.StopUsing(w.ctx, nil, in, nil); !errors.Is(e, ErrReadiness) {
		t.Fatalf("missing real hooks accepted: %v", e)
	}
	if _, e = w.s.StopUsing(w.ctx, nil, in, lifecycleBoundary{failStop: true}); e == nil {
		t.Fatal("dependency failure ignored")
	}
	var keyActive bool
	if e = w.db.Raw(`SELECT key_active FROM test_lifecycle`).Row().Scan(&keyActive); e != nil || !keyActive {
		t.Fatalf("failed stop leaked key change %v", e)
	}
	stopped, e := w.s.StopUsing(w.ctx, nil, in, lifecycleBoundary{})
	if e != nil || stopped.Status != "stopped" {
		t.Fatalf("stop=%+v %v", stopped, e)
	}
	if _, e = w.s.AuthenticateToken(w.ctx, nil, login.AccessToken); e != nil {
		t.Fatalf("stopped customer lost settlement access: %v", e)
	}
	if _, e = w.s.Login(w.ctx, nil, w.platform.Brand.ID, "lifecycle@example.test", testPassword); e != nil {
		t.Fatal(e)
	}
	if e = w.s.RequireCustomerAction(w.ctx, nil, registered.Customer.ID, w.platform.Operator.ID, "model_usage"); !errors.Is(e, ErrBusinessClosed) {
		t.Fatalf("stopped usage reopened: %v", e)
	}
	if e = w.s.RequireCustomerAction(w.ctx, nil, registered.Customer.ID, w.platform.Operator.ID, "withdrawal"); e != nil {
		t.Fatal("stop removed accrued financial rights", e)
	}
	in.ExpectedVersion = stopped.Version
	resumed, e := w.s.ResumeUsing(w.ctx, nil, in, lifecycleBoundary{})
	if e != nil {
		t.Fatal(e)
	}
	if e = w.db.Raw(`SELECT renewal_active FROM test_lifecycle`).Row().Scan(&keyActive); e != nil || keyActive {
		t.Fatal("resume implicitly restored renewal consent")
	}
	in.ExpectedVersion = resumed.Version
	closing, e := w.s.BeginCustomerClosure(w.ctx, nil, in, lifecycleBoundary{})
	if e != nil || closing.Status != "closing" {
		t.Fatal(e)
	}
	if _, e = w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, registered.InviteCode); !errors.Is(e, ErrInvalidInviteCode) {
		t.Fatalf("closing invite remained active: %v", e)
	}
	if _, e = w.s.AuthenticateToken(w.ctx, nil, login.AccessToken); e != nil {
		t.Fatalf("closing customer lost pending settlement access: %v", e)
	}
	in.ExpectedVersion = closing.Version
	if _, e = w.s.CompleteCustomerClosure(w.ctx, nil, in, lifecycleBoundary{}); !errors.Is(e, ErrReadiness) {
		t.Fatalf("unresolved usage/paid grants erased: %v", e)
	}
	if e = w.db.Exec(`UPDATE test_lifecycle SET pending_usage=false`).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = w.s.CompleteCustomerClosure(w.ctx, nil, in, lifecycleBoundary{}); !errors.Is(e, ErrReadiness) {
		t.Fatal("paid remainder waived by clicking close")
	}
	in.WaiverEvidence = "explicit informed abandonment of remaining paid grant only"
	closed, e := w.s.CompleteCustomerClosure(w.ctx, nil, in, lifecycleBoundary{})
	if e != nil || closed.Status != "closed" {
		t.Fatalf("closure=%+v %v", closed, e)
	}
	if _, e = w.s.AuthenticateToken(w.ctx, nil, login.AccessToken); !errors.Is(e, ErrInvalidCredentials) {
		t.Fatalf("closed token usable: %v", e)
	}
	chain, e := w.s.ReferralChain(w.ctx, nil, child.Customer.ID)
	if e != nil || len(chain) != 1 || chain[0].InviterID != registered.Customer.ID {
		t.Fatalf("closure rewrote descendant chain: %+v %v", chain, e)
	}
	if _, e = w.s.BeginRegistration(w.ctx, nil, RegistrationIntentRequest{BrandID: w.platform.Brand.ID, Email: "lifecycle@example.test", Password: testPassword}); !errors.Is(e, ErrEmailAlreadyRegistered) {
		t.Fatalf("closure reset brand identity/new customer history: %v", e)
	}
}

func TestPostgresFreezeRespectsExactScopeActionAndReviewVersion(t *testing.T) {
	w := world(t)
	c, owner := w.child(t, w.platform, w.platformOwner, Channel, "Scope")
	a := w.register(t, w.platform.Brand.ID, "freeze-a@example.test", c.InviteCode)
	b := w.register(t, w.platform.Brand.ID, "freeze-b@example.test", c.InviteCode)
	due := time.Now().Add(time.Hour)
	in := ScopeFreezeInput{ActorStaffID: w.platformOwner, OperatorID: w.platform.Operator.ID, ScopeKind: "customer", ScopeID: a.Customer.ID, Action: "model_usage", Enabled: true, Evidence: "verified review case", Explanation: "review limited to model calls", ReviewDueAt: &due}
	if _, e := w.s.SetScopeFreeze(w.ctx, nil, in); !errors.Is(e, ErrForbidden) {
		t.Fatalf("supplier froze child customer without role: %v", e)
	}
	in.ActorStaffID = owner
	in.OperatorID = c.Operator.ID
	frozen, e := w.s.SetScopeFreeze(w.ctx, nil, in)
	if e != nil || frozen.Version != 1 {
		t.Fatalf("freeze=%+v %v", frozen, e)
	}
	if e = w.s.RequireCustomerAction(w.ctx, nil, a.Customer.ID, c.Operator.ID, "model_usage"); !errors.Is(e, ErrFrozen) {
		t.Fatalf("freeze not applied: %v", e)
	}
	for _, check := range []struct{ customer, action string }{{a.Customer.ID, "new_purchase"}, {a.Customer.ID, "withdrawal"}, {b.Customer.ID, "model_usage"}} {
		if e = w.s.RequireCustomerAction(w.ctx, nil, check.customer, c.Operator.ID, check.action); e != nil {
			t.Fatalf("freeze escaped exact scope/action: %v", e)
		}
	}
	in.Enabled = false
	if _, e = w.s.SetScopeFreeze(w.ctx, nil, in); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale review overwrote freeze: %v", e)
	}
	in.ExpectedVersion = 1
	if _, e = w.s.SetScopeFreeze(w.ctx, nil, in); e != nil {
		t.Fatal(e)
	}
	if e = w.s.RequireCustomerAction(w.ctx, nil, a.Customer.ID, c.Operator.ID, "model_usage"); e != nil {
		t.Fatal(e)
	}
	brandFreeze := ScopeFreezeInput{ActorStaffID: w.platformOwner, OperatorID: w.platform.Operator.ID, ScopeKind: "brand", ScopeID: w.platform.Brand.ID, Action: "all_new_business", Enabled: true, Evidence: "brand-wide verified incident", Explanation: "temporary stop of new business", ReviewDueAt: &due}
	if _, e = w.s.SetScopeFreeze(w.ctx, nil, brandFreeze); e != nil {
		t.Fatal(e)
	}
	if e = w.s.RequireBusiness(w.ctx, nil, c.Operator.ID, Registration); !errors.Is(e, ErrFrozen) {
		t.Fatalf("brand freeze missed child operating scope: %v", e)
	}
	if e = w.s.RequireBusiness(w.ctx, nil, c.Operator.ID, Settlement); e != nil {
		t.Fatal("business freeze erased settlement right", e)
	}
	brandFreeze.Action = "settlement"
	if _, e = w.s.SetScopeFreeze(w.ctx, nil, brandFreeze); !errors.Is(e, ErrInvalidInput) {
		t.Fatalf("blanket settlement freeze accepted: %v", e)
	}
}
