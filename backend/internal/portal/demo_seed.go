package portal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Deliberately not exposed by Router, migrations, or normal startup.
// The one-time transaction preserves a receipt/order/ledger trail for synthetic
// assets and never executes an external payment or sends an invitation email.
type DemoOptions struct{ Confirmation, Host, ModelBaseURL, ModelKey string }
type DemoAccount struct{ Email, Kind, Role, OperatorID string }
type DemoResult struct {
	Accounts      []DemoAccount `json:"accounts"`
	ChannelInvite string        `json:"channel_invite"`
	ModelID       string        `json:"model_id,omitempty"`
	Note          string        `json:"note"`
}

func ValidateDemoTarget(c Config, in DemoOptions) error {
	if in.Confirmation != "TEST_DATA_ONLY" || in.Host != c.OfficialHost ||
		(in.Host != "test.tokpath.com" && (c.IsProduction() || in.Host != "localhost")) {
		return errors.New("explicit TEST_DATA_ONLY and the exact designated test hostname are required")
	}
	if in.ModelBaseURL != "" {
		u, err := url.Parse(in.ModelBaseURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(c.TestLoopback && u.Scheme == "http")) || len(in.ModelKey) < 24 {
			return errors.New("test model requires an explicit HTTPS endpoint and a separate bearer key")
		}
	}
	return nil
}
func (a *Application) SeedDemo(ctx context.Context, in DemoOptions) (out DemoResult, err error) {
	if err = ValidateDemoTarget(a.Config, in); err != nil {
		return
	}
	err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended('tokenhub-demo-v1',0))").Error; err != nil {
			return err
		}
		// This fixture-only table is created only by the explicitly invoked command.
		// A committed marker makes retries read-only; partial runs roll back entirely.
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS portal_demo_fixture_runs (id text PRIMARY KEY, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now())").Error; err != nil {
			return err
		}
		var previous []byte
		row := tx.Raw("SELECT result FROM portal_demo_fixture_runs WHERE id='demo-v1'").Row()
		if e := row.Scan(&previous); e == nil {
			return json.Unmarshal(previous, &out)
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		seeded, e := newApplication(ctx, a.Config, tx)
		if e != nil {
			return e
		}
		defer seeded.Close()
		builder := demoBuilder{a: seeded, tx: tx, ctx: ctx, out: &out}
		hash, e := bcrypt.GenerateFromPassword([]byte("password1"), 12)
		if e != nil {
			return e
		}
		builder.hash = string(hash)
		if e = builder.run(in); e != nil {
			return e
		}
		raw, e := json.Marshal(out)
		if e != nil {
			return e
		}
		return tx.Exec("INSERT INTO portal_demo_fixture_runs(id,result) VALUES('demo-v1',?::jsonb)", string(raw)).Error
	})
	return
}

type demoBuilder struct {
	a    *Application
	tx   *gorm.DB
	ctx  context.Context
	hash string
	out  *DemoResult
}

const demoEvidence = "TEST DATA ONLY: synthetic acceptance fixture, no real funds transferred"

