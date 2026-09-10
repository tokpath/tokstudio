package plans

import (
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
)

var (
	ErrNotFound     = errors.New("plan not found")
	ErrInvalidPlan  = errors.New("invalid plan")
	ErrNotPublished = errors.New("plan is not published")
	ErrNotPending   = errors.New("plan is not pending review")
)

const (
	OwnerPlatform = "platform"
	OwnerChannel  = "channel"

	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusPublished     = "published"
	StatusRejected      = "rejected"
	StatusArchived      = "archived"

	SubPending           = "pending"
	SubActive            = "active"
	SubPastDue           = "past_due"
	SubCancelAtPeriodEnd = "cancel_at_period_end"
	SubCancelled         = "cancelled"

	RenewAuto   = "auto"
	RenewManual = "manual"

	UnitUSDCredit   = "usd_credit"
	UnitToken       = "token"
	UnitVideoSecond = "video_second"
	UnitImageCount  = "image_count"

	SourceBonus = "bonus"
	SourcePlan  = "plan"

	EntActive    = "active"
	EntExhausted = "exhausted"
	EntExpired   = "expired"
	EntReversed  = "reversed"

	EventGrant    = "plan_grant"
	EventBonus    = "bonus_grant"
	EventDebit    = "usage_debit"
	EventExpiry   = "expiry"
	EventReversal = "reversal"

	PeriodMonthly = "monthly"
	PriceFloor    = billing.MinorPerUSD // 低于 1 USD 的渠道套餐进人工审核
	GraceDays     = 7
)

type PlanItemInput struct {
	PublicModelID string `json:"public_model_id"`
	UnitType      string `json:"unit_type"`
	Included      int64  `json:"included_amount"`
	OverageMinor  int64  `json:"overage_price_minor"`
	ExpiresIn     int    `json:"expires_in_seconds"`
}

type CreatePlanInput struct {
	OwnerType     string          `json:"owner_type"`
	OwnerID       string          `json:"owner_id"`
	Name          string          `json:"name"`
	PriceMinor    int64           `json:"price_minor"`
	Currency      string          `json:"currency"`
	BillingPeriod string          `json:"billing_period"`
	AutoRenew     bool            `json:"auto_renew_allowed"`
	Items         []PlanItemInput `json:"items"`
}

type PlanView struct {
	ID               string          `json:"id"`
	OwnerType        string          `json:"owner_type"`
	OwnerID          string          `json:"owner_id"`
	Name             string          `json:"name"`
	PriceMinor       int64           `json:"price_minor"`
	Currency         string          `json:"currency"`
	BillingPeriod    string          `json:"billing_period"`
	Status           string          `json:"status"`
	ReviewReason     string          `json:"review_reason,omitempty"`
	AutoRenewAllowed bool            `json:"auto_renew_allowed"`
	Items            []PlanItemInput `json:"items"`
}

type SubscriptionView struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	ChannelOrgID  string     `json:"channel_org_id,omitempty"`
	PlanID        string     `json:"plan_id"`
	Status        string     `json:"status"`
	PeriodStart   *time.Time `json:"current_period_start,omitempty"`
	PeriodEnd     *time.Time `json:"current_period_end,omitempty"`
	RenewalPolicy string     `json:"renewal_policy"`
	Adapter       string     `json:"payment_adapter,omitempty"`
	GraceUntil    *time.Time `json:"grace_until,omitempty"`
}

type EntitlementView struct {
	ID            string     `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      string     `json:"source_id"`
	UnitType      string     `json:"unit_type"`
	Granted       int64      `json:"granted"`
	Consumed      int64      `json:"consumed"`
	Remaining     int64      `json:"remaining"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Status        string     `json:"status"`
	PublicModelID string     `json:"public_model_id,omitempty"`
}
