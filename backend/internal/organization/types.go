// Package organization owns brands, operating entities, customer identity and
// service attribution. Its transaction methods can join an application-owned
// PostgreSQL transaction; it never creates balances or rewards implicitly.
package organization

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var (
	ErrNotFound               = errors.New("organization: not found")
	ErrInvalidInput           = errors.New("organization: invalid input")
	ErrForbidden              = errors.New("organization: forbidden")
	ErrConflict               = errors.New("organization: concurrent or unique conflict")
	ErrInvalidRelationship    = errors.New("organization: invalid supply relationship")
	ErrInvalidInviteCode      = errors.New("organization: invalid, disabled or foreign brand invite code")
	ErrEmailAlreadyRegistered = errors.New("organization: email has already belonged to an account in this brand")
	ErrInvalidVerification    = errors.New("organization: verification is invalid or expired")
	ErrInvalidCredentials     = errors.New("organization: invalid credentials")
	ErrBusinessClosed         = errors.New("organization: business feature is closed")
	ErrStaleOwnership         = errors.New("organization: service ownership version changed")
	ErrReadiness              = errors.New("organization: business opening requirements are missing")
)

type OperatorKind string

const (
	Platform OperatorKind = "platform"
	OEM      OperatorKind = "oem"
	Channel  OperatorKind = "channel"
)

type Brand struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Name            string    `json:"name"`
	OwnerOperatorID string    `json:"owner_operator_id"`
	APIBaseURL      string    `json:"api_base_url"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}
type Operator struct {
	ID               string       `json:"id"`
	BrandID          string       `json:"brand_id"`
	ParentOperatorID *string      `json:"parent_operator_id"`
	Kind             OperatorKind `json:"kind"`
	Name             string       `json:"name"`
	LegalName        string       `json:"legal_name"`
	Lifecycle        string       `json:"lifecycle"`
	CreatedAt        time.Time    `json:"created_at"`
}
type SupplyAgreement struct {
	ID                 string    `json:"id"`
	SupplierID         string    `json:"supplier_id"`
	BuyerID            string    `json:"buyer_id"`
	Status             string    `json:"status"`
	QuoteReference     string    `json:"quote_reference"`
	ExitTermsReference string    `json:"exit_terms_reference"`
	CreatedAt          time.Time `json:"created_at"`
}
type BusinessFeature string

const (
	Registration BusinessFeature = "registration"
	NewPurchase  BusinessFeature = "new_purchase"
	Renewal      BusinessFeature = "renewal"
	ModelUsage   BusinessFeature = "model_usage"
	Activity     BusinessFeature = "activity"
	Settlement   BusinessFeature = "settlement"
)

type BusinessStatus struct {
	OperatorID   string    `json:"operator_id" gorm:"primaryKey"`
	Registration bool      `json:"registration"`
	NewPurchase  bool      `json:"new_purchase"`
	Renewal      bool      `json:"renewal"`
	ModelUsage   bool      `json:"model_usage"`
	Activity     bool      `json:"activity"`
	Settlement   bool      `json:"settlement"`
	Version      int64     `json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type BootstrapInput struct{ BrandName, OperatorName, LegalName, APIBaseURL, OwnerStaffID string }
type CreateOperatorInput struct {
	ActorStaffID, ParentOperatorID                                             string
	Kind                                                                       OperatorKind
	Name, LegalName, BrandName, APIBaseURL, QuoteReference, ExitTermsReference string
}
type OperatorCreated struct {
	Operator   Operator         `json:"operator"`
	Brand      Brand            `json:"brand"`
	Supply     *SupplyAgreement `json:"supply,omitempty"`
	InviteCode string           `json:"invite_code,omitempty"`
}

