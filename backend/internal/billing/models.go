package billing

import "time"

type walletRow struct {
	ID                       string    `gorm:"column:id;primaryKey"`
	UserID                   string    `gorm:"column:user_id"`
	Currency                 string    `gorm:"column:currency"`
	AvailableMinor           int64     `gorm:"column:available_minor"`
	GiftMinor                int64     `gorm:"column:gift_minor"`
	CommissionAvailableMinor int64     `gorm:"column:commission_available_minor"`
	ReservedMinor            int64     `gorm:"column:reserved_minor"`
	Version                  int64     `gorm:"column:version"`
	CreatedAt                time.Time `gorm:"column:created_at"`
	UpdatedAt                time.Time `gorm:"column:updated_at"`
}

func (walletRow) TableName() string { return "billing_wallets" }

type ledgerRow struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	WalletID           string    `gorm:"column:wallet_id"`
	EventType          string    `gorm:"column:event_type"`
	AmountMinor        int64     `gorm:"column:amount_minor"`
	Currency           string    `gorm:"column:currency"`
	BalanceAfterMinor  int64     `gorm:"column:balance_after_minor"`
	ReservedAfterMinor int64     `gorm:"column:reserved_after_minor"`
	ReferenceType      string    `gorm:"column:reference_type"`
	ReferenceID        string    `gorm:"column:reference_id"`
	IdempotencyKey     string    `gorm:"column:idempotency_key"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

func (ledgerRow) TableName() string { return "billing_ledger" }

type quotaRow struct {
	ID             string `gorm:"column:id;primaryKey"`
	OwnerType      string `gorm:"column:owner_type"`
	OwnerID        string `gorm:"column:owner_id"`
	UnitType       string `gorm:"column:unit_type"`
	AvailableMinor int64  `gorm:"column:available_minor"`
	ReservedMinor  int64  `gorm:"column:reserved_minor"`
	Version        int64  `gorm:"column:version"`
}

func (quotaRow) TableName() string { return "billing_quota_accounts" }

type issueRuleRow struct {
	ID            string    `gorm:"column:id;primaryKey"`
	ChannelOrgID  string    `gorm:"column:channel_org_id"`
	IssueRatioBPS int64     `gorm:"column:issue_ratio_bps"`
	Version       int64     `gorm:"column:version"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (issueRuleRow) TableName() string { return "billing_quota_issue_rules" }

type quotaLedgerRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	AccountID      string    `gorm:"column:account_id"`
	EventType      string    `gorm:"column:event_type"`
	AmountMinor    int64     `gorm:"column:amount_minor"`
	ReferenceType  string    `gorm:"column:reference_type"`
	ReferenceID    string    `gorm:"column:reference_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (quotaLedgerRow) TableName() string { return "billing_quota_ledger" }

type allocationRow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	UserID        string     `gorm:"column:user_id"`
	ChannelOrgID  string     `gorm:"column:channel_org_id"`
	SourceType    string     `gorm:"column:source_type"`
	SourceID      string     `gorm:"column:source_id"`
	GrantedMinor  int64      `gorm:"column:granted_minor"`
	ConsumedMinor int64      `gorm:"column:consumed_minor"`
	Status        string     `gorm:"column:status"`
	ExpiresAt     *time.Time `gorm:"column:expires_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

func (allocationRow) TableName() string { return "billing_quota_allocations" }

type consumeRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	AllocationID   string    `gorm:"column:allocation_id"`
	RequestID      string    `gorm:"column:request_id"`
	AmountMinor    int64     `gorm:"column:amount_minor"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (consumeRow) TableName() string { return "billing_quota_consumes" }

type topupRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	UserID          string    `gorm:"column:user_id"`
	ChannelOrgID    *string   `gorm:"column:channel_org_id"`
	AmountMinor     int64     `gorm:"column:amount_minor"`
	Currency        string    `gorm:"column:currency"`
	PaymentMethod   string    `gorm:"column:payment_method"`
	Status          string    `gorm:"column:status"`
	ProviderTradeID *string   `gorm:"column:provider_trade_id"`
	RedeemCode      *string   `gorm:"column:redeem_code"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (topupRow) TableName() string { return "billing_topups" }

type redeemRow struct {
	ID             string `gorm:"column:id;primaryKey"`
	Code           string `gorm:"column:code"`
	AmountMinor    int64  `gorm:"column:amount_minor"`
	Currency       string `gorm:"column:currency"`
	MaxRedemptions int    `gorm:"column:max_redemptions"`
	RedeemedCount  int    `gorm:"column:redeemed_count"`
	Status         string `gorm:"column:status"`
}

