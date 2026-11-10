package branding

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ptr(s string) *string { return &s }

func TestDomainNormalization(t *testing.T) {
	for in, want := range map[string]string{"EXAMPLE.com.": "example.com", "bücher.example": "xn--bcher-kva.example", "  example.com  ": "example.com"} {
		got, err := NormalizeDomain(in)
		if err != nil || got != want {
			t.Fatalf("%q => %q %v", in, got, err)
		}
	}
	for _, v := range []string{"https://example.com", "*.example.com", "127.0.0.1", "example.com:443", "example.com/path", "a..example", "localhost", "-bad.example", "example.com@evil.test"} {
		if _, err := NormalizeDomain(v); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted %q", v)
		}
	}
	if got, err := NormalizeHost("EXAMPLE.com:443"); err != nil || got != "example.com" {
		t.Fatalf("Host header=%q %v", got, err)
	}
}
func TestThemeInheritanceIsPerField(t *testing.T) {
	base := InheritTheme(DefaultTheme(), ThemeOverrides{PrimaryColor: ptr("#123456"), LogoURL: ptr("https://assets.example/brand.png")})
	got := InheritTheme(base, ThemeOverrides{BackgroundColor: ptr("#112233"), LogoURL: ptr("")})
	if got.PrimaryColor != "#123456" || got.BackgroundColor != "#112233" || got.LogoURL != "" {
		t.Fatalf("theme=%+v", got)
	}
	for _, v := range []ThemeOverrides{{PrimaryColor: ptr("red")}, {LogoURL: ptr("javascript:alert(1)")}, {LogoURL: ptr("http://example.com")}} {
		if err := ValidateTheme(v); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted unsafe theme %+v", v)
		}
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
	schema := "branding_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; err != nil {
			t.Error(err)
		}
		_ = baseSQL.Close()
	})
	_, o := organization.Migrations()
	_, b := Migrations()
	for _, m := range []fs.FS{o, b} {
		entries, err := fs.ReadDir(m, ".")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			body, err := fs.ReadFile(m, entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(body)).Error }); err != nil {
				t.Fatalf("migration %s: %v", entry.Name(), err)
			}
		}
	}
	return db
}

type fixtures struct {
	ctx      context.Context
	db       *gorm.DB
	org      *organization.Service
	s        *Service
	platform organization.OperatorCreated
	owner    string
}

func setup(t *testing.T) fixtures {
	t.Helper()
	db := integrationDB(t)
	org := organization.New(db)
	ctx := context.Background()
	owner := uuid.NewString()
	p, err := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Official", OperatorName: "Platform", OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	return fixtures{ctx: ctx, db: db, org: org, s: New(db), platform: p, owner: owner}
}
func (f fixtures) child(t *testing.T, parent organization.OperatorCreated, parentOwner string, kind organization.OperatorKind) (organization.OperatorCreated, string) {
	t.Helper()
	c, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: parentOwner, ParentOperatorID: parent.Operator.ID, Kind: kind, Name: "Child", BrandName: "Brand"})
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	email := owner + "@example.test"
	inv, err := f.org.InviteStaff(f.ctx, nil, parentOwner, c.Operator.ID, email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: email, EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	return c, owner
}

