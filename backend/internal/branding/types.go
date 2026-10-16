// Package branding manages verified brand domains and scoped presentation.
// Visual configuration never mutates brands, customers, ownership or assets.
package branding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput            = errors.New("branding: invalid input")
	ErrNotFound                = errors.New("branding: not found")
	ErrDomainBound             = errors.New("branding: domain is already bound")
	ErrConflict                = errors.New("branding: configuration version changed")
	ErrVerificationUnavailable = errors.New("branding: domain verification adapter unavailable")
	ErrDomainUnverified        = errors.New("branding: domain ownership or TLS is not verified")
)

// Directory is the only cross-module dependency. Branding does not access
// organization ORM models or infer financial permissions from hierarchy.
type Directory interface {
	GetBrand(context.Context, *gorm.DB, string) (organization.Brand, error)
	GetOperator(context.Context, *gorm.DB, string) (organization.Operator, error)
	RequireBrandBranding(context.Context, *gorm.DB, string, string) error
	RequireChannelBranding(context.Context, *gorm.DB, string, string) error
}

type Domain struct {
	ID                   string          `json:"id"`
	BrandID              string          `json:"brand_id"`
	Hostname             string          `json:"hostname"`
	VerificationToken    string          `json:"verification_token"`
	Status               string          `json:"status"`
	TLSReady             bool            `json:"tls_ready"`
	VerifiedAt           *time.Time      `json:"verified_at,omitempty"`
	CheckedAt            *time.Time      `json:"checked_at,omitempty"`
	ProviderReference    string          `json:"provider_reference"`
	ProviderStatus       string          `json:"provider_status"`
	TLSStatus            string          `json:"tls_status"`
	ProviderInstructions json.RawMessage `gorm:"type:jsonb;default:'[]'" json:"provider_instructions"`
	FailureReason        string          `json:"failure_reason"`
	Version              int64           `json:"version"`
	CreatedAt            time.Time       `json:"created_at"`
}
type DomainChallenge struct {
	Domain   Domain `json:"domain"`
	TXTName  string `json:"txt_name"`
	TXTValue string `json:"txt_value"`
}
type DomainVerification struct {
	OwnershipVerified                bool
	TLSReady                         bool
	ProviderReference, FailureReason string
	ProviderStatus, TLSStatus        string
	Instructions                     []DNSInstruction
}

type DNSInstruction struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Purpose string `json:"purpose"`
}

// DomainVerifier checks the exact hostname and challenge with a real DNS/TLS
// provider. Returning only a hostname or HTTP success is not verification.
type DomainVerifier interface {
	Verify(context.Context, string, string) (DomainVerification, error)
}
type DomainVerifierFunc func(context.Context, string, string) (DomainVerification, error)

func (f DomainVerifierFunc) Verify(ctx context.Context, h, t string) (DomainVerification, error) {
	return f(ctx, h, t)
}

// Nil inherits a field. An explicit empty LogoURL removes a logo while keeping
// every other field's independent inheritance behavior.
type ThemeOverrides struct {
	PrimaryColor    *string `json:"primary_color,omitempty"`
	BackgroundColor *string `json:"background_color,omitempty"`
	TextColor       *string `json:"text_color,omitempty"`
	LogoURL         *string `json:"logo_url,omitempty"`
	SupportURL      *string `json:"support_url,omitempty"`
}
type Theme struct {
	PrimaryColor    string `json:"primary_color"`
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
	LogoURL         string `json:"logo_url"`
	SupportURL      string `json:"support_url"`
}
type ThemeConfig struct {
	BrandID   string         `json:"brand_id"`
	ScopeType string         `json:"scope_type"`
	ScopeID   string         `json:"scope_id"`
	Values    ThemeOverrides `json:"values"`
	Version   int64          `json:"version"`
	UpdatedAt time.Time      `json:"updated_at"`
}
type ThemeContext struct {
	Brand          organization.Brand    `json:"brand"`
	Operator       organization.Operator `json:"operator"`
	Theme          Theme                 `json:"theme"`
	BrandVersion   int64                 `json:"brand_version"`
	ChannelVersion int64                 `json:"channel_version"`
}
type SetThemeInput struct {
	ActorStaffID, TargetID string
	ExpectedVersion        int64
	Values                 ThemeOverrides
}
