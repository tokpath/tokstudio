package commission

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("commission record not found")
	ErrInvalid      = errors.New("invalid commission request")
	ErrBelowMinimum = errors.New("below minimum settlement")
)

const (
	KindDirect   = "direct"
	KindIndirect = "indirect"
	KindOverride = "override"
	KindChannel  = "channel"
	KindTeam     = "team"

	StatusFrozen    = "frozen"
	StatusAvailable = "available"
	StatusHeld      = "held"
	StatusSettled   = "settled"
	StatusPaid      = "paid"
	StatusReversed  = "reversed"

	PolicyM6      = "m6-v1"
	DefaultDirect   = 1500
	DefaultIndirect = 500
	DefaultOver     = 0
	DefaultChan     = 0
	DefaultTeam     = 0
	DefaultCap      = 2000
	DefaultTotal    = 2000
	FreezeDays    = 7
)

type AccrueInput struct {
	UsageEventID   string
	RequestID      string
	UserID         string
	ChannelOrgID   string
	WholesaleMinor int64
	RoleID         string
	RoleType       string
	ParentRoleID   string
	CanCommission  bool
}

type EntryView struct {
	ID                string     `json:"id"`
	UsageEventID      string     `json:"usage_event_id"`
	RequestID         string     `json:"request_id,omitempty"`
	Kind              string     `json:"kind"`
	BeneficiaryRoleID string     `json:"beneficiary_role_id,omitempty"`
	ChannelOrgID      string     `json:"channel_org_id,omitempty"`
	AmountMinor       int64      `json:"amount_minor"`
	RawAmountMinor    int64      `json:"raw_amount_minor"`
	Status            string     `json:"status"`
	PolicyVersion     string     `json:"policy_version"`
	AvailableAt       *time.Time `json:"available_at,omitempty"`
	SettlementID      string     `json:"settlement_id,omitempty"`
}

type SettlementView struct {
	ID                string    `json:"id"`
	PeriodStart       time.Time `json:"period_start"`
	PeriodEnd         time.Time `json:"period_end"`
	ChannelOrgID      string    `json:"channel_org_id,omitempty"`
	BeneficiaryRoleID string    `json:"beneficiary_role_id,omitempty"`
	AmountMinor       int64     `json:"amount_minor"`
	Status            string    `json:"status"`
	PolicyVersion     string    `json:"policy_version"`
}

type PolicyView struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	DirectBPS      int    `json:"direct_bps"`
	IndirectBPS    int    `json:"indirect_bps"`
	OverrideBPS    int    `json:"override_bps,omitempty"`
	ChannelBPS     int    `json:"channel_bps,omitempty"`
	TeamBPS        int    `json:"team_bps,omitempty"`
	CapBPS         int    `json:"cap_bps"`
	TotalBPS       int    `json:"total_bps"`
	FreezeDays     int    `json:"freeze_days"`
	MinSettleMinor int64  `json:"min_settle_minor"`
}
