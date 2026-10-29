// Package commerce keeps orders, verified receipts and asset delivery separate.
// All money mutations use ledger and accept the caller's PostgreSQL transaction.
package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput      = errors.New("commerce: invalid input")
	ErrNotFound          = errors.New("commerce: not found")
	ErrConflict          = errors.New("commerce: idempotency or quote version conflict")
	ErrScope             = errors.New("commerce: operating scope mismatch")
	ErrNotReady          = errors.New("commerce: required business adapter is not configured")
	ErrNotPaid           = errors.New("commerce: order is not fully funded")
	ErrOrderState        = errors.New("commerce: order state does not permit this action")
	ErrLatePayment       = errors.New("commerce: actual payment occurred after order expiry; receipt remains unallocated")
	ErrTimingUnverified  = errors.New("commerce: timely payment cannot be proven; receipt remains unallocated")
	ErrExcessAllocation  = errors.New("commerce: allocation exceeds receipt or order remainder")
	ErrWholePayment      = errors.New("commerce: one whole external receipt is required per order")
	ErrPaymentUnknown    = errors.New("commerce: verified unpaid payment status is required before closure")
	ErrRenewalNotDue     = errors.New("commerce: renewal is not due")
	ErrReceiptRestricted = errors.New("commerce: original receipt needs dispute or refund review before a new allocation")
)

type Package struct {
	ID                 string    `json:"id"`
	OperatorID         string    `json:"operator_id"`
	Name               string    `json:"name"`
	NewPurchaseEnabled bool      `json:"new_purchase_enabled"`
	RenewalEnabled     bool      `json:"renewal_enabled"`
	CurrentQuoteID     *string   `json:"current_quote_id"`
	CreatedAt          time.Time `json:"created_at"`
}
type PackageQuote struct {
	ID             string         `json:"id"`
	PackageID      string         `json:"package_id"`
	OperatorID     string         `json:"operator_id"`
	Version        int64          `json:"version"`
	PriceMoney     ledger.Decimal `json:"price_money"`
	PaidATG        ledger.Decimal `json:"paid_atg"`
	GiftATG        ledger.Decimal `json:"gift_atg"`
	Period         ledger.Period  `json:"period"`
	Timezone       string         `json:"timezone"`
	TermsReference string         `json:"terms_reference"`
	TermsSummary   string         `json:"terms_summary"`
	CreatedAt      time.Time      `json:"created_at"`
}
type PublishPackageInput struct {
	ActorStaffID       string         `json:"actor_staff_id"`
	OperatorID         string         `json:"operator_id"`
	PackageID          string         `json:"package_id"`
	Name               string         `json:"name"`
	TermsReference     string         `json:"terms_reference"`
	TermsSummary       string         `json:"terms_summary"`
	Timezone           string         `json:"timezone"`
	ExpectedVersion    int64          `json:"expected_version"`
	PriceMoney         ledger.Decimal `json:"price_money"`
	PaidATG            ledger.Decimal `json:"paid_atg"`
	GiftATG            ledger.Decimal `json:"gift_atg"`
	Period             ledger.Period  `json:"period"`
	NewPurchaseEnabled bool           `json:"new_purchase_enabled"`
	RenewalEnabled     bool           `json:"renewal_enabled"`
}
type Order struct {
	MonetarySnapshot         ledger.MonetarySnapshot `json:"monetary_snapshot" gorm:"serializer:json;type:jsonb"`
	ID                       string                  `json:"id"`
	OperatorID               string                  `json:"operator_id"`
	OriginalOperatorID       string                  `json:"original_operator_id" gorm:"default:(-)"`
	OriginalOwnershipVersion int64                   `json:"original_ownership_version" gorm:"default:(-)"`
	BrandID                  string                  `json:"brand_id"`
	CustomerID               string                  `json:"customer_id"`
	BusinessKey              string                  `json:"business_key"`
	Fingerprint              string                  `json:"-"`
	OwnershipVersion         int64                   `json:"ownership_version"`
	Kind                     string                  `json:"kind"`
	QuoteID                  *string                 `json:"quote_id"`
	DueMoney                 ledger.Decimal          `json:"due_money"`
	PaidATG                  ledger.Decimal          `json:"paid_atg"`
	PackageGiftATG           ledger.Decimal          `json:"package_gift_atg"`
	MerchantGiftATG          ledger.Decimal          `json:"merchant_gift_atg"`
	KOLGiftATG               ledger.Decimal          `json:"kol_gift_atg"`
	InternalMoney            ledger.Decimal          `json:"internal_money"`
	CommissionMoney          ledger.Decimal          `json:"commission_money"`
	ExternalAllocatedMoney   ledger.Decimal          `json:"external_allocated_money"`
	HoldID                   *string                 `json:"hold_id"`
	InternalStatus           string                  `json:"internal_status"`
	CoverageReference        string                  `json:"coverage_reference"`
	CoverageVersion          int64                   `json:"coverage_version"`
	CoverageStatus           string                  `json:"coverage_status"`
	MarketingReference       string                  `json:"marketing_reference"`
	CommissionSnapshot       json.RawMessage         `json:"commission_snapshot" gorm:"type:jsonb"`
	Status                   string                  `json:"status"`
	ExceptionReason          string                  `json:"exception_reason"`
	Period                   ledger.Period           `json:"period"`
	Timezone                 string                  `json:"timezone"`
	SubscriptionID           *string                 `json:"subscription_id"`
	CycleDueAt               *time.Time              `json:"cycle_due_at"`
	ExpiresAt                time.Time               `json:"expires_at"`
	ClosedAt                 *time.Time              `json:"closed_at"`
	CreatedAt                time.Time               `json:"created_at"`
	CompletedAt              *time.Time              `json:"completed_at"`
}
type CreateOrderInput struct {
	CustomerID               string          `json:"customer_id"`
	OperatorID               string          `json:"operator_id"`
	QuoteID                  string          `json:"quote_id"`
	BusinessKey              string          `json:"business_key"`
	ExpectedOwnershipVersion int64           `json:"expected_ownership_version"`
	InternalMoney            ledger.Decimal  `json:"internal_money"`
	CommissionMoney          ledger.Decimal  `json:"commission_money"`
	Validity                 time.Duration   `json:"validity"`
	ActivitySelection        json.RawMessage `json:"activity_selection"`
	SubscriptionID           *string         `json:"subscription_id"`
	CycleDueAt               *time.Time      `json:"cycle_due_at"`
	renewalAuthorized        bool
}
type CreateTopUpInput struct {
	CustomerID               string         `json:"customer_id"`
	OperatorID               string         `json:"operator_id"`
	BusinessKey              string         `json:"business_key"`
	ExpectedOwnershipVersion int64          `json:"expected_ownership_version"`
	AmountMoney              ledger.Decimal `json:"amount_money"`
	Validity                 time.Duration  `json:"validity"`
}
type Commitment struct {
	Reference string `json:"reference"`
	Version   int64  `json:"version"`
}

