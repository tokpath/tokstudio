package ops

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type TrafficSource interface {
	DimStats(ctx context.Context, dimension string) ([]DimStat, error)
}

type MoneySource interface {
	Money(ctx context.Context) (*MoneyView, error)
	DimMoney(ctx context.Context, dimension string) ([]DimStat, error)
}

type HealthSink interface {
	MarkHealth(ctx context.Context, providerID, health string) error
}

type RoleSource interface {
	MapUserRoles(ctx context.Context, userIDs []string) (map[string]string, error)
}

type runbookRow struct {
	ID        string    `gorm:"column:id;primaryKey"`
	AlertKind string    `gorm:"column:alert_kind"`
	Title     string    `gorm:"column:title"`
	Body      string    `gorm:"column:body"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (runbookRow) TableName() string { return "ops_runbooks" }

type alertRow struct {
	ID          string          `gorm:"column:id;primaryKey"`
	Kind        string          `gorm:"column:kind"`
	Severity    string          `gorm:"column:severity"`
	Status      string          `gorm:"column:status"`
	Message     string          `gorm:"column:message"`
	PayloadJSON json.RawMessage `gorm:"column:payload_json"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	ResolvedAt  *time.Time      `gorm:"column:resolved_at"`
}

func (alertRow) TableName() string { return "ops_alerts" }

