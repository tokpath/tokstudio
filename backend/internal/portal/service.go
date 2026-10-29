// Package portal owns HTTP presentation, browser sessions and application
// orchestration. Domain records are accessed only through domain services.
package portal

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"github.com/tokpath/tokstudio/backend/internal/platform/observability"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "portal", f }
func AllMigrations() []db.ModuleMigrations {
	out := []db.ModuleMigrations{}
	add := func(name string, f fs.FS) { out = append(out, db.ModuleMigrations{Module: name, FS: f}) }
	add(organization.Migrations())
	add(ledger.Migrations())
	add(branding.Migrations())
	add(jobs.Migrations())
	add(outbox.Migrations())
	add(mail.Migrations())
	add(catalog.CommerceMigrations())
	add(modelpricing.Migrations())
	add(cooperation.Migrations())
	add(commerce.Migrations())
	add(incentives.Migrations())
	add(payments.Migrations())
	add(usage.Migrations())
	add(operations.Migrations())
	add(remediation.Migrations())
	add(Migrations())
	return out
}

type Installation struct {
	ID, BrandID, OperatorID, OwnerStaffID string
	CreatedAt                             time.Time
}

func (Installation) TableName() string { return "portal_installation" }

type Staff struct {
	ID, Email, DisplayName, PasswordHash, Status string
	Version                                      int64
	EmailVerified                                bool
	MustChangePassword                           bool
	CreatedAt                                    time.Time
}

func (Staff) TableName() string { return "portal_staff" }

type Application struct {
	ModelPricing       *modelpricing.Service
	DomainVerifier     branding.DomainVerifier
	Metrics            *observability.Metrics
	Redis              *redis.Client
	ObjectStore        *objectstore.Store
	Config             Config
	DB                 *gorm.DB
	Organization       *organization.Service
	Branding           *branding.Service
	Ledger             *ledger.Service
	Commerce           *commerce.Service
	Cooperation        *cooperation.Service
	Incentives         *incentives.Service
	Catalog            catalog.CommerceCatalog
	CatalogChanges     *catalog.ChangeController
	Payments           *payments.Service
	Usage              *usage.Service
	Jobs               *jobs.Service
	Mail               *mail.Service
	Runtime            *modelruntime.Bifrost
	Remediation        *remediation.Service
	AutoReplenishments *cooperation.Replenisher
	ProcurementRefunds *cooperation.ProcurementRefunds
	Installation       Installation
	Operations         *operations.Service
	GoogleExchanger    GoogleExchanger
}

func New(ctx context.Context, c Config, gdb *gorm.DB) (*Application, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := db.Readiness(ctx, gdb, AllMigrations()); err != nil {
		return nil, err
	}
	return newApplication(ctx, c, gdb)
}