type Membership struct {
	ID         string    `json:"id"`
	OperatorID string    `json:"operator_id"`
	StaffID    string    `json:"staff_id"`
	Role       string    `json:"role"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
type StaffInvitation struct {
	ID         string    `json:"id"`
	OperatorID string    `json:"operator_id"`
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	Token      string    `json:"-"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// VerifiedStaff must originate from the application's staff authentication flow.
// An email supplied directly by an HTTP client is not a verified staff identity.
type VerifiedStaff struct {
	ID, Email     string
	EmailVerified bool
}

type Customer struct {
	Version               int64     `json:"version" gorm:"default:(-)"`
	ID                    string    `json:"id"`
	BrandID               string    `json:"brand_id"`
	DisplayName           string    `json:"display_name"`
	Status                string    `json:"status"`
	RegisteredAt          time.Time `json:"registered_at"`
	NewCustomerRecordedAt time.Time `json:"new_customer_recorded_at"`
}
type Ownership struct {
	ID                string     `json:"id"`
	CustomerID        string     `json:"customer_id"`
	BrandID           string     `json:"brand_id"`
	OperatorID        string     `json:"operator_id"`
	Version           int64      `json:"version"`
	EffectiveAt       time.Time  `json:"effective_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	EvidenceReference string     `json:"evidence_reference"`
}
type RegistrationIntentRequest struct{ BrandID, Email, InviteCode, Password string }
type RegistrationIntent struct {
	ID           string     `json:"id"`
	BrandID      string     `json:"brand_id"`
	OperatorID   string     `json:"operator_id"`
	Email        string     `json:"email"`
	InviteCodeID *string    `json:"invite_code_id,omitempty"`
	InviterID    *string    `json:"inviter_id,omitempty"`
	Purpose      string     `json:"purpose"`
	Status       string     `json:"status"`
	ExpiresAt    time.Time  `json:"expires_at"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
	CustomerID   *string    `json:"customer_id,omitempty"`
}

// VerificationToken is delivered to the verified email channel, never a public
// registration-preview response. Only its SHA-256 digest is persisted.
type VerificationChallenge struct {
	Intent            RegistrationIntent `json:"intent"`
	VerificationToken string             `json:"-"`
}
type RegistrationAttribution struct {
	Brand        Brand    `json:"brand"`
	Operator     Operator `json:"operator"`
	InviteCodeID *string  `json:"invite_code_id,omitempty"`
	InviterID    *string  `json:"inviter_id,omitempty"`
}
type CompleteRegistrationInput struct{ IntentID, VerificationToken, DisplayName string }
type RegistrationResult struct {
	Customer        Customer  `json:"customer"`
	Ownership       Ownership `json:"ownership"`
	InviteCode      string    `json:"invite_code"`
	InviterID       *string   `json:"inviter_id,omitempty"`
	NewlyRegistered bool      `json:"newly_registered"`
}

// RegistrationHook runs once, inside the same registration transaction. Reward
// modules use it to advance qualification and record delivery or a durable
// delivery obligation. A failure rolls back registration; no fake grant exists.
type RegistrationHook interface {
	OnRegistered(context.Context, *gorm.DB, RegistrationResult) error
}
type RegistrationHookFunc func(context.Context, *gorm.DB, RegistrationResult) error

func (f RegistrationHookFunc) OnRegistered(ctx context.Context, tx *gorm.DB, r RegistrationResult) error {
	return f(ctx, tx, r)
}

type Referral struct {
	InviteeID, InviterID, BrandID, RegistrationOperatorID string
	CreatedAt                                             time.Time
}
type Principal struct {
	Customer          Customer  `json:"customer"`
	Ownership         Ownership `json:"ownership"`
	CredentialVersion int64     `json:"credential_version"`
}
type LoginResult struct {
	Principal   Principal `json:"principal"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Requirement struct {
	Code        string `json:"code"`
	Ready       bool   `json:"ready"`
	Description string `json:"description"`
}

// ReadinessChecker validates live model, pricing, payment, coverage or other
// prerequisites using the caller's transaction. Missing adapters cannot enable
// a feature. Registration has intrinsic identity-only prerequisites.
type ReadinessChecker interface {
	Check(context.Context, *gorm.DB, Operator, BusinessFeature) ([]Requirement, error)
}
type ReadinessError struct{ Missing []Requirement }

func (e *ReadinessError) Error() string { return ErrReadiness.Error() }
func (e *ReadinessError) Unwrap() error { return ErrReadiness }

type SetBusinessStatusInput struct {
	ActorStaffID, OperatorID string
	Feature                  BusinessFeature
	Enabled                  bool
	ExpectedVersion          int64
	EvidenceReference        string
}

// TakeoverOwnershipInput is for a completed, evidence-backed takeover
// orchestrator. Every current channel customer must be supplied, with its
// observed version. Financial responsibility must move in the same outer tx.
type TakeoverOwnershipInput struct {
	ChannelID, BrandOwnerOperatorID, EvidenceReference string
	ExpectedVersions                                   map[string]int64
}
