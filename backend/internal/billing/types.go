package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInsufficientQuota   = errors.New("insufficient channel quota")
	ErrNotFound            = errors.New("billing record not found")
	ErrConflict            = errors.New("idempotency conflict")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrInvalidIssueRatio   = errors.New("invalid issue ratio")
	ErrRedeemUnavailable   = errors.New("redeem code unavailable")
	ErrTopupNotPending     = errors.New("topup is not pending")
	ErrAuthNotReserved     = errors.New("authorization not reserved")
)

// Commissioner 由 commission 模块实现。billing 只提交 usage 摘要，不读佣金表。
type Commissioner interface {
	AccrueUsage(ctx context.Context, usageEventID, requestID, userID, channelOrgID string, wholesaleMinor int64) error
	ReverseUsage(ctx context.Context, usageEventID string) error
}

type ChannelPool interface {
	ResolvePoolChannelID(ctx context.Context, channelID string) (string, error)
}

type Qualifier interface {
	Consider(ctx context.Context, userID, channelOrgID string, singleTopup, lifetimeSpend int64) error
}

// EntitlementCoverer 由 plans 模块实现。billing 只问“能覆盖多少 USD”，不读套餐表。
type EntitlementCoverer interface {
	AvailableUSD(ctx context.Context, userID string) (int64, error)
	ConsumeUSD(ctx context.Context, userID, requestID string, amount int64) (int64, error)
	ReverseByRequest(ctx context.Context, requestID string) error
	ReverseKeep(ctx context.Context, requestID string, keep int64) error
}

const (
	CurrencyUSD = "USD"

	EventTopup            = "topup"
	EventAuthorization    = "authorization"
	EventRelease          = "release"
	EventUsageDebit       = "usage_debit"
	EventRefund           = "refund"
	EventAdjustment       = "adjustment"
	EventCommissionDebit  = "commission_debit"
	EventGiftCredit       = "gift_credit"
	EventCommissionCredit = "commission_credit"

	AuthReserved              = "reserved"
	AuthSettled               = "settled"
	AuthReleased              = "released"
	AuthPendingReconciliation = "pending_reconciliation"
	AuthReversed              = "reversed"

	UsageConfirmed = "confirmed"
	UsagePending   = "pending_reconciliation"
	UsageVoided    = "voided"

	ChargeCommitted = "committed"
	ChargeReversed  = "reversed"

	CommissionFrozen   = "frozen"
	CommissionReversed = "reversed"

	TopupPending           = "pending"
	TopupPaid              = "paid"
	TopupFailed            = "failed"
	TopupRefunded          = "refunded"
	TopupPartiallyRefunded = "partially_refunded"

	RedeemE2E      = "THE2E"
	RedeemCredit10 = "THCREDIT10"

	CommissionPolicyM3 = "m3-stub-v1"
	CommissionRateBPS  = 1000 // 批发价的 10%，只作可冲正挂接点；完整策略在 M6。
)

type ReserveInput struct {
	UserID         string          `json:"user_id"`
	ChannelOrgID   string          `json:"channel_org_id"`
	APIKeyID       string          `json:"api_key_id"`
	RequestID      string          `json:"request_id"`
	PublicModelID  string          `json:"public_model_id"`
	PriceVersionID string          `json:"price_version_id"`
	UnitPrices     json.RawMessage `json:"unit_prices"`
	ReserveMinor   int64           `json:"reserve_minor"`
}

type Reservation struct {
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	AmountMinor int64  `json:"amount_minor"`
	Status      string `json:"status"`
	Currency    string `json:"currency"`
}