// SalesCoverage is mandatory for ATG sales. Implementations must lock the
// current coverage/price version and reserve real capacity in the same tx.
type SalesCoverage interface {
	Reserve(context.Context, *gorm.DB, Order) (Commitment, error)
	Restore(context.Context, *gorm.DB, Order) (Commitment, error)
	Fulfill(context.Context, *gorm.DB, Order) error
	Release(context.Context, *gorm.DB, Order) error
}
type MarketingQuote struct {
	FinalMoney         ledger.Decimal  `json:"final_money"`
	MerchantGiftATG    ledger.Decimal  `json:"merchant_gift_atg"`
	KOLGiftATG         ledger.Decimal  `json:"kol_gift_atg"`
	Reference          string          `json:"reference"`
	CommissionSnapshot json.RawMessage `json:"commission_snapshot"`
}
type Marketing interface {
	Quote(context.Context, *gorm.DB, Order, json.RawMessage) (MarketingQuote, error)
	Fulfill(context.Context, *gorm.DB, Order) error
	Release(context.Context, *gorm.DB, Order) error
}

// CommissionFunding tracks real entry provenance while commerce owns cash
// reservations. Every method joins the same caller-owned transaction.
type CommissionFunding interface {
	ReserveCommissionFunding(context.Context, *gorm.DB, string, string, string, ledger.Decimal) error
	CompleteCommissionFunding(context.Context, *gorm.DB, string, string, string) error
	ReleaseCommissionFunding(context.Context, *gorm.DB, string, string, string) error
	RequireCommissionFundingExecution(context.Context, *gorm.DB, string, string, string, string) error
	RestoreCommissionFunding(context.Context, *gorm.DB, string, string, string, string, ledger.Decimal) error
}
type ReceiptAllocationGuard interface {
	RequireReceiptAllocation(context.Context, *gorm.DB, string, string) error
}