func TestPostgresDomainRequiresOwnershipAndTLSAndUniqueBinding(t *testing.T) {
	f := setup(t)
	o, oOwner := f.child(t, f.platform, f.owner, organization.OEM)
	d, err := f.s.RequestDomain(f.ctx, nil, f.owner, f.platform.Brand.ID, "Example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ResolveHost(f.ctx, nil, "example.test"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pending domain resolved: %v", err)
	}
	if _, err = f.s.RequestDomain(f.ctx, nil, oOwner, o.Brand.ID, "EXAMPLE.test."); !errors.Is(err, ErrDomainBound) {
		t.Fatalf("domain bound to two brands: %v", err)
	}
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, d.Domain.ID, true, 1); !errors.Is(err, ErrDomainUnverified) {
		t.Fatalf("unverified domain enabled: %v", err)
	}
	if _, err = f.s.VerifyDomain(f.ctx, f.owner, d.Domain.ID, nil); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("missing verifier silently succeeded: %v", err)
	}
	// A deterministic provider double tests persistence. This is deliberately
	// not an assertion of real public DNS, certificate or Cloudflare readiness.
	verified, err := f.s.VerifyDomain(f.ctx, f.owner, d.Domain.ID, DomainVerifierFunc(func(_ context.Context, host, challenge string) (DomainVerification, error) {
		if host != "example.test" || challenge != d.Domain.VerificationToken {
			t.Fatal("wrong verification target")
		}
		return DomainVerification{OwnershipVerified: true, ProviderReference: "test-provider:ownership"}, nil
	}))
	if err != nil || verified.Status != "verified" {
		t.Fatalf("verification=%+v %v", verified, err)
	}
	if _, err = f.s.ResolveHost(f.ctx, nil, "example.test"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("TLS pending domain resolved: %v", err)
	}
	active, err := f.s.VerifyDomain(f.ctx, f.owner, d.Domain.ID, DomainVerifierFunc(func(context.Context, string, string) (DomainVerification, error) {
		return DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "test-provider:tls-ready"}, nil
	}))
	if err != nil || active.Status != "verified" {
		t.Fatalf("verification activated domain: %+v %v", active, err)
	}
	if _, err = f.s.ResolveHost(f.ctx, nil, active.Hostname); !errors.Is(err, ErrNotFound) {
		t.Fatal("unapproved domain resolved", err)
	}
	active, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, active.ID, true, active.Version)
	if err != nil || active.Status != "active" {
		t.Fatalf("activation=%+v %v", active, err)
	}
	b, err := f.s.ResolveHost(f.ctx, nil, "EXAMPLE.test:443")
	if err != nil || b.ID != f.platform.Brand.ID {
		t.Fatalf("host=%+v %v", b, err)
	}
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, oOwner, d.Domain.ID, false, active.Version); !errors.Is(err, organization.ErrForbidden) {
		t.Fatalf("other OEM changed domain: %v", err)
	}
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, d.Domain.ID, false, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale domain version accepted: %v", err)
	}
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, d.Domain.ID, false, active.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ResolveHost(f.ctx, nil, "example.test"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled domain resolved: %v", err)
	}
}