// newApplication binds services to an already validated connection. The explicit
// demo command uses the caller's transaction after New verified readiness.
func newApplication(ctx context.Context, c Config, gdb *gorm.DB) (*Application, error) {
	a := &Application{Config: c, DB: gdb, Metrics: observability.New()}
	if e := gdb.WithContext(ctx).First(&a.Installation, "id='platform'").Error; e != nil {
		return nil, errors.New("new system is not bootstrapped: run cmd/migrate with bootstrap credentials")
	}
	if c.RedisURL != "" {
		options, e := redis.ParseURL(c.RedisURL)
		if e != nil {
			return nil, e
		}
		a.Redis = redis.NewClient(options)
	}
	if c.S3Bucket != "" {
		store, e := NewObjectStore(ctx, c)
		if e != nil {
			return nil, e
		}
		a.ObjectStore = store
	}
	a.Organization = organization.New(gdb)
	a.Organization.SetMonetaryReadiness(func(ctx context.Context, tx *gorm.DB) error { _, e := ledger.RequireMonetary(ctx, tx); return e })
	if err := a.Organization.AdoptOfficialGoogleEnv(ctx, a.Installation.BrandID, c.AppKey, c.GoogleClientID, c.GoogleClientSecret, c.GoogleRedirect); err != nil {
		return nil, err
	}
	a.Ledger = ledger.New(gdb)
	a.Branding = branding.NewWithDirectory(gdb, a.Organization)
	a.Branding.SetAssetStore(a.ObjectStore)
	a.DomainVerifier = branding.DNSAndTLSVerifier{}
	if c.CloudflareToken != "" || c.CloudflareZone != "" {
		verifier, e := branding.NewCloudflareVerifier(branding.CloudflareSettings{APIToken: c.CloudflareToken, ZoneID: c.CloudflareZone, CNAME: c.CloudflareCNAME}, nil)
		if e != nil {
			return nil, e
		}
		a.DomainVerifier = verifier
	}
	a.ModelPricing = &modelpricing.Service{DB: gdb, AllowTestLoopback: c.TestLoopback}
	a.Catalog = catalog.CommerceCatalog{Costs: a.ModelPricing, DB: gdb, Operators: cooperation.CatalogDirectory{Organization: a.Organization}, PlatformOperatorID: a.Installation.OperatorID, EncryptionKey: c.AppKey}
	a.Cooperation = cooperation.New(gdb, a.Ledger, a.Organization)
	a.Cooperation.Rates = cooperation.CatalogRates{Catalog: a.Catalog, Organization: a.Organization}
	a.Commerce = commerce.New(gdb)
	a.Commerce.SetMonetaryReadiness(func(ctx context.Context, tx *gorm.DB) error { _, e := ledger.RequireMonetary(ctx, tx); return e })
	a.Commerce.SetSalesCoverage(a.Cooperation)
	a.Incentives = incentives.New(gdb)
	a.Incentives.SetMonetaryReadiness(func(ctx context.Context, tx *gorm.DB) error { _, e := ledger.RequireMonetary(ctx, tx); return e })
	a.Incentives.SetRegistrationCoverage(a.Cooperation)
	a.Commerce.SetMarketing(a.Incentives)
	a.Jobs = jobs.New(gdb)
	a.Mail = mail.NewService(gdb, c.AppKey, mail.SMTPOptions{Host: c.SMTPHost, Port: c.SMTPPort, Username: c.SMTPUsername, Password: c.SMTPPassword, From: c.SMTPFrom, Security: c.SMTPSecurity, Production: c.IsProduction()})
	a.Payments = payments.NewService(gdb, c.AppKey, a.Organization, payments.NewTransport(c.TestLoopback), portalReceiptSink{Application: a, CommerceSink: payments.CommerceSink{Commerce: a.Commerce}})
	runtime, err := modelruntime.NewBifrost(ctx, modelruntime.BifrostConfig{Costs: a.ModelPricing, AllowTestLoopback: c.TestLoopback, Providers: []string{"openai", "anthropic", "gemini", "openrouter", "elevenlabs"}, Credentials: a.Catalog, EndpointGuard: a.guardEndpoint, ResourceGuard: a.guardResource})
	if err != nil {
		return nil, err
	}
	a.Runtime = runtime
	a.Usage = usage.New(gdb, usage.Dependencies{Ledger: a.Ledger, Organization: a.Organization, Cooperation: a.Cooperation, Catalog: a.Catalog, Runtime: runtime, GrantObserver: a.Incentives, EncryptionKey: c.AppKey, ResultStore: a.ObjectStore, AdmissionLimiter: usage.RedisLimiter{Client: a.Redis}})
	a.Operations = &operations.Service{DB: gdb, EncryptionKey: c.AppKey, Authorize: a.Organization, Recipients: recipientDirectory{a}, Mail: a.Mail, Source: operations.DomainSource{Payments: a.Payments, Ledger: a.Ledger, Cooperation: a.Cooperation, Usage: a.Usage}, Results: a.Usage, InvoiceStore: a.ObjectStore}
	a.Operations.Restrictions = a.Organization
	a.Remediation = remediation.New(gdb, remediation.Dependencies{Ledger: a.Ledger, Organization: a.Organization, Commerce: a.Commerce, Cooperation: a.Cooperation, Incentives: a.Incentives, Payments: a.Payments})
	a.Commerce.SetReceiptGuard(a.Remediation)
	a.Organization.SetOperatorExitChecker(a.Remediation)
	a.Operations.Source = operations.DomainSource{Payments: a.Payments, Ledger: a.Ledger, Cooperation: a.Cooperation, Usage: a.Usage, Commerce: a.Commerce, Refunds: operations.RefundFinancialSourceFunc(func(ctx context.Context, tx *gorm.DB, op string, from, to time.Time) (operations.RefundFinancialFacts, error) {
		facts, e := a.Remediation.FinancialRefundFacts(ctx, tx, op, from, to)
		return operations.RefundFinancialFacts{PendingFees: facts.PendingFees, PendingExecutions: facts.PendingExecutions, OutstandingMoney: facts.OutstandingMoney, MismatchCount: facts.MismatchCount}, e
	})}
	a.ProcurementRefunds = &cooperation.ProcurementRefunds{Cooperation: a.Cooperation, Payments: a.Payments, Collector: a.Commerce, Capacity: a.Remediation, ObserveReceiptRefund: func(ctx context.Context, tx *gorm.DB, op, receipt, reference, factID string, amount ledger.Decimal) error {
		_, e := a.Remediation.ObserveReceiptRefund(ctx, tx, op, receipt, reference, factID, amount)
		return e
	}}
	a.Payments.RefundApproval = cooperation.CombinedRefundApproval{Procurement: a.ProcurementRefunds, Other: a.Remediation}
	a.Payments.RefundSink = cooperation.CombinedRefundSink{Procurement: a.ProcurementRefunds, Other: a.Remediation}
	a.Operations.Invoices = operations.CommerceInvoiceSource{Commerce: a.Commerce, Adjustments: a.Remediation}
	a.Operations.ProcurementInvoices = operations.DomainProcurementInvoiceSource{Cooperation: a.Cooperation, Commerce: a.Commerce, Adjustments: a.Remediation}
	a.AutoReplenishments = &cooperation.Replenisher{Cooperation: a.Cooperation, Payments: a.Payments, Collector: a.Commerce, EncryptionKey: c.AppKey, Observer: a.Operations}
	a.CatalogChanges = &catalog.ChangeController{Catalog: a.Catalog, Audience: a.Organization, Notices: a.Operations, ValidateCoverage: a.pricePublished, ValidateChange: a.validateCatalogChange}
	return a, nil
}
func (a *Application) Close() {
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.Runtime != nil {
		a.Runtime.Close()
	}
}
func Bootstrap(ctx context.Context, c Config, gdb *gorm.DB) (Installation, error) {
	var installed Installation
	err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('portal.bootstrap',0))").Error; e != nil {
			return e
		}
		e := tx.First(&installed, "id='platform'").Error
		if e == nil {
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		email := strings.ToLower(strings.TrimSpace(c.BootstrapEmail))
		if !mail.ValidAddress(email) {
			return errors.New("initial TOKENHUB_BOOTSTRAP_STAFF_EMAIL is required")
		}
		hash, e := organization.HashPassword(c.BootstrapPassword)
		if e != nil {
			return e
		}
		if c.BrandName == "" || c.OperatorName == "" || c.LegalName == "" {
			return errors.New("initial brand, operator and legal names must be explicitly configured")
		}
		staff := Staff{ID: uuid.NewString(), Email: email, DisplayName: c.BootstrapName, PasswordHash: hash, Status: "active", Version: 1, EmailVerified: true, CreatedAt: time.Now().UTC()}
		if staff.DisplayName == "" {
			staff.DisplayName = email
		}
		if e = tx.Create(&staff).Error; e != nil {
			return e
		}
		created, e := organization.New(gdb).BootstrapPlatform(ctx, tx, organization.BootstrapInput{BrandName: c.BrandName, OperatorName: c.OperatorName, LegalName: c.LegalName, OwnerStaffID: staff.ID})
		if e != nil {
			return e
		}
		installed = Installation{ID: "platform", BrandID: created.Brand.ID, OperatorID: created.Operator.ID, OwnerStaffID: staff.ID, CreatedAt: time.Now().UTC()}
		return tx.Create(&installed).Error
	})
	return installed, err
}
