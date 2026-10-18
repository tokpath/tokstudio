package organization

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testPassword = "test-only-correct-password"

func TestNormalizeEmailAndRelationship(t *testing.T) {
	got, err := NormalizeEmail("  Alice+tag@EXAMPLE.COM ")
	if err != nil || got != "alice+tag@example.com" {
		t.Fatalf("email=%q err=%v", got, err)
	}
	for _, v := range []string{"Alice <alice@example.com>", "bad", "x\n@example.com"} {
		if _, err := NormalizeEmail(v); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted %q", v)
		}
	}
	for _, p := range []OperatorKind{Platform, OEM, Channel} {
		for _, c := range []OperatorKind{Platform, OEM, Channel} {
			valid := p == Platform && (c == OEM || c == Channel) || p == OEM && c == Channel
			if (ValidateRelationship(Operator{Kind: p}, c) == nil) != valid {
				t.Fatalf("relationship %s -> %s", p, c)
			}
		}
	}
	if _, err := HashPassword("short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("short password accepted")
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("bcrypt truncation allowed")
	}
}

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL not set; real PostgreSQL integration was not run")
	}
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	baseSQL, err := base.DB()
	if err != nil {
		t.Fatal(err)
	}
	schema := "organization_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(12)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; err != nil {
			t.Error(err)
		}
		_ = baseSQL.Close()
	})
	_, migrations := Migrations()
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(migrations, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(body)).Error }); err != nil {
			t.Fatalf("migration %s: %v", entry.Name(), err)
		}
	}
	return db
}

type testWorld struct {
	db            *gorm.DB
	s             *Service
	ctx           context.Context
	platform      OperatorCreated
	platformOwner string
}