type drillRow struct {
	ID         string    `gorm:"column:id;primaryKey"`
	Status     string    `gorm:"column:status"`
	RPOMinutes int       `gorm:"column:rpo_minutes"`
	RTOMinutes int       `gorm:"column:rto_minutes"`
	Method     string    `gorm:"column:method"`
	Evidence   string    `gorm:"column:evidence"`
	Actor      *string   `gorm:"column:actor_user_id"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (drillRow) TableName() string { return "ops_backup_drills" }

type canaryRow struct {
	ID           string    `gorm:"column:id;primaryKey"`
	RouteKey     string    `gorm:"column:route_key"`
	ProviderSlug string    `gorm:"column:provider_slug"`
	Percent      int       `gorm:"column:percent"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (canaryRow) TableName() string { return "ops_canary" }

type Service struct {
	db      *gorm.DB
	redis   *redis.Client
	traffic TrafficSource
	money   MoneySource
	health  HealthSink
	roles   RoleSource
}

func New(db *gorm.DB, rdb *redis.Client) *Service {
	return &Service{db: db, redis: rdb}
}

func (s *Service) SetSources(traffic TrafficSource, money MoneySource, health HealthSink, roles RoleSource) {
	s.traffic = traffic
	s.money = money
	s.health = health
	s.roles = roles
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "ops", sub
}

func (s *Service) Seed(ctx context.Context) error {
	now := time.Now().UTC()
	books := []runbookRow{
		{ID: "rb_pending", AlertKind: AlertPending, Title: "待对账 usage", Body: "1. GET /admin/usage 找 pending_reconciliation\n2. POST /admin/usage/replay 按 request_id 回放\n3. 禁止按估算扣款。", CreatedAt: now},
		{ID: "rb_circuit", AlertKind: AlertCircuit, Title: "Provider 熔断", Body: "1. GET /admin/providers 看 health\n2. POST /admin/providers/:id/health-check 探测\n3. 连续成功或 POST /admin/ops/circuit/:id reset 后恢复。", CreatedAt: now},
		{ID: "rb_backup", AlertKind: AlertBackup, Title: "备份演练", Body: "1. 运行 scripts/backup_drill.sh\n2. 目标 RPO ≤ 15 分钟、RTO ≤ 1 小时\n3. Redis 不是账务事实源，丢失后只重建限流。", CreatedAt: now},
		{ID: "rb_payment", AlertKind: "payment_chaos", Title: "支付异常演练", Body: "伪造签名必须 401；合法 webhook 按 event_id 幂等。", CreatedAt: now},
		{ID: "rb_media", AlertKind: "media_chaos", Title: "媒体异常演练", Body: "失败任务必须释放预授权；回调按 event_id 幂等。", CreatedAt: now},
	}
	for i := range books {
		if err := s.db.WithContext(ctx).Where("id = ?", books[i].ID).FirstOrCreate(&books[i]).Error; err != nil {
			return err
		}
	}
	canary := canaryRow{ID: "cny_chat", RouteKey: CanaryChat, ProviderSlug: "echo-backup", Percent: 0, UpdatedAt: now}
	return s.db.WithContext(ctx).Where("route_key = ?", CanaryChat).FirstOrCreate(&canary).Error
}

func (s *Service) Dashboard(ctx context.Context) (*Dashboard, error) {
	out := &Dashboard{Version: PolicyVersion, Dimensions: map[string][]DimStat{}}
	if s.money != nil {
		if money, err := s.money.Money(ctx); err == nil && money != nil {
			out.Totals = *money
		}
	}
	for _, dim := range []string{DimProvider, DimModel, DimChannel, DimUser, DimAPIKey, DimAgent} {
		stats := []DimStat{}
		if dim == DimAgent {
			stats = s.agentStats(ctx)
		} else {
			if s.traffic != nil {
				if rows, err := s.traffic.DimStats(ctx, dim); err == nil {
					stats = rows
				}
			}
			if s.money != nil {
				if money, err := s.money.DimMoney(ctx, dim); err == nil {
					stats = mergeMoney(stats, money)
				}
			}
		}
		out.Dimensions[dim] = stats
	}
	fillOverview(&out.Totals, out.Dimensions[DimProvider])
	out.Alerts, _ = s.ListAlerts(ctx, StatusOpen)
	out.Canary, _ = s.Canary(ctx, CanaryChat)
	out.LastDrill, _ = s.LastDrill(ctx)
	out.Runbooks, _ = s.ListRunbooks(ctx)
	return out, nil
}

func mergeMoney(traffic, money []DimStat) []DimStat {
	byKey := map[string]DimStat{}
	for _, row := range traffic {
		byKey[row.Key] = row
	}
	for _, row := range money {
		cur := byKey[row.Key]
		cur.Dimension = firstNonEmpty(cur.Dimension, row.Dimension)
		cur.Key = row.Key
		cur.UsageMinor = row.UsageMinor
		cur.RevenueMinor = row.RevenueMinor
		cur.CostMinor = row.CostMinor
		cur.MarginMinor = row.RevenueMinor - row.CostMinor
		byKey[row.Key] = cur
	}
	out := make([]DimStat, 0, len(byKey))
	for _, row := range byKey {
		out = append(out, row)
	}
	return out
}

func (s *Service) agentStats(ctx context.Context) []DimStat {
	var users []DimStat
	if s.traffic != nil {
		users, _ = s.traffic.DimStats(ctx, DimUser)
	}
	if s.money != nil {
		if money, err := s.money.DimMoney(ctx, DimUser); err == nil {
			users = mergeMoney(users, money)
		}
	}
	if s.roles == nil || len(users) == 0 {
		return nil
	}
	ids := make([]string, 0, len(users))
	for _, row := range users {
		ids = append(ids, row.Key)
	}
	roles, err := s.roles.MapUserRoles(ctx, ids)
	if err != nil {
		return nil
	}
	byRole := map[string]DimStat{}
	for _, row := range users {
		role := roles[row.Key]
		if role == "" {
			continue
		}
		cur := byRole[role]
		cur.Dimension = DimAgent
		cur.Key = role
		cur.Requests += row.Requests
		cur.Successes += row.Successes
		cur.Errors += row.Errors
		cur.Fallbacks += row.Fallbacks
		cur.UsageMinor += row.UsageMinor
		cur.RevenueMinor += row.RevenueMinor
		cur.CostMinor += row.CostMinor
		if row.LatencyP95MS > cur.LatencyP95MS {
			cur.LatencyP95MS = row.LatencyP95MS
		}
		if cur.LatencyP50MS == 0 || (row.LatencyP50MS > 0 && row.LatencyP50MS < cur.LatencyP50MS) {
			cur.LatencyP50MS = row.LatencyP50MS
		}
		byRole[role] = cur
	}
	out := make([]DimStat, 0, len(byRole))
	for _, row := range byRole {
		if row.Requests > 0 {
			row.SuccessRate = float64(row.Successes) / float64(row.Requests)
		}
		row.MarginMinor = row.RevenueMinor - row.CostMinor
		out = append(out, row)
	}
	return out
}

func fillOverview(totals *MoneyView, providers []DimStat) {
	var req, ok, errs, p50, p95 int64
	for _, row := range providers {
		req += row.Requests
		ok += row.Successes
		errs += row.Errors
		if row.LatencyP95MS > p95 {
			p95 = row.LatencyP95MS
		}
		if p50 == 0 || (row.LatencyP50MS > 0 && row.LatencyP50MS < p50) {
			p50 = row.LatencyP50MS
		}
	}
	if req > 0 {
		totals.SuccessRate = float64(ok) / float64(req)
	}
	totals.LatencyP50MS = p50
	totals.LatencyP95MS = p95
	totals.UpstreamErrors = errs
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Service) ListRunbooks(ctx context.Context) ([]RunbookView, error) {
	var rows []runbookRow
	if err := s.db.WithContext(ctx).Order("alert_kind").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RunbookView, 0, len(rows))
	for _, row := range rows {
		out = append(out, RunbookView{ID: row.ID, AlertKind: row.AlertKind, Title: row.Title, Body: row.Body})
	}
	return out, nil
}

