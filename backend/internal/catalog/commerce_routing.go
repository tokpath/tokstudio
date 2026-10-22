package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRouteUnavailable = errors.New("route capacity or verified health unavailable")

// Zero values select conservative defaults; zero concurrency means no extra
// platform capacity limit, without bypassing provider or customer limits.
type RouteLimits struct {
	MaxConcurrent         int `json:"max_concurrent"`
	FailureThreshold      int `json:"failure_threshold"`
	UnknownUsageThreshold int `json:"unknown_usage_threshold"`
	RecoverySuccesses     int `json:"recovery_successes"`
	CooldownSeconds       int `json:"cooldown_seconds"`
}

func (r RouteLimits) valid() bool {
	return r.MaxConcurrent >= 0 && r.MaxConcurrent <= 100000 && r.FailureThreshold >= 0 && r.FailureThreshold <= 100 && r.UnknownUsageThreshold >= 0 && r.UnknownUsageThreshold <= 100 && r.RecoverySuccesses >= 0 && r.RecoverySuccesses <= 20 && r.CooldownSeconds >= 0 && r.CooldownSeconds <= 86400
}
func (r RouteLimits) defaults() RouteLimits {
	if r.FailureThreshold == 0 {
		r.FailureThreshold = 3
	}
	if r.UnknownUsageThreshold == 0 {
		r.UnknownUsageThreshold = 2
	}
	if r.RecoverySuccesses == 0 {
		r.RecoverySuccesses = 2
	}
	if r.CooldownSeconds == 0 {
		r.CooldownSeconds = 60
	}
	return r
}

type RoutingPolicy struct {
	ModelID  string `json:"model_id" gorm:"primaryKey"`
	Strategy string `json:"strategy"`
	Evidence string `json:"evidence"`
	Version  uint64 `json:"version"`
}

func (RoutingPolicy) TableName() string { return "tokenhub_route_policies" }

