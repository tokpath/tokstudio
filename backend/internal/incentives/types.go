// Package incentives owns qualification, two-hop commissions, campaign
// commitments and verified manual commission payouts. It never changes a
// customer's USD wallet or ATG to collect a commission recovery.
package incentives

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput    = errors.New("incentives: invalid input")
	ErrNotFound        = errors.New("incentives: not found")
	ErrConflict        = errors.New("incentives: conflicting command or version")
	ErrNotEligible     = errors.New("incentives: customer or campaign is not eligible")
	ErrNotReady        = errors.New("incentives: policy or coverage adapter is not configured")
	ErrLimit           = errors.New("incentives: activity use limit is exhausted")
	ErrCombination     = errors.New("incentives: activity combination is not explicitly authorized")
	ErrBlocked         = errors.New("incentives: commission release or use is blocked")
	ErrWithdrawalState = errors.New("incentives: withdrawal state does not permit this action")
	ErrRewardPromise   = errors.New("incentives: an active registration reward promise is required")
)

type Policy struct {
	ID                    string         `json:"id"`
	OperatorID            string         `json:"operator_id"`
	Version               int64          `json:"version"`
	ThresholdMoney        ledger.Decimal `json:"threshold_money"`
	DirectRate            ledger.Decimal `json:"direct_rate"`
	IndirectRate          ledger.Decimal `json:"indirect_rate"`
	ConfirmationSeconds   int64          `json:"confirmation_seconds"`
	InviteeATG            ledger.Decimal `json:"invitee_atg"`
	InviterATG            ledger.Decimal `json:"inviter_atg"`
	RewardValiditySeconds int64          `json:"reward_validity_seconds"`
	KOLCrossActivityLimit int            `json:"kol_cross_activity_limit"`
	EffectiveAt           time.Time      `json:"effective_at"`
	BusinessSequence      int64          `json:"business_sequence"`
}
type PolicyInput struct {
	Evidence              string         `json:"evidence"`
	ThresholdMoney        ledger.Decimal `json:"threshold_money"`
	DirectRate            ledger.Decimal `json:"direct_rate"`
	IndirectRate          ledger.Decimal `json:"indirect_rate"`
	ConfirmationSeconds   int64          `json:"confirmation_seconds"`
	InviteeATG            ledger.Decimal `json:"invitee_atg"`
	InviterATG            ledger.Decimal `json:"inviter_atg"`
	RewardValiditySeconds int64          `json:"reward_validity_seconds"`
	KOLCrossActivityLimit int            `json:"kol_cross_activity_limit"`
	ExpectedVersion       int64          `json:"expected_version"`
}
type Qualification struct {
	OriginalOperatorID string         `json:"original_operator_id" gorm:"default:(-)"`
	OperatorID         string         `json:"operator_id"`
	CustomerID         string         `json:"customer_id"`
	ConfirmedMoney     ledger.Decimal `json:"confirmed_money"`
	AcquiredAt         *time.Time     `json:"acquired_at"`
	AcquiredPolicyID   *string        `json:"acquired_policy_id"`
	PendingOffsetMoney ledger.Decimal `json:"pending_offset_money"`
}
type FunctionState struct {
	OriginalOperatorID string    `json:"original_operator_id" gorm:"default:(-)"`
	ID                 string    `json:"id"`
	OperatorID         string    `json:"operator_id"`
	CustomerID         string    `json:"customer_id"`
	Status             string    `json:"status"`
	EvidenceReference  string    `json:"evidence_reference"`
	ActorID            string    `json:"actor_id"`
	ReleaseBlocked     bool      `json:"release_blocked"`
	EffectiveAt        time.Time `json:"effective_at"`
	BusinessSequence   int64     `json:"business_sequence"`
}
type CampaignInput struct {
	Name             string          `json:"name"`
	Evidence         string          `json:"evidence"`
	ActorStaffID     string          `json:"actor_staff_id"`
	OwnerCustomerID  string          `json:"owner_customer_id"`
	OperatorID       string          `json:"operator_id"`
	CampaignID       string          `json:"campaign_id"`
	Kind             string          `json:"kind"`
	PackageID        string          `json:"package_id"`
	Eligibility      string          `json:"eligibility"`
	ExpectedVersion  int64           `json:"expected_version"`
	Enabled          bool            `json:"enabled"`
	StartsAt         time.Time       `json:"starts_at"`
	EndsAt           time.Time       `json:"ends_at"`
	FinalPriceMoney  *ledger.Decimal `json:"final_price_money"`
	GiftATG          ledger.Decimal  `json:"gift_atg"`
	ConcessionRate   ledger.Decimal  `json:"concession_rate"`
	PerCustomerLimit int64           `json:"per_customer_limit"`
	TotalUseLimit    int64           `json:"total_use_limit"`
}
type Campaign struct {
	OriginalOperatorID    string         `json:"original_operator_id" gorm:"default:(-)"`
	ID                    string         `json:"id"`
	OperatorID            string         `json:"operator_id"`
	Kind                  string         `json:"kind"`
	OwnerCustomerID       *string        `json:"owner_customer_id"`
	CurrentVersion        int64          `json:"current_version"`
	ReservedGiftATG       ledger.Decimal `json:"reserved_gift_atg"`
	UsedGiftATG           ledger.Decimal `json:"used_gift_atg"`
	ReservedDiscountMoney ledger.Decimal `json:"reserved_discount_money"`
	UsedDiscountMoney     ledger.Decimal `json:"used_discount_money"`
	ReservedUses          int64          `json:"reserved_uses"`
	UsedUses              int64          `json:"used_uses"`
}
type CampaignVersion struct {
	Name             string          `json:"name"`
	ID               string          `json:"id"`
	CampaignID       string          `json:"campaign_id"`
	PackageID        string          `json:"package_id"`
	Eligibility      string          `json:"eligibility"`
	Version          int64           `json:"version"`
	Enabled          bool            `json:"enabled"`
	StartsAt         time.Time       `json:"starts_at"`
	EndsAt           time.Time       `json:"ends_at"`
	FinalPriceMoney  *ledger.Decimal `json:"final_price_money"`
	GiftATG          ledger.Decimal  `json:"gift_atg"`
	ConcessionRate   ledger.Decimal  `json:"concession_rate"`
	PerCustomerLimit int64           `json:"per_customer_limit"`
	TotalUseLimit    int64           `json:"total_use_limit"`
	CreatedAt        time.Time       `json:"created_at"`
}
type ActivitySelection struct {
	MerchantCampaignID string `json:"merchant_campaign_id"`
	KOLCampaignID      string `json:"kol_campaign_id"`
}
type OrderSnapshot struct {
	OriginalOperatorID string         `json:"original_operator_id" gorm:"default:(-)"`
	OrderID            string         `json:"order_id"`
	OperatorID         string         `json:"operator_id"`
	CustomerID         string         `json:"customer_id"`
	PolicyID           string         `json:"policy_id"`
	DirectCustomerID   *string        `json:"direct_customer_id"`
	IndirectCustomerID *string        `json:"indirect_customer_id"`
	DirectRate         ledger.Decimal `json:"direct_rate"`
	IndirectRate       ledger.Decimal `json:"indirect_rate"`
	ConcessionRate     ledger.Decimal `json:"concession_rate"`
	MerchantCampaignID *string        `json:"merchant_campaign_id"`
	KOLCampaignID      *string        `json:"kol_campaign_id"`
	MerchantVersionID  *string        `json:"merchant_version_id"`
	KOLVersionID       *string        `json:"kol_version_id"`
	KOLCustomerID      *string        `json:"kol_customer_id"`
	MerchantGiftATG    ledger.Decimal `json:"merchant_gift_atg"`
	KOLGiftATG         ledger.Decimal `json:"kol_gift_atg"`
	DiscountMoney      ledger.Decimal `json:"discount_money"`
	GiftRemainder      string         `json:"gift_remainder"`
	Status             string         `json:"status"`
	CreatedAt          time.Time      `json:"created_at"`
}
type GrantEventInput struct {
	EventID          string         `json:"event_id"`
	OperatorID       string         `json:"operator_id"`
	CustomerID       string         `json:"customer_id"`
	OrderID          string         `json:"order_id"`
	Kind             string         `json:"kind"`
	PaidMoney        ledger.Decimal `json:"paid_money"`
	OccurredAt       time.Time      `json:"occurred_at"`
	BusinessSequence int64          `json:"business_sequence"`
}
type ConfirmationEvent struct {
	OriginalOperatorID string         `json:"original_operator_id" gorm:"default:(-)"`
	ID                 string         `json:"id"`
	SourceEventID      string         `json:"source_event_id"`
	OperatorID         string         `json:"operator_id"`
	CustomerID         string         `json:"customer_id"`
	OrderID            string         `json:"order_id"`
	Kind               string         `json:"kind"`
	PaidMoney          ledger.Decimal `json:"paid_money"`
	PolicyID           string         `json:"policy_id"`
	OccurredAt         time.Time      `json:"occurred_at"`
	SourceOccurredAt   time.Time      `json:"source_occurred_at"`
	ConfirmAt          time.Time      `json:"confirm_at"`
	Status             string         `json:"status"`
	Fingerprint        string         `json:"fingerprint"`
	BusinessSequence   int64          `json:"business_sequence"`
	OriginalEventID    *string        `json:"original_event_id"`
}
type CommissionEntry struct {
	OriginalOperatorID string         `json:"original_operator_id" gorm:"default:(-)"`
	ID                 string         `json:"id"`
	EventID            string         `json:"event_id"`
	OperatorID         string         `json:"operator_id"`
	OrderID            string         `json:"order_id"`
	BeneficiaryID      string         `json:"beneficiary_id"`
	Hop                int            `json:"hop"`
	AmountMoney        ledger.Decimal `json:"amount_money"`
	ReversedMoney      ledger.Decimal `json:"reversed_money"`
	OffsetMoney        ledger.Decimal `json:"offset_money"`
	Status             string         `json:"status"`
	ConfirmAt          time.Time      `json:"confirm_at"`
	CreatedAt          time.Time      `json:"created_at"`
}
type CorrectionInput struct {
	ActorStaffID    string         `json:"actor_staff_id"`
	OperatorID      string         `json:"operator_id"`
	OriginalEventID string         `json:"original_event_id"`
	BusinessKey     string         `json:"business_key"`
	Evidence        string         `json:"evidence"`
	AmountMoney     ledger.Decimal `json:"amount_money"`
}
type WithdrawalPolicy struct {
	Currency             string         `json:"currency,omitempty" gorm:"-"`
	MethodID             string         `json:"method_id"`
	MethodName           string         `json:"method_name"`
	Enabled              bool           `json:"enabled"`
	ID                   string         `json:"id"`
	OperatorID           string         `json:"operator_id"`
	Version              int64          `json:"version"`
	MinimumMoney         ledger.Decimal `json:"minimum_money"`
	FlatFeeMoney         ledger.Decimal `json:"flat_fee_money"`
	FeeRate              ledger.Decimal `json:"fee_rate"`
	RequiresDualApproval bool           `json:"requires_dual_approval"`
	MethodReference      string         `json:"method_reference"`
	EffectiveAt          time.Time      `json:"effective_at"`
}
type WithdrawalInput struct {
	OperatorID         string         `json:"operator_id"`
	CustomerID         string         `json:"customer_id"`
	BusinessKey        string         `json:"business_key"`
	RecipientReference string         `json:"recipient_reference"`
	PolicyID           string         `json:"policy_id"`
	AmountMoney        ledger.Decimal `json:"amount_money"`
}
type WithdrawalQuote struct {
	Policy       WithdrawalPolicy `json:"policy"`
	AmountMoney  ledger.Decimal   `json:"amount_money"`
	FeeMoney     ledger.Decimal   `json:"fee_money"`
	NetMoney     ledger.Decimal   `json:"net_money"`
	FeeRemainder string           `json:"fee_remainder"`
}
type Withdrawal struct {
	MethodID           string         `json:"method_id"`
	MethodName         string         `json:"method_name"`
	OriginalOperatorID string         `json:"original_operator_id" gorm:"default:(-)"`
	ID                 string         `json:"id"`
	OperatorID         string         `json:"operator_id"`
	CustomerID         string         `json:"customer_id"`
	BusinessKey        string         `json:"business_key"`
	Fingerprint        string         `json:"fingerprint"`
	PolicyID           string         `json:"policy_id"`
	AmountMoney        ledger.Decimal `json:"amount_money"`
	FeeMoney           ledger.Decimal `json:"fee_money"`
	NetMoney           ledger.Decimal `json:"net_money"`
	FeeRemainder       string         `json:"fee_remainder"`
	RecipientReference string         `json:"recipient_reference"`
	HoldID             string         `json:"hold_id"`
	Status             string         `json:"status"`
	ApprovedBy         *string        `json:"approved_by"`
	PaymentActorID     *string        `json:"payment_actor_id"`
	PaymentIntentID    *string        `json:"payment_intent_id"`
	PaymentReference   *string        `json:"payment_reference"`
	PaymentEvidence    string         `json:"payment_evidence"`
	PaidAt             *time.Time     `json:"paid_at"`
	CreatedAt          time.Time      `json:"created_at"`
}
type WithdrawalPaymentProof struct {
	PaymentReference  string         `json:"payment_reference"`
	Evidence          string         `json:"evidence"`
	Currency          string         `json:"currency"`
	OriginalAmount    string         `json:"original_amount"`
	ValuationEvidence string         `json:"valuation_evidence"`
	PaidMoney         ledger.Decimal `json:"paid_money"`
	PaidAt            time.Time      `json:"paid_at"`
}
type RegistrationPromise struct {
	OriginalOperatorID    string         `json:"original_operator_id" gorm:"default:(-)"`
	IntentID              string         `json:"intent_id"`
	OperatorID            string         `json:"operator_id"`
	InviterID             *string        `json:"inviter_id"`
	PolicyID              string         `json:"policy_id"`
	InviteeATG            ledger.Decimal `json:"invitee_atg"`
	InviterATG            ledger.Decimal `json:"inviter_atg"`
	ActualInviterATG      ledger.Decimal `json:"actual_inviter_atg"`
	RewardValiditySeconds int64          `json:"reward_validity_seconds"`
	CoverageReference     string         `json:"coverage_reference"`
	CoverageVersion       int64          `json:"coverage_version"`
	ExpiresAt             time.Time      `json:"expires_at"`
	Status                string         `json:"status"`
	CustomerID            *string        `json:"customer_id"`
	RegisteredAt          *time.Time     `json:"registered_at"`
	LastError             string         `json:"last_error"`
	CreatedAt             time.Time      `json:"created_at"`
}
type RegistrationCoverage interface {
	ReserveRegistration(context.Context, *gorm.DB, RegistrationPromise) (commerce.Commitment, error)
	FulfillRegistration(context.Context, *gorm.DB, RegistrationPromise, ledger.Decimal) error
	ReleaseRegistration(context.Context, *gorm.DB, RegistrationPromise) error
}
type Event struct {
	ID          string          `json:"id"`
	OperatorID  string          `json:"operator_id"`
	AggregateID string          `json:"aggregate_id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload" gorm:"type:jsonb"`
	CreatedAt   time.Time       `json:"created_at"`
	PublishedAt *time.Time      `json:"published_at"`
}