type SettleInput struct {
	RequestID       string          `json:"request_id"`
	AttemptID       string          `json:"attempt_id"`
	UserID          string          `json:"user_id"`
	APIKeyID        string          `json:"api_key_id"`
	ChannelOrgID    string          `json:"channel_org_id"`
	PublicModelID   string          `json:"public_model_id"`
	ProviderID      string          `json:"provider_id"`
	UpstreamModelID string          `json:"upstream_model_id"`
	Usage           map[string]int  `json:"usage"`
	PriceVersionID  string          `json:"price_version_id"`
	UnitPrices      json.RawMessage `json:"unit_prices"`
	MissingUsage    bool            `json:"missing_usage"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Resolution      string          `json:"resolution"`
}

type Settlement struct {
	ChargeID     string `json:"charge_id,omitempty"`
	UsageEventID string `json:"usage_event_id,omitempty"`
	AmountMinor  int64  `json:"amount_minor"`
	State        string `json:"state"`
	Currency     string `json:"currency"`
}

type BalanceView struct {
	UserID                   string `json:"user_id"`
	Currency                 string `json:"currency"`
	AvailableMinor           int64  `json:"available_minor"`
	GiftMinor                int64  `json:"gift_minor"`
	PurchasedMinor           int64  `json:"purchased_minor"`
	CommissionAvailableMinor int64  `json:"commission_available_minor"`
	ReservedMinor            int64  `json:"reserved_minor"`
	AvailableUSD             string `json:"available"`
	ReservedUSD              string `json:"reserved"`
	ChannelQuota             int64  `json:"channel_quota_minor,omitempty"`
	AllocationRemaining      int64  `json:"allocation_remaining_minor,omitempty"`
}

type LedgerView struct {
	ID             string    `json:"id"`
	EventType      string    `json:"event_type"`
	AmountMinor    int64     `json:"amount_minor"`
	BalanceAfter   int64     `json:"balance_after_minor"`
	ReservedAfter  int64     `json:"reserved_after_minor"`
	ReferenceType  string    `json:"reference_type,omitempty"`
	ReferenceID    string    `json:"reference_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type UsageView struct {
	ID               string          `json:"id"`
	RequestID        string          `json:"request_id"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	UserID           string          `json:"user_id,omitempty"`
	APIKeyID         string          `json:"api_key_id,omitempty"`
	ChannelOrgID     string          `json:"channel_org_id,omitempty"`
	PublicModelID    string          `json:"public_model_id"`
	ProviderID       string          `json:"provider_id,omitempty"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	ReasoningTokens  int64           `json:"reasoning_tokens,omitempty"`
	UnitUsage        json.RawMessage `json:"unit_usage"`
	UnitPrices       json.RawMessage `json:"unit_prices"`
	PriceVersionID   string          `json:"price_version_id,omitempty"`
	CustomerMinor    int64           `json:"customer_amount_minor"`
	UpstreamMinor    int64           `json:"upstream_cost_minor"`
	WholesaleMinor   int64           `json:"wholesale_amount_minor"`
	State            string          `json:"state"`
	OccurredAt       time.Time       `json:"occurred_at"`
}

// QueryUsageInput 按渠道→用户→API Key，再交叉模型过滤账本。空字段表示不过滤。
type QueryUsageInput struct {
	UserID        string
	APIKeyID      string
	ChannelOrgID  string
	PublicModelID string
	Limit         int
}

type TopupView struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	RedeemCode    string    `json:"redeem_code,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type ReportView struct {
	RevenueMinor     int64 `json:"revenue_minor"`
	UpstreamMinor    int64 `json:"upstream_cost_minor"`
	WholesaleMinor   int64 `json:"wholesale_minor"`
	CommissionMinor  int64 `json:"commission_liability_minor"`
	RefundMinor      int64 `json:"refund_minor"`
	GrossProfitMinor int64 `json:"gross_profit_minor"`
	PendingCount     int64 `json:"pending_reconciliation_count"`
}

type DimMoneyView struct {
	Dimension        string `json:"dimension"`
	Key              string `json:"key"`
	Requests         int64  `json:"requests"`
	UsageMinor       int64  `json:"usage_minor"`
	RevenueMinor     int64  `json:"revenue_minor"`
	CostMinor        int64  `json:"cost_minor"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	VideoSeconds     int64  `json:"video_seconds"`
	ImageCount       int64  `json:"image_count"`
	AudioSeconds     int64  `json:"audio_seconds"`
}

type UsageUnits struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
	VideoSeconds     int64 `json:"video_seconds"`
	ImageCount       int64 `json:"image_count"`
	AudioSeconds     int64 `json:"audio_seconds"`
	UsageMinor       int64 `json:"usage_minor,omitempty"`
}

type RiskView struct {
	LowBalanceWallets int64 `json:"low_balance_wallets"`
	ReservedMinor     int64 `json:"wallet_reserved_minor"`
	ChannelSpendMinor int64 `json:"channel_spend_minor"`
	PreauthFailed     int64 `json:"preauth_failed"`
}

type QuotaView struct {
	OwnerID         string `json:"owner_id"`
	AvailableMinor  int64  `json:"available_minor"`
	ReservedMinor   int64  `json:"reserved_minor"`
	IssuedMinor     int64  `json:"issued_minor,omitempty"`
	ConsumedMinor   int64  `json:"consumed_minor,omitempty"`
	AllocationCount int64  `json:"allocation_count,omitempty"`
	IssueRatioBPS   int64  `json:"issue_ratio_bps"`
	UnitType        string `json:"unit_type"`
}

// IssueRuleView 是渠道“充值金额 -> 服务额度”换算比。10000 BPS = 1:1。
type IssueRuleView struct {
	ChannelOrgID  string    `json:"channel_org_id"`
	IssueRatioBPS int64     `json:"issue_ratio_bps"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type AllocationView struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	ChannelOrgID   string    `json:"channel_org_id"`
	SourceType     string    `json:"source_type"`
	SourceID       string    `json:"source_id"`
	GrantedMinor   int64     `json:"granted_minor"`
	ConsumedMinor  int64     `json:"consumed_minor"`
	RemainingMinor int64     `json:"remaining_minor"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type CommissionView struct {
	ID            string `json:"id"`
	UsageEventID  string `json:"usage_event_id"`
	AmountMinor   int64  `json:"amount_minor"`
	Status        string `json:"status"`
	PolicyVersion string `json:"policy_version"`
}