type RouteHealth struct {
	UpstreamID        string     `json:"upstream_id" gorm:"primaryKey"`
	Circuit           string     `json:"circuit"`
	Epoch             int64      `json:"epoch"`
	Failures          int        `json:"failures"`
	UnknownUsage      int        `json:"unknown_usage"`
	RecoverySuccesses int        `json:"recovery_successes"`
	LatencyMicros     int64      `json:"latency_micros"`
	OpenUntil         *time.Time `json:"open_until,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (RouteHealth) TableName() string { return "tokenhub_route_health" }

type routeLease struct {
	AttemptID             string `gorm:"primaryKey"`
	UpstreamID, RequestID string
	Epoch                 int64
	Probe                 bool
	Limits                json.RawMessage `gorm:"type:jsonb"`
	StartedAt, ExpiresAt  time.Time
	FinishedAt            *time.Time
}

func (routeLease) TableName() string { return "tokenhub_route_leases" }

type RoutingView struct {
	Policy RoutingPolicy `json:"policy"`
	Health []RouteHealth `json:"health"`
}

func (s CommerceCatalog) RoutingPolicy(ctx context.Context, model string) (RoutingPolicy, error) {
	var p RoutingPolicy
	e := s.DB.WithContext(ctx).First(&p, "model_id=?", model).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return RoutingPolicy{ModelID: model, Strategy: "priority"}, nil
	}
	return p, e
}
func (s CommerceCatalog) SaveRoutingPolicy(ctx context.Context, actor string, p RoutingPolicy, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	if p.Evidence == "" {
		p.Evidence = "路由配置"
	}
	if len(p.Evidence) > 2000 || !validRouteStrategy(p.Strategy) {
		return ErrInvalidInput
	}
	return s.mutate(ctx, func(c CommerceCatalog) error { return c.writeRoutingPolicy(ctx, p, expected) })
}
func validRouteStrategy(strategy string) bool {
	return strategy == "priority" || strategy == "lowest_latency" || strategy == "lowest_cost"
}
func (s CommerceCatalog) writeRoutingPolicy(ctx context.Context, p RoutingPolicy, expected uint64) error {
	if _, e := s.PublicModel(ctx, p.ModelID); e != nil {
		return e
	}
	p.Version = expected + 1
	if expected == 0 {
		if e := s.DB.WithContext(ctx).Create(&p).Error; e != nil {
			return e
		}
	} else {
		r := s.DB.WithContext(ctx).Model(&RoutingPolicy{}).Where("model_id=? AND version=?", p.ModelID, expected).Updates(map[string]any{"strategy": p.Strategy, "evidence": p.Evidence, "version": p.Version})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrCatalogVersion
		}
	}
	b, _ := json.Marshal(p)
	return s.DB.WithContext(ctx).Exec("INSERT INTO tokenhub_route_events(id,upstream_id,kind,reference,data) VALUES(?,?,?,?,?::jsonb)", uuid.NewString(), "", "policy_changed", uuid.NewString(), string(b)).Error
}
func (s CommerceCatalog) AdminRouting(ctx context.Context, actor, model string) (RoutingView, error) {
	if !s.platform(actor) {
		return RoutingView{}, ErrCatalogForbidden
	}
	p, e := s.RoutingPolicy(ctx, model)
	if e != nil {
		return RoutingView{}, e
	}
	h, _, e := s.routeHealth(ctx, model)
	if e != nil {
		return RoutingView{}, e
	}
	rows := make([]RouteHealth, 0, len(h))
	for _, v := range h {
		rows = append(rows, v)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpstreamID < rows[j].UpstreamID })
	return RoutingView{Policy: p, Health: rows}, nil
}
func (s CommerceCatalog) RouteHealth(ctx context.Context, model string) (map[string]RouteHealth, time.Time, error) {
	return s.routeHealth(ctx, model)
}
func (s CommerceCatalog) routeHealth(ctx context.Context, model string) (map[string]RouteHealth, time.Time, error) {
	var rows []RouteHealth
	var now time.Time
	e := s.DB.WithContext(ctx).Raw("SELECT clock_timestamp()").Row().Scan(&now)
	if e != nil {
		return nil, now, e
	}
	e = s.DB.WithContext(ctx).Table("tokenhub_route_health h").Select("h.*").Joins("JOIN tokenhub_upstreams u ON u.id=h.upstream_id").Where("u.public_model_id=?", model).Find(&rows).Error
	out := map[string]RouteHealth{}
	for _, r := range rows {
		out[r.UpstreamID] = r
	}
	return out, now, e
}
func (s CommerceCatalog) rankRoutes(ctx context.Context, q *ModelQuote) error {
	p, e := s.RoutingPolicy(ctx, q.ModelID)
	if e != nil {
		return e
	}
	if p.Strategy == "priority" {
		return nil
	}
	health, _, e := s.routeHealth(ctx, q.ModelID)
	if e != nil {
		return e
	}
	costs := map[string]*big.Rat{}
	if p.Strategy == "lowest_cost" {
		for _, p := range q.Supplier {
			v, e := p.Price.MaximumCharge(q.Bounds)
			if e != nil {
				return e
			}
			costs[p.UpstreamID] = v
		}
	}
	sort.SliceStable(q.Candidates, func(i, j int) bool {
		a, b := q.Candidates[i], q.Candidates[j]
		if p.Strategy == "lowest_cost" {
			if n := costs[a.ID].Cmp(costs[b.ID]); n != 0 {
				return n < 0
			}
		}
		if p.Strategy == "lowest_latency" {
			x, y := health[a.ID].LatencyMicros, health[b.ID].LatencyMicros
			// Unmeasured routes keep configured priority instead of pretending 0 ms.
			if x > 0 && y > 0 && x != y {
				return x < y
			}
			if (x > 0) != (y > 0) {
				return x > 0
			}
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
	return nil
}

// ClaimRoute is called in the same transaction that writes dispatch intent.
// A circuit allows one verification request at a time after its cooldown; an
// old accepted request never gains routes or changes price snapshots here.
func (s CommerceCatalog) ClaimRoute(ctx context.Context, upstream, attempt, request string, deadline time.Time) error {
	return s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		var row catalogJSONRow
		var u UpstreamModel
		if e := t.Table("tokenhub_upstreams").First(&row, "id=?", upstream).Error; e != nil {
			return e
		}
		if e := json.Unmarshal(row.Definition, &u); e != nil {
			return e
		}
		if !u.Enabled || !u.Healthy {
			return ErrRouteUnavailable
		}
		if e := t.Clauses(clause.OnConflict{DoNothing: true}).Create(&RouteHealth{UpstreamID: upstream, Circuit: "closed"}).Error; e != nil {
			return e
		}
		var h RouteHealth
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&h, "upstream_id=?", upstream).Error; e != nil {
			return e
		}
		var now time.Time
		if e := t.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
			return e
		}
		if !deadline.After(now) || (h.OpenUntil != nil && h.OpenUntil.After(now)) {
			return ErrRouteUnavailable
		}
		var old routeLease
		e := t.First(&old, "attempt_id=?", attempt).Error
		if e == nil {
			if old.UpstreamID != upstream || old.RequestID != request || old.FinishedAt != nil {
				return ErrRouteUnavailable
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		limits := u.Routing.defaults()
		var active int64
		if e = t.Model(&routeLease{}).Where("upstream_id=? AND finished_at IS NULL AND expires_at>?", upstream, now).Count(&active).Error; e != nil {
			return e
		}
		probe := h.Circuit != "closed"
		if (limits.MaxConcurrent > 0 && active >= int64(limits.MaxConcurrent)) || (probe && active > 0) {
			return ErrRouteUnavailable
		}
		b, _ := json.Marshal(limits)
		return t.Create(&routeLease{AttemptID: attempt, UpstreamID: upstream, RequestID: request, Epoch: h.Epoch, Probe: probe, Limits: b, StartedAt: now, ExpiresAt: deadline}).Error
	})
}

// ObserveRoute accepts runtime facts only. Idempotent observations and epoch
// fencing stop late successes from reopening a circuit tripped by newer faults.
func (s CommerceCatalog) ObserveRoute(ctx context.Context, attempt string, result modelruntime.Result) error {
	if result.State == "submitted" {
		return nil
	}
	return s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		var lease routeLease
		e := t.First(&lease, "attempt_id=?", attempt).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		var h RouteHealth
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&h, "upstream_id=?", lease.UpstreamID).Error; e != nil {
			return e
		}
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lease, "attempt_id=?", attempt).Error; e != nil {
			return e
		}
		if lease.FinishedAt != nil {
			return nil
		}
		var now time.Time
		if e = t.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
			return e
		}
		good := result.State == "completed" && result.Usage.Reliable
		unknown := (result.State == "completed" || result.State == "outcome_unknown" || result.ErrorCode == "usage_unverified_before_deadline") && !result.Usage.Reliable
		var limits RouteLimits
		if e = json.Unmarshal(lease.Limits, &limits); e != nil {
			return e
		}
		latency := now.Sub(lease.StartedAt).Microseconds()
		if latency < 0 {
			latency = 0
		}
		previous := h.Circuit
		if lease.Epoch == h.Epoch {
			if good {
				h.Failures = 0
				h.UnknownUsage = 0
				if h.LatencyMicros == 0 {
					h.LatencyMicros = latency
				} else {
					h.LatencyMicros = (h.LatencyMicros*7 + latency) / 8
				}
				if lease.Probe {
					h.RecoverySuccesses++
					if h.RecoverySuccesses >= limits.RecoverySuccesses {
						h.Circuit = "closed"
						h.OpenUntil = nil
						h.RecoverySuccesses = 0
					}
				}
			} else {
				h.Failures++
				if unknown {
					h.UnknownUsage++
				}
				h.RecoverySuccesses = 0
				if lease.Probe || h.Failures >= limits.FailureThreshold || h.UnknownUsage >= limits.UnknownUsageThreshold {
					h.Circuit = "open"
					until := now.Add(time.Duration(limits.CooldownSeconds) * time.Second)
					h.OpenUntil = &until
					h.Epoch++
				}
			}
		}
		h.UpdatedAt = now
		if e = t.Save(&h).Error; e != nil {
			return e
		}
		if e = t.Model(&routeLease{}).Where("attempt_id=?", attempt).Update("finished_at", now).Error; e != nil {
			return e
		}
		data, _ := json.Marshal(map[string]any{"reliable": result.Usage.Reliable, "successful": good, "unknown_usage": unknown, "previous_circuit": previous, "circuit": h.Circuit, "epoch": h.Epoch, "latency_micros": latency})
		return t.Exec("INSERT INTO tokenhub_route_events(id,upstream_id,kind,reference,data) VALUES(?,?,?,?,?::jsonb)", uuid.NewString(), lease.UpstreamID, "attempt_observed", attempt, string(data)).Error
	})
}