// VerifiedReceipt is an adapter-to-domain fact, never an unverified HTTP body.
// Provider callbacks must be authenticated and queried before constructing it.
// Manual facts must use RecordManualReceipt, which checks finance permission.
type VerifiedReceipt struct {
	OriginalFeeAmount    *ledger.Decimal `json:"original_fee_amount,omitempty"`
	OriginalNetAmount    *ledger.Decimal `json:"original_net_amount,omitempty"`
	OperatorID           string          `json:"operator_id"`
	Provider             string          `json:"provider"`
	ProviderReceiptID    string          `json:"provider_receipt_id"`
	GrossMoney           ledger.Decimal  `json:"gross_money"`
	NetMoney             *ledger.Decimal `json:"net_money"`
	FeeMoney             *ledger.Decimal `json:"fee_money"`
	FeeStatus            string          `json:"fee_status"`
	Currency             string          `json:"currency"`
	OriginalAmount       string          `json:"original_amount"`
	Network              string          `json:"network"`
	RecipientReference   string          `json:"recipient_reference"`
	PayerReference       string          `json:"payer_reference"`
	ValuationEvidence    string          `json:"valuation_evidence"`
	VerificationEvidence string          `json:"verification_evidence"`
	VerifiedBy           string          `json:"verified_by"`
	PaidAt               *time.Time      `json:"paid_at"`
	ObservedAt           time.Time       `json:"observed_at"`
	SettledAt            *time.Time      `json:"settled_at"`
}
type Receipt struct {
	OriginalFeeAmount    *ledger.Decimal `json:"original_fee_amount,omitempty"`
	OriginalNetAmount    *ledger.Decimal `json:"original_net_amount,omitempty"`
	ID                   string          `json:"id"`
	OperatorID           string          `json:"operator_id"`
	Provider             string          `json:"provider"`
	ProviderReceiptID    string          `json:"provider_receipt_id"`
	Fingerprint          string          `json:"-"`
	GrossMoney           ledger.Decimal  `json:"gross_money"`
	NetMoney             *ledger.Decimal `json:"net_money"`
	FeeMoney             *ledger.Decimal `json:"fee_money"`
	FeeStatus            string          `json:"fee_status"`
	AllocatedMoney       ledger.Decimal  `json:"allocated_money"`
	Currency             string          `json:"currency"`
	OriginalAmount       string          `json:"original_amount"`
	Network              string          `json:"network"`
	RecipientReference   string          `json:"recipient_reference"`
	PayerReference       string          `json:"payer_reference"`
	ValuationEvidence    string          `json:"valuation_evidence"`
	VerificationEvidence string          `json:"verification_evidence"`
	VerifiedBy           string          `json:"verified_by"`
	PaidAt               *time.Time      `json:"paid_at"`
	ObservedAt           time.Time       `json:"observed_at"`
	SettledAt            *time.Time      `json:"settled_at"`
	CreatedAt            time.Time       `json:"created_at"`
}
type Allocation struct {
	ID          string         `json:"id"`
	OperatorID  string         `json:"operator_id"`
	BusinessKey string         `json:"business_key"`
	ReceiptID   string         `json:"receipt_id"`
	OrderID     string         `json:"order_id"`
	AmountMoney ledger.Decimal `json:"amount_money"`
	TimingProof string         `json:"timing_proof"`
	CreatedAt   time.Time      `json:"created_at"`
}
type AllocateReceiptInput struct {
	ActorStaffID string         `json:"actor_staff_id"`
	OperatorID   string         `json:"operator_id"`
	BusinessKey  string         `json:"business_key"`
	ReceiptID    string         `json:"receipt_id"`
	OrderID      string         `json:"order_id"`
	AmountMoney  ledger.Decimal `json:"amount_money"`
}
type Delivery struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"order_id"`
	GrantID     *string    `json:"grant_id"`
	EffectiveAt time.Time  `json:"effective_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	DeliveredAt time.Time  `json:"delivered_at"`
}

// PaymentClosureProof comes from a successful provider/manual reconciliation.
// Absence of a local callback is not proof that no money has been received.
type PaymentClosureProof struct {
	VerifiedUnpaid    bool   `json:"verified_unpaid"`
	EvidenceReference string `json:"evidence_reference"`
}
type Subscription struct {
	ID                    string     `json:"id"`
	OperatorID            string     `json:"operator_id"`
	OriginalOperatorID    string     `json:"original_operator_id" gorm:"default:(-)"`
	CustomerID            string     `json:"customer_id"`
	PackageID             string     `json:"package_id"`
	SourceOrderID         string     `json:"source_order_id"`
	AuthorizedQuoteID     string     `json:"authorized_quote_id"`
	AuthorizationEvidence string     `json:"authorization_evidence"`
	AllowCommission       bool       `json:"allow_commission"`
	Status                string     `json:"status"`
	NextDueAt             time.Time  `json:"next_due_at"`
	NoticeQuoteID         *string    `json:"notice_quote_id"`
	NoticeCycleDueAt      *time.Time `json:"notice_cycle_due_at"`
	NoticeDeliveredAt     *time.Time `json:"notice_delivered_at"`
	NoticeEvidence        string     `json:"notice_evidence"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"created_at"`
}
type RenewalResult struct {
	Status   string    `json:"status"`
	Order    *Order    `json:"order"`
	Delivery *Delivery `json:"delivery"`
}