func TestPostgresThemeScopeInheritanceWithoutPerSaveHistory(t *testing.T) {
	f := setup(t)
	official, officialOwner := f.child(t, f.platform, f.owner, organization.Channel)
	o, oOwner := f.child(t, f.platform, f.owner, organization.OEM)
	oc, ocOwner := f.child(t, o, oOwner, organization.Channel)
	brand, err := f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: f.platform.Brand.ID, Values: ThemeOverrides{PrimaryColor: ptr("#123456"), BackgroundColor: ptr("#abcdef"), LogoURL: ptr("https://assets.example/brand.png")}})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: officialOwner, TargetID: official.Operator.ID, Values: ThemeOverrides{PrimaryColor: ptr("#654321"), LogoURL: ptr("")}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := f.s.ResolveTheme(f.ctx, nil, f.platform.Brand.ID, official.Operator.ID)
	if err != nil || resolved.Theme.PrimaryColor != "#654321" || resolved.Theme.BackgroundColor != "#abcdef" || resolved.Theme.LogoURL != "" || resolved.BrandVersion != 1 || resolved.ChannelVersion != 1 {
		t.Fatalf("resolved=%+v %v", resolved, err)
	}
	_, err = f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: f.platform.Brand.ID, ExpectedVersion: brand.Version, Values: ThemeOverrides{PrimaryColor: ptr("#111111"), BackgroundColor: ptr("#222222"), LogoURL: ptr("https://assets.example/new.png")}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = f.s.ResolveTheme(f.ctx, nil, f.platform.Brand.ID, official.Operator.ID)
	if err != nil || resolved.Theme.PrimaryColor != "#654321" || resolved.Theme.BackgroundColor != "#222222" || resolved.Theme.LogoURL != "" {
		t.Fatalf("inherited update=%+v %v", resolved, err)
	}
	_, err = f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: officialOwner, TargetID: official.Operator.ID, ExpectedVersion: channel.Version, Values: ThemeOverrides{}})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = f.s.ResolveTheme(f.ctx, nil, f.platform.Brand.ID, official.Operator.ID)
	if err != nil || resolved.Theme.PrimaryColor != "#111111" || resolved.Theme.LogoURL != "https://assets.example/new.png" {
		t.Fatalf("reset to inherited=%+v %v", resolved, err)
	}
	_, err = f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: oc.Operator.ID, Values: ThemeOverrides{PrimaryColor: ptr("#ffffff")}})
	if !errors.Is(err, organization.ErrForbidden) {
		t.Fatalf("platform bypassed direct OEM parent: %v", err)
	}
	if _, err = f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: ocOwner, TargetID: oc.Operator.ID, Values: ThemeOverrides{PrimaryColor: ptr("#ffffff")}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ResolveTheme(f.ctx, nil, f.platform.Brand.ID, oc.Operator.ID); !errors.Is(err, organization.ErrForbidden) {
		t.Fatalf("cross brand theme context: %v", err)
	}
	var count int64
	if err = f.db.Table("branding_changes").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("unexpected per-save appearance history count=%d err=%v", count, err)
	}
	if _, err = f.s.SetBrandTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: f.platform.Brand.ID, ExpectedVersion: 1, Values: ThemeOverrides{}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale theme update: %v", err)
	}
}

func TestPostgresBackgroundDomainChecksNeverApproveAndRoutingIsRequired(t *testing.T) {
	f := setup(t)
	o, owner := f.child(t, f.platform, f.owner, organization.OEM)
	challenge, e := f.s.RequestDomain(f.ctx, nil, owner, o.Brand.ID, "pending.example.test")
	if e != nil {
		t.Fatal(e)
	}
	ready := DomainVerifierFunc(func(context.Context, string, string) (DomainVerification, error) {
		return DomainVerification{OwnershipVerified: true, TLSReady: true, ProviderReference: "isolated-cert"}, nil
	})
	if e = f.s.CheckPendingDomain(f.ctx, challenge.Domain.ID, ready); e != nil {
		t.Fatal(e)
	}
	domain, e := f.s.GetDomain(f.ctx, nil, owner, challenge.Domain.ID)
	if e != nil {
		t.Fatal(e)
	}
	if domain.Status != "verified" || domain.RouteReady {
		t.Fatal("background check approved domain", domain)
	}
	if _, e = f.s.SetDomainEnabled(f.ctx, nil, f.owner, domain.ID, true, domain.Version); !errors.Is(e, ErrDomainUnverified) {
		t.Fatal("TLS certificate alone accepted", e)
	}
	ready = DomainVerifierFunc(func(context.Context, string, string) (DomainVerification, error) {
		return DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "isolated-cert-route"}, nil
	})
	if e = f.s.CheckPendingDomain(f.ctx, domain.ID, ready); e != nil {
		t.Fatal(e)
	}
	domain, e = f.s.GetDomain(f.ctx, nil, owner, domain.ID)
	if e != nil || domain.Status != "verified" {
		t.Fatal(domain, e)
	}
	if _, e = f.s.ResolveHost(f.ctx, nil, domain.Hostname); !errors.Is(e, ErrNotFound) {
		t.Fatal("automatic approval", e)
	}
	now := f.s.now()
	f.s.now = func() time.Time { return now.Add(11 * time.Minute) }
	if _, e = f.s.SetDomainEnabled(f.ctx, nil, f.owner, domain.ID, true, domain.Version); !errors.Is(e, ErrDomainUnverified) {
		t.Fatal("stale evidence accepted", e)
	}
}