func (s *demoBuilder) staff(inviter, operator, email, name, role string) (Staff, error) {
	p := Staff{ID: uuid.NewString(), Email: email, DisplayName: name, PasswordHash: s.hash, Status: "active", EmailVerified: true, Version: 1, CreatedAt: time.Now().UTC()}
	var count int64
	if e := s.tx.Model(&Staff{}).Where("email=?", email).Count(&count).Error; e != nil {
		return p, e
	}
	if count > 0 {
		return p, fmt.Errorf("reserved test account already exists: %s", email)
	}
	if e := s.tx.Create(&p).Error; e != nil {
		return p, e
	}
	invite, e := s.a.Organization.InviteStaff(s.ctx, s.tx, inviter, operator, email, role)
	if e != nil {
		return p, e
	}
	_, e = s.a.Organization.AcceptStaffInvitation(s.ctx, s.tx, invite.Token, organization.VerifiedStaff{ID: p.ID, Email: email, EmailVerified: true})
	if e == nil {
		s.out.Accounts = append(s.out.Accounts, DemoAccount{email, "staff", role, operator})
	}
	return p, e
}
func (s *demoBuilder) operator(parent, actor, email, name string, kind organization.OperatorKind) (organization.OperatorCreated, Staff, error) {
	created, e := s.a.Organization.CreateOperator(s.ctx, s.tx, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: parent, Kind: kind, Name: name, LegalName: "测试主体 · 不用于真实交易", BrandName: "测试 OEM 品牌"})
	if e != nil {
		return created, Staff{}, e
	}
	owner, e := s.staff(actor, created.Operator.ID, email, name+"负责人", "owner")
	if e != nil {
		return created, owner, e
	}
	terms := json.RawMessage(`{"version":1,"voluntary_refund":false,"method":"non_refundable","terms_reference":"TEST DATA ONLY"}`)
	_, e = s.a.Cooperation.DeliverPurchase(s.ctx, s.tx, cooperation.PurchaseInput{BuyerID: created.Operator.ID, SupplierID: parent, OrderID: uuid.NewString(), ReceiptReference: demoEvidence, ActorID: actor, PaidUSD: ledger.MustDecimal("800"), PaidATG: ledger.MustDecimal("1000"), ExitTerms: terms, At: time.Now()})
	if e != nil {
		return created, owner, e
	}
	_, e = s.a.Cooperation.PublishPurchaseQuote(s.ctx, s.tx, actor, cooperation.PurchaseQuote{SupplierID: parent, BuyerID: created.Operator.ID, PriceUSD: ledger.MustDecimal("80"), PaidATG: ledger.MustDecimal("100"), GiftATG: ledger.MustDecimal("10"), ExitTerms: terms, TermsReference: demoEvidence, Enabled: true})
	return created, owner, e
}
func (s *demoBuilder) setup(operator, actor string) (commerce.PackageQuote, incentives.Policy, error) {
	d := ledger.MustDecimal
	policy, e := s.a.Incentives.CurrentPolicy(s.ctx, s.tx, operator)
	if errors.Is(e, incentives.ErrNotReady) {
		policy, e = s.a.Incentives.SetPolicy(s.ctx, s.tx, actor, operator, incentives.PolicyInput{ThresholdUSD: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 60, InviteeATG: ledger.Zero(), InviterATG: ledger.Zero(), RewardValiditySeconds: 2592000, KOLCrossActivityLimit: 3, Evidence: demoEvidence})
	}
	if e != nil {
		return commerce.PackageQuote{}, policy, e
	}
	_, e = s.a.Payments.SaveMerchant(s.ctx, actor, payments.Merchant{ID: uuid.NewString(), OperatorID: operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "TEST-ONLY-NOT-A-REAL-BANK-ACCOUNT", "instructions": "仅限测试验收，请勿转入真实资金。由测试经营人员登记合成收款事实。"}}, 0)
	if e != nil {
		return commerce.PackageQuote{}, policy, e
	}
	quote, e := s.a.Commerce.PublishPackage(s.ctx, s.tx, commerce.PublishPackageInput{ActorStaffID: actor, OperatorID: operator, Name: "测试 · 月度体验包", TermsReference: demoEvidence, Timezone: "Asia/Shanghai", PriceUSD: d("80"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if e != nil {
		return quote, policy, e
	}
	_, e = s.a.Commerce.PublishPackage(s.ctx, s.tx, commerce.PublishPackageInput{ActorStaffID: actor, OperatorID: operator, Name: "测试 · 长期备用包", TermsReference: demoEvidence, Timezone: "Asia/Shanghai", PriceUSD: d("20"), PaidATG: d("20"), GiftATG: d("2"), Period: ledger.Lifetime, NewPurchaseEnabled: true})
	if e != nil {
		return quote, policy, e
	}
	_, e = s.a.Incentives.SetWithdrawalPolicy(s.ctx, s.tx, actor, operator, incentives.WithdrawalPolicy{MinimumUSD: d("1"), FlatFeeUSD: ledger.Zero(), FeeRate: ledger.Zero(), MethodReference: "TEST ONLY: artificial evidence, no real payout"}, 0)
	return quote, policy, e
}
func (s *demoBuilder) customer(brand, operator, email, name, invite string) (organization.RegistrationResult, error) {
	// Verified synthetic identities are created inside this transaction only.
	// Real public registration still requires configured verification mail.
	challenge, e := s.a.Organization.BeginRegistration(s.ctx, s.tx, organization.RegistrationIntentRequest{BrandID: brand, Email: email, Password: "temporary-fixture-password", InviteCode: invite})
	if e != nil {
		return organization.RegistrationResult{}, e
	}
	if _, e = s.a.Incentives.PrepareRegistrationPromise(s.ctx, s.tx, challenge.Intent); e != nil {
		return organization.RegistrationResult{}, e
	}
	if _, e = s.a.Organization.VerifyRegistration(s.ctx, s.tx, challenge.Intent.ID, challenge.VerificationToken); e != nil {
		return organization.RegistrationResult{}, e
	}
	result, e := s.a.Organization.CompleteRegistration(s.ctx, s.tx, organization.CompleteRegistrationInput{IntentID: challenge.Intent.ID, VerificationToken: challenge.VerificationToken, DisplayName: name}, s.a.Incentives)
	if e != nil {
		return result, e
	}
	e = s.tx.Table("organization_customer_credentials").Where("customer_id=?", result.Customer.ID).Update("password_hash", s.hash).Error
	if e == nil {
		s.out.Accounts = append(s.out.Accounts, DemoAccount{email, "customer", "customer", operator})
	}
	return result, e
}
func (s *demoBuilder) fund(actor string, order commerce.Order) error {
	now := time.Now().UTC()
	amount := order.DueUSD
	receipt, e := s.a.Commerce.RecordManualReceipt(s.ctx, s.tx, actor, commerce.VerifiedReceipt{OperatorID: order.OperatorID, ProviderReceiptID: "TEST-ONLY-" + order.ID, GrossUSD: amount, FeeStatus: "pending", Currency: "USD", OriginalAmount: amount.String(), RecipientReference: "TEST-ONLY-MERCHANT", PayerReference: "TEST-ONLY-CUSTOMER", ValuationEvidence: "synthetic USD = USD, no conversion", VerificationEvidence: demoEvidence, PaidAt: &now})
	if e != nil {
		return e
	}
	if _, e = s.a.Commerce.AllocateReceipt(s.ctx, s.tx, commerce.AllocateReceiptInput{ActorStaffID: actor, OperatorID: order.OperatorID, BusinessKey: "demo-allocate:" + order.ID, ReceiptID: receipt.ID, OrderID: order.ID, AmountUSD: amount}); e != nil {
		return e
	}
	_, e = s.a.Commerce.Fulfill(s.ctx, s.tx, order.ID, "demo-seed")
	return e
}
func (s *demoBuilder) balance(actor, operator, customer string, quote commerce.PackageQuote) error {
	order, e := s.a.Commerce.CreateTopUpOrder(s.ctx, s.tx, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: "demo-topup:" + customer, ExpectedOwnershipVersion: 1, AmountUSD: ledger.MustDecimal("200")})
	if e != nil {
		return e
	}
	if e = s.fund(actor, order); e != nil {
		return e
	}
	order, e = s.a.Commerce.CreateOrder(s.ctx, s.tx, commerce.CreateOrderInput{OperatorID: operator, CustomerID: customer, QuoteID: quote.ID, BusinessKey: "demo-package:" + customer, ExpectedOwnershipVersion: 1, InternalUSD: ledger.MustDecimal("80"), CommissionUSD: ledger.Zero()})
	if e != nil {
		return e
	}
	_, e = s.a.Commerce.Fulfill(s.ctx, s.tx, order.ID, "demo-seed")
	return e
}
func (s *demoBuilder) run(in DemoOptions) error {
	a := s.a
	platform, owner := a.Installation.OperatorID, a.Installation.OwnerStaffID
	s.out.Note = "Synthetic test data only. New test accounts use password1; the existing owner is unchanged. OEM public domain is not activated by this command. Real mail and payment providers still require configuration."
	var existingOwner Staff
	if e := s.tx.First(&existingOwner, "id=?", owner).Error; e != nil {
		return e
	}
	if strings.HasSuffix(existingOwner.Email, "@tokenhub.local") && organization.CheckPassword(existingOwner.PasswordHash, "password1") {
		s.out.Accounts = append(s.out.Accounts, DemoAccount{existingOwner.Email, "staff", "owner", platform})
	}
	for _, role := range []string{"operations", "finance", "support", "readonly"} {
		if _, e := s.staff(owner, platform, role+"@tokenhub.local", "测试 · "+role, role); e != nil {
			return e
		}
	}
	tech, e := s.staff(owner, platform, "tech@tokenhub.local", "测试 · 平台技术", "readonly")
	if e != nil {
		return e
	}
	if e = s.tx.Exec("INSERT INTO organization_staff_grants(membership_id,action) SELECT id,'catalog.manage' FROM organization_staff_memberships WHERE operator_id=? AND staff_id=?", platform, tech.ID).Error; e != nil {
		return e
	}
	oem, oemOwner, e := s.operator(platform, owner, "oem@tokenhub.local", "测试 · OEM", organization.OEM)
	if e != nil {
		return e
	}
	channel, channelOwner, e := s.operator(platform, owner, "channel@tokenhub.local", "测试 · 官方渠道", organization.Channel)
	if e != nil {
		return e
	}
	oemChannel, _, e := s.operator(oem.Operator.ID, oemOwner.ID, "oem-channel@tokenhub.local", "测试 · OEM渠道", organization.Channel)
	if e != nil {
		return e
	}
	s.out.ChannelInvite = channel.InviteCode
	if _, e = s.staff(channelOwner.ID, channel.Operator.ID, "channel-finance@tokenhub.local", "测试 · 渠道财务", "finance"); e != nil {
		return e
	}
	quote, policy, e := s.setup(platform, owner)
	if e != nil {
		return fmt.Errorf("platform setup: %w", e)
	}
	channelQuote, _, e := s.setup(channel.Operator.ID, channelOwner.ID)
	if e != nil {
		return fmt.Errorf("channel setup: %w", e)
	}
	if _, _, e = s.setup(oem.Operator.ID, oemOwner.ID); e != nil {
		return e
	}
	if in.ModelBaseURL != "" {
		if e = s.model(in, [][2]string{{platform, channel.Operator.ID}, {platform, oem.Operator.ID}, {oem.Operator.ID, oemChannel.Operator.ID}}); e != nil {
			return e
		}
	}
	// Temporarily allow synthetic account/order creation, restoring every flag
	// before commit unless actual model readiness permits opening that feature.
	type saved struct {
		OperatorID                                      string
		Registration, NewPurchase, ModelUsage, Activity bool
	}
	var flags []saved
	if e = s.tx.Table("organization_operator_statuses").Where("operator_id IN ?", []string{platform, channel.Operator.ID}).Find(&flags).Error; e != nil {
		return e
	}
	if e = s.tx.Table("organization_operator_statuses").Where("operator_id IN ?", []string{platform, channel.Operator.ID}).Updates(map[string]any{"registration": true, "new_purchase": true, "activity": true}).Error; e != nil {
		return e
	}
	kol, e := s.customer(a.Installation.BrandID, platform, "kol@tokenhub.local", "测试 · 推广达人", "")
	if e != nil {
		return fmt.Errorf("kol: %w", e)
	}
	customer, e := s.customer(a.Installation.BrandID, platform, "customer@tokenhub.local", "测试 · 已购客户", kol.InviteCode)
	if e != nil {
		return e
	}
	if _, e = s.customer(a.Installation.BrandID, platform, "new@tokenhub.local", "测试 · 新客户", ""); e != nil {
		return e
	}
	channelCustomer, e := s.customer(a.Installation.BrandID, channel.Operator.ID, "channel-customer@tokenhub.local", "测试 · 渠道客户", channel.InviteCode)
	if e != nil {
		return e
	}
	for _, p := range []organization.RegistrationResult{kol, customer} {
		if e = s.balance(owner, platform, p.Customer.ID, quote); e != nil {
			return fmt.Errorf("platform balance: %w", e)
		}
	}
	if e = s.balance(channelOwner.ID, channel.Operator.ID, channelCustomer.Customer.ID, channelQuote); e != nil {
		return e
	}
	// Existing, historically confirmed KOL is an explicit opening fixture, not a
	// path to obtain qualification from an unconfirmed new purchase.
	acquired := time.Now().Add(-24 * time.Hour)
	if e = s.tx.Exec("INSERT INTO incentives_qualification_accounts(operator_id,customer_id,confirmed_usd,acquired_at,acquired_policy_id,pending_offset_usd) VALUES(?,?,?,?,?,0) ON CONFLICT(operator_id,customer_id) DO UPDATE SET confirmed_usd=EXCLUDED.confirmed_usd,acquired_at=EXCLUDED.acquired_at,acquired_policy_id=EXCLUDED.acquired_policy_id", platform, kol.Customer.ID, "100", acquired, policy.ID).Error; e != nil {
		return e
	}
	for i := range s.out.Accounts {
		if s.out.Accounts[i].Email == "kol@tokenhub.local" {
			s.out.Accounts[i].Role = "kol"
		}
	}
	if _, e = a.Incentives.PublishKOLCampaign(s.ctx, s.tx, kol.Customer.ID, incentives.CampaignInput{OperatorID: platform, PackageID: quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().AddDate(1, 0, 0), ConcessionRate: ledger.MustDecimal("0.1"), PerCustomerLimit: 2, TotalUseLimit: 100, GiftBudgetATG: ledger.MustDecimal("1000"), Evidence: demoEvidence}); e != nil {
		return e
	}
	if _, e = a.Incentives.PublishCampaign(s.ctx, s.tx, incentives.CampaignInput{ActorStaffID: owner, OperatorID: platform, Kind: "merchant", PackageID: quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().AddDate(1, 0, 0), GiftATG: ledger.MustDecimal("5"), PerCustomerLimit: 2, TotalUseLimit: 100, GiftBudgetATG: ledger.MustDecimal("500"), DiscountBudgetUSD: ledger.Zero(), Evidence: demoEvidence}); e != nil {
		return e
	}
	for _, f := range flags {
		if e = s.tx.Table("organization_operator_statuses").Where("operator_id=?", f.OperatorID).Updates(map[string]any{"registration": f.Registration, "new_purchase": f.NewPurchase, "model_usage": f.ModelUsage, "activity": f.Activity}).Error; e != nil {
			return e
		}
	}
	if in.ModelBaseURL != "" {
		for _, pair := range [][2]string{{platform, owner}, {channel.Operator.ID, channelOwner.ID}, {oem.Operator.ID, oemOwner.ID}} {
			for _, feature := range []organization.BusinessFeature{organization.NewPurchase, organization.Renewal, organization.ModelUsage, organization.Activity} {
				state, e := a.Organization.GetBusinessStatus(s.ctx, s.tx, pair[0])
				if e != nil {
					return e
				}
				if _, e = a.Organization.SetBusinessStatus(s.ctx, s.tx, organization.SetBusinessStatusInput{ActorStaffID: pair[1], OperatorID: pair[0], Feature: feature, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: demoEvidence}, a); e != nil {
					var missing *organization.ReadinessError
					if errors.As(e, &missing) {
						return fmt.Errorf("opening %s %s: %v", pair[0], feature, missing.Missing)
					}
					return e
				}
			}
		}
	}
	return nil
}
func (s *demoBuilder) model(in DemoOptions, children [][2]string) error {
	a := s.a
	platform := a.Installation.OperatorID
	modelID := "demo-chat"
	provider, credential, route := uuid.NewString(), uuid.NewString(), uuid.NewString()
	cap := modelruntime.Capability{Operation: modelruntime.Chat, Streaming: true, InputTypes: []string{"text"}, Parameters: []string{"max_completion_tokens"}, RequiredParameters: []string{"max_completion_tokens"}, OutputLimitParameter: "max_completion_tokens", ParameterConstraints: map[string]modelruntime.ParameterConstraint{"max_completion_tokens": {Min: "6", Max: "256"}}, MaxInputBytes: 8192, MaxOutput: 256, BillableBounds: map[string]string{"input": "8192", "output": "256"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}, {Component: "output", Unit: "token", Path: "/completion_tokens"}}, EvidenceID: "independent-test-model-contract-v1", VerifiedAt: time.Now(), ParametersVerified: true, UsageVerified: true, BoundsVerified: true, RecoveryVerified: true}
	m := catalog.PublicModel{ID: modelID, Identity: modelID, DisplayName: "测试模型 · Demo Chat（固定回复，无真实 AI）", Capabilities: []modelruntime.Capability{cap}}
	syncCap := cap
	syncCap.Streaming = false
	m.Capabilities = []modelruntime.Capability{syncCap, cap}
	if e := a.Catalog.SavePublicModel(s.ctx, platform, m, 0); e != nil {
		return e
	}
	if _, e := a.Catalog.SaveCredential(s.ctx, platform, credential, provider, "测试模型密钥", in.ModelKey, true, 0); e != nil {
		return e
	}
	u := catalog.UpstreamModel{ID: route, PublicModelID: modelID, Identity: modelID, ProviderID: provider, Provider: "openai", Model: "tokenhub-test-chat-v1", CredentialID: credential, Endpoints: map[modelruntime.Operation]string{modelruntime.Chat: strings.TrimRight(in.ModelBaseURL, "/") + "/v1/chat/completions"}, Capabilities: m.Capabilities, Enabled: true, Healthy: true}
	if e := a.Catalog.SaveUpstream(s.ctx, platform, u, 0); e != nil {
		return e
	}
	m.Enabled = true
	if e := a.Catalog.SavePublicModel(s.ctx, platform, m, 1); e != nil {
		return e
	}
	if e := a.Catalog.SetListing(s.ctx, platform, modelID, true, 0); e != nil {
		return e
	}
	scopes := []catalog.PriceScope{{Kind: "supplier", SellerID: provider, BuyerID: route}, {Kind: "retail", SellerID: platform}}
	for _, pair := range children {
		parent, child := pair[0], pair[1]
		if e := a.Catalog.SetAuthorization(s.ctx, parent, child, modelID, true, 0); e != nil {
			return e
		}
		if e := a.Catalog.SetListing(s.ctx, child, modelID, true, 0); e != nil {
			return e
		}
		scopes = append(scopes, catalog.PriceScope{Kind: "supply", SellerID: parent, BuyerID: child}, catalog.PriceScope{Kind: "retail", SellerID: child})
	}
	for _, scope := range scopes {
		actor := scope.SellerID
		if scope.Kind == "supplier" {
			actor = platform
		}
		p := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: modelID, Layer: "regular", EffectiveAt: time.Now().Add(-time.Minute), Published: true, NoticeID: demoEvidence, Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1000", Rate: "0.1"}, {Key: "output", Unit: "token", Per: "1000", Rate: "0.2"}}}}
		if e := a.Catalog.PublishPrice(s.ctx, actor, p, a.pricePublished); e != nil {
			return e
		}
	}
	s.out.ModelID = modelID
	return nil
}
