package ops

import (
	"errors"
	"time"
)

var (
	ErrRateLimited = errors.New("rate limited")
	ErrNotFound    = errors.New("ops record not found")
	ErrInvalid     = errors.New("invalid ops request")
)

const (
	DimProvider = "provider"
	DimModel    = "model"
	DimChannel  = "channel"
	DimUser     = "user"
	DimAPIKey   = "api_key"
	DimAgent    = "agent"

	AlertPending    = "pending_reconciliation"
	AlertCircuit    = "provider_circuit_open"
	AlertLowSuccess = "low_success_rate"
	AlertBackup     = "backup_drill_missing"

	StatusOpen     = "open"
	StatusResolved = "resolved"
	SeverityHigh   = "high"
	SeverityMed    = "medium"

	DefaultRPM       = 60
	CircuitThreshold = 3
	RPOMinutes       = 15
	RTOMinutes       = 60
	CanaryChat       = "chat"
	PolicyVersion    = "m7-v1"
)

type DimStat struct {
	Dimension    string  `json:"dimension"`
	Key          string  `json:"key"`
	Requests     int64   `json:"requests"`
	Successes    int64   `json:"successes"`
	Errors       int64   `json:"errors"`
	SuccessRate  float64 `json:"success_rate"`
	LatencyP50MS int64   `json:"latency_p50_ms"`
	LatencyP95MS int64   `json:"latency_p95_ms"`
	Fallbacks    int64   `json:"fallbacks"`
	UsageMinor   int64   `json:"usage_minor,omitempty"`
	RevenueMinor int64   `json:"revenue_minor,omitempty"`
	CostMinor    int64   `json:"cost_minor,omitempty"`
	MarginMinor  int64   `json:"gross_profit_minor,omitempty"`
}

type MoneyView struct {
	RevenueMinor     int64 `json:"revenue_minor"`
	UpstreamMinor    int64 `json:"upstream_cost_minor"`
	WholesaleMinor   int64 `json:"wholesale_minor"`
	CommissionMinor  int64 `json:"commission_liability_minor"`
	RefundMinor      int64 `json:"refund_minor"`
	GrossProfitMinor int64 `json:"gross_profit_minor"`
	PendingCount     int64 `json:"pending_reconciliation_count"`
}

type Dashboard struct {
	Version    string               `json:"version"`
	Totals     MoneyView            `json:"totals"`
	Dimensions map[string][]DimStat `json:"dimensions"`
	Alerts     []AlertView          `json:"alerts"`
	Canary     *CanaryView          `json:"canary,omitempty"`
	LastDrill  *DrillView           `json:"last_backup_drill,omitempty"`
	Runbooks   []RunbookView        `json:"runbooks"`
}

type AlertView struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type RunbookView struct {
	ID        string `json:"id"`
	AlertKind string `json:"alert_kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

type DrillView struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	RPOMinutes int       `json:"rpo_minutes"`
	RTOMinutes int       `json:"rto_minutes"`
	Method     string    `json:"method"`
	Evidence   string    `json:"evidence"`
	CreatedAt  time.Time `json:"created_at"`
}

type CanaryView struct {
	RouteKey     string `json:"route_key"`
	ProviderSlug string `json:"provider_slug"`
	Percent      int    `json:"percent"`
}

type DrillResult struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Passed bool   `json:"passed"`
}