func world(t *testing.T) testWorld {
	t.Helper()
	db := integrationDB(t)
	s := New(db)
	ctx := context.Background()
	owner := uuid.NewString()
	p, err := s.BootstrapPlatform(ctx, nil, BootstrapInput{BrandName: "Official", OperatorName: "Platform", OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	openRegistration(t, s, p.Operator.ID, owner)
	return testWorld{db: db, s: s, ctx: ctx, platform: p, platformOwner: owner}
}
func openRegistration(t *testing.T, s *Service, operatorID, owner string) {
	t.Helper()
	st, err := s.GetBusinessStatus(context.Background(), nil, operatorID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetBusinessStatus(context.Background(), nil, SetBusinessStatusInput{ActorStaffID: owner, OperatorID: operatorID, Feature: Registration, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "test-opening"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
func (w testWorld) child(t *testing.T, parent OperatorCreated, parentOwner string, kind OperatorKind, name string) (OperatorCreated, string) {
	t.Helper()
	c, err := w.s.CreateOperator(w.ctx, nil, CreateOperatorInput{ActorStaffID: parentOwner, ParentOperatorID: parent.Operator.ID, Kind: kind, Name: name, BrandName: name + " brand"})
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	email := owner + "@example.test"
	inv, err := w.s.InviteStaff(w.ctx, nil, parentOwner, c.Operator.ID, email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.AcceptStaffInvitation(w.ctx, nil, inv.Token, VerifiedStaff{ID: owner, Email: email, EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	openRegistration(t, w.s, c.Operator.ID, owner)
	return c, owner
}
func (w testWorld) challenge(t *testing.T, brand, email, code string) VerificationChallenge {
	t.Helper()
	v, err := w.s.BeginRegistration(w.ctx, nil, RegistrationIntentRequest{BrandID: brand, Email: email, InviteCode: code, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.VerifyRegistration(w.ctx, nil, v.Intent.ID, v.VerificationToken); err != nil {
		t.Fatal(err)
	}
	return v
}
func (w testWorld) register(t *testing.T, brand, email, code string) RegistrationResult {
	t.Helper()
	v := w.challenge(t, brand, email, code)
	r, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPostgresFourChainsAndPermissionScope(t *testing.T) {
	w := world(t)
	o, oOwner := w.child(t, w.platform, w.platformOwner, OEM, "OEM")
	official, cOwner := w.child(t, w.platform, w.platformOwner, Channel, "Official channel")
	oc, ocOwner := w.child(t, o, oOwner, Channel, "OEM channel")
	if o.Brand.ID == w.platform.Brand.ID || official.Brand.ID != w.platform.Brand.ID || oc.Brand.ID != o.Brand.ID {
		t.Fatal("brand boundary lost")
	}
	if _, err := w.s.CreateOperator(w.ctx, nil, CreateOperatorInput{ActorStaffID: cOwner, ParentOperatorID: official.Operator.ID, Kind: Channel, Name: "illegal"}); !errors.Is(err, ErrInvalidRelationship) {
		t.Fatalf("channel descendants: %v", err)
	}
	if err := w.s.RequirePermission(w.ctx, nil, w.platformOwner, oc.Operator.ID, "ledger.read"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("supplier financial access: %v", err)
	}
	c := w.register(t, o.Brand.ID, "retail@example.test", oc.InviteCode)
	if _, err := w.s.CustomerInScope(w.ctx, nil, oOwner, o.Operator.ID, c.Customer.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("upstream customer detail: %v", err)
	}
	if _, err := w.s.CustomerInScope(w.ctx, nil, ocOwner, oc.Operator.ID, c.Customer.ID); err != nil {
		t.Fatal(err)
	}
	// Database rejects a supply edge which does not match the fixed parent.
	err := w.db.Exec(`UPDATE organization_supply_agreements SET supplier_id=? WHERE buyer_id=?`, w.platform.Operator.ID, oc.Operator.ID).Error
	if err == nil {
		t.Fatal("database accepted illegal supplier")
	}
	if _, err = w.s.BootstrapPlatform(w.ctx, nil, BootstrapInput{BrandName: "Duplicate", OperatorName: "Duplicate", OwnerStaffID: uuid.NewString()}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second platform: %v", err)
	}
}

func TestPostgresConcurrentEmailRegistrationAndReferralBoundaries(t *testing.T) {
	w := world(t)
	c1, _ := w.child(t, w.platform, w.platformOwner, Channel, "A")
	c2, _ := w.child(t, w.platform, w.platformOwner, Channel, "B")
	o, _ := w.child(t, w.platform, w.platformOwner, OEM, "OEM")
	a := w.challenge(t, w.platform.Brand.ID, "RACE@example.test", c1.InviteCode)
	b := w.challenge(t, w.platform.Brand.ID, "race@example.test", c2.InviteCode)
	var successes, hooks atomic.Int64
	errs := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, v := range []VerificationChallenge{a, b} {
		wg.Add(1)
		go func(v VerificationChallenge) {
			defer wg.Done()
			<-start
			_, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}, RegistrationHookFunc(func(context.Context, *gorm.DB, RegistrationResult) error { hooks.Add(1); return nil }))
			if err == nil {
				successes.Add(1)
			}
			errs <- err
		}(v)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrEmailAlreadyRegistered) {
			t.Fatalf("unexpected registration error: %v", err)
		}
	}
	if successes.Load() != 1 || hooks.Load() != 1 {
		t.Fatalf("success=%d rewards=%d", successes.Load(), hooks.Load())
	}
	other := w.register(t, o.Brand.ID, "race@example.test", "")
	if other.Customer.BrandID != o.Brand.ID {
		t.Fatal("cross-brand email registration failed")
	}
	if _, err := w.s.ResolveRegistrationIntent(w.ctx, nil, o.Brand.ID, c1.InviteCode); !errors.Is(err, ErrInvalidInviteCode) {
		t.Fatalf("foreign brand code: %v", err)
	}
	if _, err := w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, "MISSING"); !errors.Is(err, ErrInvalidInviteCode) {
		t.Fatalf("invalid code silently fallback: %v", err)
	}
	one := w.register(t, w.platform.Brand.ID, "one@example.test", c1.InviteCode)
	two := w.register(t, w.platform.Brand.ID, "two@example.test", one.InviteCode)
	three := w.register(t, w.platform.Brand.ID, "three@example.test", two.InviteCode)
	chain, err := w.s.ReferralChain(w.ctx, nil, three.Customer.ID)
	if err != nil || len(chain) != 2 || chain[0].InviterID != two.Customer.ID || chain[1].InviterID != one.Customer.ID {
		t.Fatalf("chain=%+v err=%v", chain, err)
	}
	outsider := w.register(t, w.platform.Brand.ID, "outsider@example.test", c2.InviteCode)
	err = w.db.Exec(`INSERT INTO organization_referral_edges(invitee_id,inviter_id,brand_id,registration_operator_id,created_at) VALUES(?,?,?,?,now())`, outsider.Customer.ID, one.Customer.ID, w.platform.Brand.ID, c2.Operator.ID).Error
	if err == nil {
		t.Fatal("database accepted cross-operator promotion")
	}
	err = w.db.Exec(`UPDATE organization_referral_edges SET inviter_id=? WHERE invitee_id=?`, three.Customer.ID, two.Customer.ID).Error
	if err == nil {
		t.Fatal("database rewrote invitation edge")
	}
}

func TestPostgresRegistrationProofAtomicHooksAndAccountRecovery(t *testing.T) {
	w := world(t)
	v := w.challenge(t, w.platform.Brand.ID, "first@example.test", "")
	if _, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: "forged"}); !errors.Is(err, ErrInvalidVerification) {
		t.Fatalf("forged verification: %v", err)
	}
	failure := errors.New("reward obligation unavailable")
	_, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}, RegistrationHookFunc(func(context.Context, *gorm.DB, RegistrationResult) error { return failure }))
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var count int64
	if err = w.db.Model(&customerRow{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial registration persisted: %d %v", count, err)
	}
	var hookCount int
	r, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}, RegistrationHookFunc(func(context.Context, *gorm.DB, RegistrationResult) error { hookCount++; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}, RegistrationHookFunc(func(context.Context, *gorm.DB, RegistrationResult) error { hookCount++; return nil }))
	if err != nil || replay.Customer.ID != r.Customer.ID || replay.NewlyRegistered || hookCount != 1 {
		t.Fatalf("idempotency: %+v %v calls=%d", replay, err, hookCount)
	}
	var auth credentialRow
	if err = w.db.First(&auth, "customer_id=?", r.Customer.ID).Error; err != nil || auth.PasswordHash == testPassword || !CheckPassword(auth.PasswordHash, testPassword) {
		t.Fatalf("password digest: %v", err)
	}
	var stored intentRow
	if err = w.db.First(&stored, "id=?", v.Intent.ID).Error; err != nil || stored.VerificationHash == v.VerificationToken || stored.PasswordHash != "" {
		t.Fatalf("intent secret handling: %v", err)
	}
	login, err := w.s.Login(w.ctx, nil, w.platform.Brand.ID, "first@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	var tok accessTokenRow
	if err = w.db.First(&tok, "customer_id=?", r.Customer.ID).Error; err != nil || tok.TokenHash == login.AccessToken || tok.TokenHash != tokenHash(login.AccessToken) {
		t.Fatalf("access token digest: %v", err)
	}
	reset, err := w.s.BeginPasswordReset(w.ctx, nil, w.platform.Brand.ID, "first@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.VerifyEmail(w.ctx, nil, reset.Intent.ID, reset.VerificationToken); err != nil {
		t.Fatal(err)
	}
	if err = w.s.CompletePasswordReset(w.ctx, nil, reset.Intent.ID, reset.VerificationToken, "replacement-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.AuthenticateToken(w.ctx, nil, login.AccessToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("reset kept token valid: %v", err)
	}
	if _, err = w.s.Login(w.ctx, nil, w.platform.Brand.ID, "first@example.test", testPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password valid: %v", err)
	}
	change, err := w.s.BeginEmailChange(w.ctx, nil, r.Customer.ID, "replacement-password", "second@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.VerifyEmail(w.ctx, nil, change.Intent.ID, change.VerificationToken); err != nil {
		t.Fatal(err)
	}
	if err = w.s.CompleteEmailChange(w.ctx, nil, change.Intent.ID, change.VerificationToken); err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.Login(w.ctx, nil, w.platform.Brand.ID, "second@example.test", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.BeginRegistration(w.ctx, nil, RegistrationIntentRequest{BrandID: w.platform.Brand.ID, Email: "first@example.test", Password: testPassword}); !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("old identity reset new-customer eligibility: %v", err)
	}
	if own, err := w.s.CurrentOwnership(w.ctx, nil, r.Customer.ID); err != nil || own.ID != r.Ownership.ID {
		t.Fatalf("email change rewrote ownership: %+v %v", own, err)
	}
	if err = w.db.Exec(`DELETE FROM organization_customer_identities WHERE customer_id=?`, r.Customer.ID).Error; err == nil {
		t.Fatal("database allowed history deletion")
	}
}

func TestPostgresStaffOpeningAndIndependentBusinessStatuses(t *testing.T) {
	w := world(t)
	c, owner := w.child(t, w.platform, w.platformOwner, Channel, "Channel")
	staffID := uuid.NewString()
	inv, err := w.s.InviteStaff(w.ctx, nil, owner, c.Operator.ID, "finance@example.test", "finance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.AcceptStaffInvitation(w.ctx, nil, inv.Token, VerifiedStaff{ID: staffID, Email: "wrong@example.test", EmailVerified: true}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong email accepted invite: %v", err)
	}
	if _, err = w.s.AcceptStaffInvitation(w.ctx, nil, inv.Token, VerifiedStaff{ID: staffID, Email: "finance@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	if err = w.s.RequirePermission(w.ctx, nil, staffID, c.Operator.ID, "payments.manage"); err != nil {
		t.Fatal(err)
	}
	if err = w.s.RequirePermission(w.ctx, nil, staffID, w.platform.Operator.ID, "payments.manage"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross space finance: %v", err)
	}
	if err = w.s.SetMembership(w.ctx, nil, SetMembershipInput{ActorStaffID: owner, OperatorID: c.Operator.ID, StaffID: owner, Role: "readonly", Status: "active"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("removed last owner: %v", err)
	}
	if err = w.s.SetMembership(w.ctx, nil, SetMembershipInput{ActorStaffID: owner, OperatorID: c.Operator.ID, StaffID: staffID, Role: "custom", Status: "active", Grants: []string{"staff.manage"}}); err != nil {
		t.Fatal(err)
	}
	if err = w.s.SetMembership(w.ctx, nil, SetMembershipInput{ActorStaffID: staffID, OperatorID: c.Operator.ID, StaffID: staffID, Role: "owner", Status: "active"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("custom staff escalated to owner: %v", err)
	}
	st, err := w.s.GetBusinessStatus(w.ctx, nil, c.Operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.s.SetBusinessStatus(w.ctx, nil, SetBusinessStatusInput{ActorStaffID: owner, OperatorID: c.Operator.ID, Feature: NewPurchase, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "opening"}, nil)
	if !errors.Is(err, ErrReadiness) {
		t.Fatalf("feature enabled without dependencies: %v", err)
	}
	st, err = w.s.SetBusinessStatus(w.ctx, nil, SetBusinessStatusInput{ActorStaffID: owner, OperatorID: c.Operator.ID, Feature: Registration, Enabled: false, ExpectedVersion: st.Version, EvidenceReference: "pause-registration"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.s.RequireBusiness(w.ctx, nil, c.Operator.ID, Registration); !errors.Is(err, ErrBusinessClosed) {
		t.Fatalf("registration still open: %v", err)
	}
	if err = w.s.RequireBusiness(w.ctx, nil, c.Operator.ID, Settlement); err != nil {
		t.Fatalf("pause stopped existing settlement: %v", err)
	}
	if !st.Settlement || st.NewPurchase {
		t.Fatal("independent status flags lost")
	}
}

func TestPostgresTakeoverIsCompleteAtomicAndVersionsTokens(t *testing.T) {
	w := world(t)
	c, _ := w.child(t, w.platform, w.platformOwner, Channel, "Channel")
	a := w.register(t, w.platform.Brand.ID, "a@example.test", c.InviteCode)
	b := w.register(t, w.platform.Brand.ID, "b@example.test", a.InviteCode)
	login, err := w.s.Login(w.ctx, nil, w.platform.Brand.ID, "a@example.test", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	in := TakeoverOwnershipInput{ChannelID: c.Operator.ID, BrandOwnerOperatorID: w.platform.Operator.ID, EvidenceReference: "approved-takeover", ExpectedVersions: map[string]int64{a.Customer.ID: 1, b.Customer.ID: 1}}
	if _, err = w.s.ApplyChannelTakeoverOwnership(w.ctx, nil, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("standalone transfer allowed: %v", err)
	}
	rollback := errors.New("financial transfer failed")
	err = w.db.Transaction(func(tx *gorm.DB) error {
		if _, err := w.s.ApplyChannelTakeoverOwnership(w.ctx, tx, in); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = w.s.AssertOwnershipVersion(w.ctx, nil, a.Customer.ID, c.Operator.ID, 1); err != nil {
		t.Fatalf("outer rollback did not restore ownership: %v", err)
	}
	partial := in
	partial.ExpectedVersions = map[string]int64{a.Customer.ID: 1}
	err = w.db.Transaction(func(tx *gorm.DB) error { _, err := w.s.ApplyChannelTakeoverOwnership(w.ctx, tx, partial); return err })
	if !errors.Is(err, ErrStaleOwnership) {
		t.Fatalf("partial takeover accepted: %v", err)
	}
	err = w.db.Transaction(func(tx *gorm.DB) error { _, err := w.s.ApplyChannelTakeoverOwnership(w.ctx, tx, in); return err })
	if err != nil {
		t.Fatal(err)
	}
	principal, err := w.s.AuthenticateToken(w.ctx, nil, login.AccessToken)
	if err != nil || principal.Ownership.OperatorID != w.platform.Operator.ID || principal.Ownership.Version != 2 {
		t.Fatalf("token has stale ownership: %+v %v", principal, err)
	}
	if err = w.s.AssertOwnershipVersion(w.ctx, nil, a.Customer.ID, c.Operator.ID, 1); !errors.Is(err, ErrStaleOwnership) {
		t.Fatalf("old version still writable: %v", err)
	}
	chain, err := w.s.ReferralChain(w.ctx, nil, b.Customer.ID)
	if err != nil || len(chain) != 1 || chain[0].InviterID != a.Customer.ID || chain[0].RegistrationOperatorID != c.Operator.ID {
		t.Fatalf("takeover rewrote referral: %+v %v", chain, err)
	}
	attribution, err := w.s.ResolveRegistrationIntent(w.ctx, nil, w.platform.Brand.ID, a.InviteCode)
	if err != nil || attribution.Operator.ID != w.platform.Operator.ID {
		t.Fatalf("user code stale after takeover: %+v %v", attribution, err)
	}
}

func TestPostgresVerificationExpiry(t *testing.T) {
	w := world(t)
	v, err := w.s.BeginRegistration(w.ctx, nil, RegistrationIntentRequest{BrandID: w.platform.Brand.ID, Email: "expiry@example.test", Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.db.Model(&intentRow{}).Where("id=?", v.Intent.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = w.s.VerifyRegistration(w.ctx, nil, v.Intent.ID, v.VerificationToken); !errors.Is(err, ErrInvalidVerification) {
		t.Fatalf("expired email token: %v", err)
	}
	if _, err = w.s.CompleteRegistration(w.ctx, nil, CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}); !errors.Is(err, ErrInvalidVerification) {
		t.Fatalf("expired registration: %v", err)
	}
}