func (s *Service) ListAlerts(ctx context.Context, status string) ([]AlertView, error) {
	q := s.db.WithContext(ctx).Order("created_at DESC").Limit(50)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []alertRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AlertView, 0, len(rows))
	for _, row := range rows {
		out = append(out, AlertView{ID: row.ID, Kind: row.Kind, Severity: row.Severity, Status: row.Status, Message: row.Message, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (s *Service) openAlert(ctx context.Context, kind, severity, message string, payload any) error {
	var existing alertRow
	if err := s.db.WithContext(ctx).Where("kind = ? AND status = ?", kind, StatusOpen).First(&existing).Error; err == nil {
		return nil
	}
	body, _ := json.Marshal(payload)
	row := alertRow{
		ID: id.New("alt"), Kind: kind, Severity: severity, Status: StatusOpen,
		Message: message, PayloadJSON: body, CreatedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *Service) resolveAlert(ctx context.Context, kind string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&alertRow{}).
		Where("kind = ? AND status = ?", kind, StatusOpen).
		Updates(map[string]any{"status": StatusResolved, "resolved_at": now}).Error
}

func (s *Service) EvaluateAlerts(ctx context.Context) ([]AlertView, error) {
	if s.money != nil {
		if money, err := s.money.Money(ctx); err == nil && money != nil {
			if money.PendingCount > 0 {
				_ = s.openAlert(ctx, AlertPending, SeverityHigh, "存在待对账预授权，禁止按估算扣款", money)
			} else {
				_ = s.resolveAlert(ctx, AlertPending)
			}
		}
	}
	if s.traffic != nil {
		if rows, err := s.traffic.DimStats(ctx, DimProvider); err == nil {
			low := false
			for _, row := range rows {
				if row.Requests >= 5 && row.SuccessRate < 0.5 {
					low = true
					_ = s.openAlert(ctx, AlertLowSuccess, SeverityMed, "成功率低于 50%："+row.Key, row)
				}
			}
			if !low {
				_ = s.resolveAlert(ctx, AlertLowSuccess)
			}
		}
	}
	if last, _ := s.LastDrill(ctx); last == nil {
		_ = s.openAlert(ctx, AlertBackup, SeverityMed, "尚未记录备份恢复演练", nil)
	} else {
		_ = s.resolveAlert(ctx, AlertBackup)
	}
	return s.ListAlerts(ctx, StatusOpen)
}

func (s *Service) RecordBackupDrill(ctx context.Context, method, evidence, actor string, passed bool) (*DrillView, error) {
	status := "passed"
	if !passed {
		status = "failed"
	}
	row := drillRow{
		ID: id.New("bdr"), Status: status, RPOMinutes: RPOMinutes, RTOMinutes: RTOMinutes,
		Method: method, Evidence: evidence, CreatedAt: time.Now().UTC(),
	}
	if actor != "" {
		row.Actor = &actor
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	if passed {
		_ = s.resolveAlert(ctx, AlertBackup)
	}
	return drillView(row), nil
}

func (s *Service) LastDrill(ctx context.Context) (*DrillView, error) {
	var row drillRow
	if err := s.db.WithContext(ctx).Order("created_at DESC").First(&row).Error; err != nil {
		return nil, nil
	}
	return drillView(row), nil
}

func (s *Service) Canary(ctx context.Context, routeKey string) (*CanaryView, error) {
	if routeKey == "" {
		routeKey = CanaryChat
	}
	var row canaryRow
	if err := s.db.WithContext(ctx).Where("route_key = ?", routeKey).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return &CanaryView{RouteKey: row.RouteKey, ProviderSlug: row.ProviderSlug, Percent: row.Percent}, nil
}

func (s *Service) SetCanary(ctx context.Context, routeKey, slug string, percent int) (*CanaryView, error) {
	if routeKey == "" {
		routeKey = CanaryChat
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if slug == "" {
		slug = "echo-backup"
	}
	var row canaryRow
	err := s.db.WithContext(ctx).Where("route_key = ?", routeKey).First(&row).Error
	if err != nil {
		row = canaryRow{ID: id.New("cny"), RouteKey: routeKey}
	}
	row.ProviderSlug = slug
	row.Percent = percent
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return &CanaryView{RouteKey: row.RouteKey, ProviderSlug: row.ProviderSlug, Percent: row.Percent}, nil
}

func (s *Service) CanarySlug(ctx context.Context, force bool) string {
	item, err := s.Canary(ctx, CanaryChat)
	if err != nil || item == nil || item.ProviderSlug == "" {
		return ""
	}
	if force || item.Percent >= 100 {
		return item.ProviderSlug
	}
	return ""
}

func drillView(row drillRow) *DrillView {
	return &DrillView{
		ID: row.ID, Status: row.Status, RPOMinutes: row.RPOMinutes, RTOMinutes: row.RTOMinutes,
		Method: row.Method, Evidence: row.Evidence, CreatedAt: row.CreatedAt,
	}
}