func (redeemRow) TableName() string { return "billing_redeem_codes" }

type authRow struct {
	ID                  string    `gorm:"column:id;primaryKey"`
	WalletID            string    `gorm:"column:wallet_id"`
	UserID              string    `gorm:"column:user_id"`
	ChannelOrgID        *string   `gorm:"column:channel_org_id"`
	RequestID           string    `gorm:"column:request_id"`
	AmountMinor         int64     `gorm:"column:amount_minor"`
	WalletReservedMinor int64     `gorm:"column:wallet_reserved_minor"`
	GiftReservedMinor   int64     `gorm:"column:gift_reserved_minor"`
	GiftSettledMinor    int64     `gorm:"column:gift_settled_minor"`
	SettledMinor        int64     `gorm:"column:settled_minor"`
	Currency            string    `gorm:"column:currency"`
	Status              string    `gorm:"column:status"`
	PriceVersionID      *string   `gorm:"column:price_version_id"`
	UnitPrices          []byte    `gorm:"column:unit_prices_json"`
	ExpiresAt           time.Time `gorm:"column:expires_at"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (authRow) TableName() string { return "billing_authorizations" }

type usageRow struct {
	ID                   string    `gorm:"column:id;primaryKey"`
	RequestID            string    `gorm:"column:request_id"`
	AttemptID            *string   `gorm:"column:attempt_id"`
	UserID               string    `gorm:"column:user_id"`
	APIKeyID             *string   `gorm:"column:api_key_id"`
	ChannelOrgID         *string   `gorm:"column:channel_org_id"`
	PublicModelID        string    `gorm:"column:public_model_id"`
	ProviderID           *string   `gorm:"column:provider_id"`
	UpstreamModelID      *string   `gorm:"column:upstream_model_id"`
	UnitUsage            []byte    `gorm:"column:unit_usage_json"`
	UnitPrices           []byte    `gorm:"column:unit_prices_json"`
	PriceVersionID       *string   `gorm:"column:price_version_id"`
	CustomerAmountMinor  int64     `gorm:"column:customer_amount_minor"`
	UpstreamCostMinor    int64     `gorm:"column:upstream_cost_minor"`
	WholesaleAmountMinor int64     `gorm:"column:wholesale_amount_minor"`
	Currency             string    `gorm:"column:currency"`
	State                string    `gorm:"column:state"`
	FactSource           *string   `gorm:"column:fact_source"`
	IdempotencyKey       string    `gorm:"column:idempotency_key"`
	OccurredAt           time.Time `gorm:"column:occurred_at"`
}

func (usageRow) TableName() string { return "billing_usage_events" }

type chargeRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	RequestID       string    `gorm:"column:request_id"`
	UsageEventID    string    `gorm:"column:usage_event_id"`
	AuthorizationID *string   `gorm:"column:authorization_id"`
	AmountMinor     int64     `gorm:"column:amount_minor"`
	PriceVersionID  *string   `gorm:"column:price_version_id"`
	Status          string    `gorm:"column:status"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (chargeRow) TableName() string { return "billing_customer_charges" }

type costRow struct {
	ID          string    `gorm:"column:id;primaryKey"`
	RequestID   string    `gorm:"column:request_id"`
	AttemptID   string    `gorm:"column:attempt_id"`
	ProviderID  string    `gorm:"column:provider_id"`
	AmountMinor int64     `gorm:"column:amount_minor"`
	Currency    string    `gorm:"column:currency"`
	UnitUsage   []byte    `gorm:"column:unit_usage_json"`
	UnitPrices  []byte    `gorm:"column:unit_prices_json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (costRow) TableName() string { return "billing_cost_entries" }

type commissionRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	UsageEventID    string    `gorm:"column:usage_event_id"`
	ChannelOrgID    *string   `gorm:"column:channel_org_id"`
	PolicyVersion   string    `gorm:"column:policy_version"`
	BaseAmountMinor int64     `gorm:"column:base_amount_minor"`
	AmountMinor     int64     `gorm:"column:amount_minor"`
	Status          string    `gorm:"column:status"`
	ReversalOf      *string   `gorm:"column:reversal_of"`
	IdempotencyKey  string    `gorm:"column:idempotency_key"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (commissionRow) TableName() string { return "billing_commission_ledger" }
