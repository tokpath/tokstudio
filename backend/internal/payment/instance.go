package payment

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type instanceRow struct {
	ID                    string     `gorm:"column:id;primaryKey"`
	ChannelOrgID          string     `gorm:"column:channel_org_id"`
	Adapter               string     `gorm:"column:adapter"`
	Name                  string     `gorm:"column:name"`
	Mode                  string     `gorm:"column:mode"`
	Enabled               bool       `gorm:"column:enabled"`
	RefundEnabled         bool       `gorm:"column:refund_enabled"`
	SortOrder             int        `gorm:"column:sort_order"`
	MinAmountMinor        *int64     `gorm:"column:min_amount_minor"`
	MaxAmountMinor        *int64     `gorm:"column:max_amount_minor"`
	DailyLimitMinor       *int64     `gorm:"column:daily_limit_minor"`
	CredentialsCiphertext string     `gorm:"column:credentials_ciphertext"`
	CredentialsMeta       []byte     `gorm:"column:credentials_meta;type:jsonb"`
	LastTestedAt          *time.Time `gorm:"column:last_tested_at"`
	LastTestOK            *bool      `gorm:"column:last_test_ok"`
	LastPaidAt            *time.Time `gorm:"column:last_paid_at"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (instanceRow) TableName() string { return "payment_provider_instances" }

type settingsRow struct {
	ChannelOrgID     string    `gorm:"column:channel_org_id;primaryKey"`
	QuickAmountsJSON []byte    `gorm:"column:quick_amounts_json;type:jsonb"`
	MinPayMajor      int64     `gorm:"column:min_pay_major"`
	MaxPayMajor      int64     `gorm:"column:max_pay_major"`
	TimeoutMinutes   int       `gorm:"column:timeout_minutes"`
	HelpText         string    `gorm:"column:help_text"`
	HelpImageURL     string    `gorm:"column:help_image_url"`
	ProductPrefix    string    `gorm:"column:product_prefix"`
	ProductSuffix    string    `gorm:"column:product_suffix"`
	MaxPendingOrders int       `gorm:"column:max_pending_orders"`
	CancelRateLimit  int       `gorm:"column:cancel_rate_limit"`
	FeeBPS           int       `gorm:"column:fee_bps"`
	OnlineDisabled   bool      `gorm:"column:online_disabled"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (settingsRow) TableName() string { return "payment_channel_settings" }

type adapterFlagRow struct {
	Adapter   string    `gorm:"column:adapter;primaryKey"`
	Enabled   bool      `gorm:"column:enabled"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (adapterFlagRow) TableName() string { return "payment_adapter_flags" }

func (s *Service) CreateInstance(ctx context.Context, channelOrgID string, in InstanceInput) (*InstanceView, error) {
	if channelOrgID == "" {
		return nil, ErrInstanceNotFound
	}
	adapter := strings.ToLower(strings.TrimSpace(in.Adapter))
	plugin, ok := s.plugin(adapter)
	if !ok {
		return nil, ErrInvalidAdapter
	}
	if plugin.Spec().Kind == KindManual {
		return nil, ErrInvalidAdapter
	}
	if !s.adapterEnabled(ctx, adapter) {
		return nil, ErrAdapterDisabled
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = adapter + " 商户"
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode != ModeLive {
		mode = ModeSandbox
	}
	now := time.Now().UTC()
	var maxSort int
	_ = s.db.WithContext(ctx).Model(&instanceRow{}).
		Where("channel_org_id = ? AND adapter = ?", channelOrgID, adapter).
		Select("COALESCE(MAX(sort_order), -1)").Scan(&maxSort)
	row := instanceRow{
		ID: id.New("ppi"), ChannelOrgID: channelOrgID, Adapter: adapter, Name: name,
		Mode: mode, Enabled: true, RefundEnabled: true, SortOrder: maxSort + 1,
		CreatedAt: now, UpdatedAt: now, CredentialsMeta: []byte(`{}`),
	}
	if in.RefundEnabled != nil {
		row.RefundEnabled = *in.RefundEnabled
	}
	if in.MinAmountMinor != nil {
		row.MinAmountMinor = in.MinAmountMinor
	}
	if in.MaxAmountMinor != nil {
		row.MaxAmountMinor = in.MaxAmountMinor
	}
	if in.DailyLimitMinor != nil {
		row.DailyLimitMinor = in.DailyLimitMinor
	}
	if len(in.Credentials) > 0 {
		sealed, meta, err := sealCredentials(s.signKey, adapter, mergeCredentials(nil, in.Credentials))
		if err != nil {
			return nil, err
		}
		row.CredentialsCiphertext = sealed
		row.CredentialsMeta = meta
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return s.instanceView(row, ""), nil
}

func (s *Service) PatchInstance(ctx context.Context, channelOrgID, instanceID string, in InstanceInput) (*InstanceView, error) {
	var row instanceRow
	q := s.db.WithContext(ctx).Where("id = ?", instanceID)
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrInstanceNotFound
	}
	if in.Name != "" {
		row.Name = strings.TrimSpace(in.Name)
	}
	if in.Mode == ModeLive || in.Mode == ModeSandbox {
		row.Mode = in.Mode
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.RefundEnabled != nil {
		row.RefundEnabled = *in.RefundEnabled
	}
	if in.MinAmountMinor != nil {
		row.MinAmountMinor = in.MinAmountMinor
	}
	if in.MaxAmountMinor != nil {
		row.MaxAmountMinor = in.MaxAmountMinor
	}
	if in.DailyLimitMinor != nil {
		row.DailyLimitMinor = in.DailyLimitMinor
	}
	if len(in.Credentials) > 0 {
		existing, err := openCredentials(s.signKey, row.CredentialsCiphertext)
		if err != nil {
			existing = map[string]string{}
		}
		sealed, meta, err := sealCredentials(s.signKey, row.Adapter, mergeCredentials(existing, in.Credentials))
		if err != nil {
			return nil, err
		}
		row.CredentialsCiphertext = sealed
		row.CredentialsMeta = meta
		row.LastTestOK = nil
		row.LastTestedAt = nil
	}
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return s.instanceView(row, ""), nil
}

func (s *Service) TestInstance(ctx context.Context, channelOrgID, instanceID string) (*InstanceView, error) {
	var row instanceRow
	q := s.db.WithContext(ctx).Where("id = ?", instanceID)
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrInstanceNotFound
	}
	creds, _ := openCredentials(s.signKey, row.CredentialsCiphertext)
	plugin, ok := s.plugin(row.Adapter)
	okTest := false
	if ok {
		if err := plugin.Test(ctx, TestInput{Credentials: creds, Mode: row.Mode}); err == nil {
			okTest = true
		}
	} else {
		set, _ := parseCredMeta(row.CredentialsMeta)
		okTest = len(s.missingFor(row.Adapter, set)) == 0
	}
	now := time.Now().UTC()
	row.LastTestedAt = &now
	row.LastTestOK = &okTest
	row.UpdatedAt = now
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	if !okTest {
		return s.instanceView(row, ""), ErrInstanceIncomplete
	}
	return s.instanceView(row, ""), nil
}

func (s *Service) GoLiveInstance(ctx context.Context, channelOrgID, instanceID string) (*InstanceView, error) {
	var row instanceRow
	q := s.db.WithContext(ctx).Where("id = ?", instanceID)
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrInstanceNotFound
	}
	set, _ := parseCredMeta(row.CredentialsMeta)
	if len(s.missingFor(row.Adapter, set)) > 0 {
		return nil, ErrInstanceIncomplete
	}
	if row.LastTestOK == nil || !*row.LastTestOK {
		return nil, ErrNotTested
	}
	now := time.Now().UTC()
	row.Mode = ModeLive
	row.Enabled = true
	row.UpdatedAt = now
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return s.instanceView(row, ""), nil
}

func (s *Service) ListInstances(ctx context.Context, channelOrgID, adapter string) ([]InstanceView, error) {
	var rows []instanceRow
	q := s.db.WithContext(ctx).Where("channel_org_id = ?", channelOrgID).Order("adapter, sort_order, created_at")
	if adapter != "" {
		q = q.Where("adapter = ?", adapter)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]InstanceView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *s.instanceView(row, ""))
	}
	return out, nil
}

func (s *Service) GetSettings(ctx context.Context, channelOrgID string) (*SettingsView, error) {
	row, err := s.ensureSettings(ctx, channelOrgID)
	if err != nil {
		return nil, err
	}
	bps := billing.DefaultIssueRatioBPS
	if s.billing != nil {
		if rule, err := s.billing.IssueRule(ctx, channelOrgID); err == nil && rule != nil {
			bps = rule.IssueRatioBPS
		}
	}
	return settingsView(row, bps), nil
}

func (s *Service) PatchSettings(ctx context.Context, channelOrgID string, in SettingsInput) (*SettingsView, error) {
	row, err := s.ensureSettings(ctx, channelOrgID)
	if err != nil {
		return nil, err
	}
	if in.QuickAmounts != nil {
		cleaned := cleanQuickAmounts(in.QuickAmounts)
		raw, _ := json.Marshal(cleaned)
		row.QuickAmountsJSON = raw
	}
	if in.MinPayMajor != nil && *in.MinPayMajor > 0 {
		row.MinPayMajor = *in.MinPayMajor
	}
	if in.MaxPayMajor != nil && *in.MaxPayMajor > 0 {
		row.MaxPayMajor = *in.MaxPayMajor
	}
	if in.TimeoutMinutes != nil && *in.TimeoutMinutes > 0 {
		row.TimeoutMinutes = *in.TimeoutMinutes
	}
	if in.HelpText != nil {
		row.HelpText = *in.HelpText
	}
	if in.HelpImageURL != nil {
		row.HelpImageURL = *in.HelpImageURL
	}
	if in.ProductPrefix != nil {
		row.ProductPrefix = *in.ProductPrefix
	}
	if in.ProductSuffix != nil {
		row.ProductSuffix = *in.ProductSuffix
	}
	if in.MaxPendingOrders != nil && *in.MaxPendingOrders > 0 {
		row.MaxPendingOrders = *in.MaxPendingOrders
	}
	if in.CancelRateLimit != nil && *in.CancelRateLimit >= 0 {
		row.CancelRateLimit = *in.CancelRateLimit
	}
	if in.FeeBPS != nil && *in.FeeBPS >= 0 && *in.FeeBPS <= 2000 {
		row.FeeBPS = *in.FeeBPS
	}
	if in.OnlineDisabled != nil {
		row.OnlineDisabled = *in.OnlineDisabled
	}
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, channelOrgID)
}

func (s *Service) Overview(ctx context.Context, channelOrgID, callbackOrigin string) (*OverviewView, error) {
	view := &OverviewView{
		ChannelOrgID:   channelOrgID,
		FenPerUSD:      FenPerUSD,
		CallbackOrigin: strings.TrimRight(callbackOrigin, "/"),
		Lanes:          []LaneView{},
	}
	var instances []InstanceView
	onlineDisabled := false
	if channelOrgID != "" {
		if settings, err := s.GetSettings(ctx, channelOrgID); err == nil && settings != nil {
			view.OnlineDisabled = settings.OnlineDisabled
			view.IssueRatioBPS = settings.IssueRatioBPS
			onlineDisabled = settings.OnlineDisabled
		}
		if items, err := s.ListInstances(ctx, channelOrgID, ""); err == nil {
			instances = items
		}
	}
	byAdapter := map[string][]InstanceView{}
	for _, item := range instances {
		byAdapter[item.Adapter] = append(byAdapter[item.Adapter], item)
	}
	for _, plugin := range s.userFacing() {
		adapter := plugin.Spec().ID
		spec := plugin.Spec()
		lane := LaneView{
			Adapter: adapter, DisplayName: spec.DisplayName, Kind: spec.Kind, BrandColor: spec.BrandColor,
			CheckoutMode: spec.CheckoutMode, PayCurrency: spec.PayCurrency, State: LaneNone,
			AutoRenew: spec.AutoRenew, Schema: spec.Credentials,
		}
		items := byAdapter[adapter]
		lane.InstanceCount = len(items)
		if !s.adapterEnabled(ctx, adapter) {
			lane.State = LaneDisabled
			view.Lanes = append(view.Lanes, lane)
			continue
		}
		if onlineDisabled {
			if len(items) == 0 {
				lane.State = LaneNone
			} else {
				lane.State = LaneDisabled
			}
			view.Lanes = append(view.Lanes, lane)
			continue
		}
		var missing []string
		hasLive, hasSandbox, hasIncomplete := false, false, false
		for _, item := range items {
			if item.LastPaidAt != nil && (lane.LastPaidAt == nil || item.LastPaidAt.After(*lane.LastPaidAt)) {
				lane.LastPaidAt = item.LastPaidAt
			}
			if !item.Enabled {
				continue
			}
			if len(item.MissingFields) > 0 {
				hasIncomplete = true
				if len(missing) == 0 {
					missing = item.MissingFields
				}
				continue
			}
			if item.Mode == ModeLive {
				hasLive = true
			} else {
				hasSandbox = true
			}
		}
		switch {
		case hasLive:
			lane.State = LaneLive
		case hasSandbox:
			lane.State = LaneSandbox
		case hasIncomplete || len(items) > 0:
			lane.State = LaneConfiguring
			lane.MissingFields = missing
		default:
			lane.State = LaneNone
		}
		view.Lanes = append(view.Lanes, lane)
	}
	return view, nil
}

func (s *Service) UserCheckout(ctx context.Context, channelOrgID string) (*UserCheckoutView, error) {
	if channelOrgID == "" {
		channelOrgID = identity.OfficialChannelID
	}
	settings, err := s.GetSettings(ctx, channelOrgID)
	if err != nil {
		return nil, err
	}
	view := &UserCheckoutView{ChannelOrgID: channelOrgID, OnlineDisabled: settings.OnlineDisabled, Settings: settings, HelpText: settings.HelpText, HelpImageURL: settings.HelpImageURL}
	if settings.OnlineDisabled {
		view.EmptyReason = "channel_disabled"
		view.Methods = []CheckoutMethodView{}
		return view, nil
	}
	instances, err := s.ListInstances(ctx, channelOrgID, "")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, plugin := range s.userFacing() {
		spec := plugin.Spec()
		if seen[spec.ID] || !s.adapterEnabled(ctx, spec.ID) {
			continue
		}
		var chosen *InstanceView
		for i := range instances {
			item := instances[i]
			if item.Adapter != spec.ID || !item.Enabled || len(item.MissingFields) > 0 {
				continue
			}
			if item.Mode != ModeLive && item.Mode != ModeSandbox {
				continue
			}
			chosen = &item
			break
		}
		if chosen == nil {
			continue
		}
		seen[spec.ID] = true
		view.Methods = append(view.Methods, CheckoutMethodView{
			Adapter: spec.ID, DisplayName: spec.DisplayName, Name: chosen.Name,
			Sandbox: chosen.Mode != ModeLive, AutoRenew: spec.AutoRenew,
			BrandColor: spec.BrandColor, CheckoutMode: spec.CheckoutMode, PayCurrency: spec.PayCurrency,
		})
	}
	if len(view.Methods) == 0 {
		view.EmptyReason = "not_configured"
		view.Methods = []CheckoutMethodView{}
	}
	return view, nil
}

func (s *Service) QuoteForChannel(ctx context.Context, channelOrgID, adapter string, payMajor int64) (*QuoteView, error) {
	settings, err := s.GetSettings(ctx, channelOrgID)
	if err != nil {
		return nil, err
	}
	return QuotePay(adapter, payMajor, int64(settings.FeeBPS), settings.IssueRatioBPS)
}

func (s *Service) AdapterFlags(ctx context.Context) ([]AdapterFlagView, error) {
	var rows []adapterFlagRow
	if err := s.db.WithContext(ctx).Order("adapter").Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := map[string]adapterFlagRow{}
	for _, row := range rows {
		byID[row.Adapter] = row
	}
	reg := s.reg
	if reg == nil {
		reg = DefaultRegistry()
	}
	out := make([]AdapterFlagView, 0, len(reg.List()))
	for _, plugin := range reg.List() {
		id := plugin.Spec().ID
		enabled := true
		if row, ok := byID[id]; ok {
			enabled = row.Enabled
		}
		out = append(out, AdapterFlagView{Adapter: id, Enabled: enabled})
	}
	return out, nil
}

func (s *Service) SetAdapterEnabled(ctx context.Context, adapter string, enabled bool) (*AdapterFlagView, error) {
	adapter = strings.ToLower(strings.TrimSpace(adapter))
	if _, ok := s.plugin(adapter); !ok {
		return nil, ErrInvalidAdapter
	}
	now := time.Now().UTC()
	row := adapterFlagRow{Adapter: adapter, Enabled: enabled, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return &AdapterFlagView{Adapter: adapter, Enabled: enabled}, nil
}

func (s *Service) methodAllowed(ctx context.Context, channelOrgID, adapter string) error {
	plugin, ok := s.plugin(adapter)
	if !ok {
		return ErrInvalidAdapter
	}
	if plugin.Spec().Kind == KindManual {
		return nil
	}
	if !s.adapterEnabled(ctx, adapter) {
		return ErrAdapterDisabled
	}
	settings, err := s.ensureSettings(ctx, channelOrgID)
	if err != nil {
		return err
	}
	if settings.OnlineDisabled {
		return ErrOnlineDisabled
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&instanceRow{}).Where("channel_org_id = ? AND adapter = ?", channelOrgID, adapter).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		// 还没配商户时保留沙箱下单，避免打断现有套餐/webhook 测试。
		return nil
	}
	var rows []instanceRow
	if err := s.db.WithContext(ctx).Where("channel_org_id = ? AND adapter = ? AND enabled = ?", channelOrgID, adapter, true).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		set, _ := parseCredMeta(row.CredentialsMeta)
		if len(s.missingFor(adapter, set)) == 0 {
			return nil
		}
	}
	return ErrMethodUnavailable
}

func (s *Service) userFacing() []Adapter {
	if s.reg != nil {
		return s.reg.UserFacing()
	}
	return DefaultRegistry().UserFacing()
}

func (s *Service) adapterEnabled(ctx context.Context, adapter string) bool {
	var row adapterFlagRow
	if err := s.db.WithContext(ctx).Where("adapter = ?", adapter).First(&row).Error; err != nil {
		return true
	}
	return row.Enabled
}

func (s *Service) ensureSettings(ctx context.Context, channelOrgID string) (settingsRow, error) {
	var row settingsRow
	err := s.db.WithContext(ctx).Where("channel_org_id = ?", channelOrgID).First(&row).Error
	if err == nil {
		return row, nil
	}
	if err != gorm.ErrRecordNotFound {
		return row, err
	}
	now := time.Now().UTC()
	raw, _ := json.Marshal([]int64{100, 300, 500, 1000})
	row = settingsRow{
		ChannelOrgID: channelOrgID, QuickAmountsJSON: raw,
		MinPayMajor: 10, MaxPayMajor: 50000, TimeoutMinutes: 15,
		MaxPendingOrders: 3, CancelRateLimit: 10, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		// 并发下可能已经插入。
		if err := s.db.WithContext(ctx).Where("channel_org_id = ?", channelOrgID).First(&row).Error; err != nil {
			return row, err
		}
	}
	return row, nil
}

func (s *Service) touchLastPaid(ctx context.Context, channelOrgID, adapter string) {
	if channelOrgID == "" || adapter == "" {
		return
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&instanceRow{}).
		Where("channel_org_id = ? AND adapter = ? AND enabled = ?", channelOrgID, adapter, true).
		Updates(map[string]any{"last_paid_at": now, "updated_at": now})
}

func (s *Service) instanceView(row instanceRow, webhookURL string) *InstanceView {
	set, public := parseCredMeta(row.CredentialsMeta)
	missing := s.missingFor(row.Adapter, set)
	state := LaneConfiguring
	if !row.Enabled {
		state = LaneDisabled
	} else if len(missing) == 0 {
		if row.Mode == ModeLive {
			state = LaneLive
		} else {
			state = LaneSandbox
		}
	}
	fieldsSet := make([]string, 0, len(set))
	for k := range set {
		fieldsSet = append(fieldsSet, k)
	}
	view := &InstanceView{
		ID: row.ID, ChannelOrgID: row.ChannelOrgID, Adapter: row.Adapter, Name: row.Name,
		Mode: row.Mode, State: state, Enabled: row.Enabled, RefundEnabled: row.RefundEnabled,
		SortOrder: row.SortOrder, MinAmountMinor: row.MinAmountMinor, MaxAmountMinor: row.MaxAmountMinor,
		DailyLimitMinor: row.DailyLimitMinor, MissingFields: missing, PublicFields: public,
		FieldsSet: fieldsSet, WebhookURL: webhookURL, LastTestedAt: row.LastTestedAt,
		LastTestOK: row.LastTestOK, LastPaidAt: row.LastPaidAt, CreatedAt: row.CreatedAt,
	}
	return view
}

func (s *Service) missingFor(adapter string, set map[string]bool) []string {
	if plugin, ok := s.plugin(adapter); ok {
		return missingRequiredSpec(plugin.Spec(), set)
	}
	return missingRequired(adapter, set)
}

func (s *Service) instancesForAdapter(ctx context.Context, adapter string) []instanceRow {
	if adapter == "" {
		return nil
	}
	var rows []instanceRow
	if err := s.db.WithContext(ctx).Where("adapter = ? AND enabled = ?", adapter, true).
		Order("sort_order, created_at").Find(&rows).Error; err != nil {
		return nil
	}
	return rows
}

func (s *Service) firstReadyInstance(ctx context.Context, channelOrgID, adapter string) *instanceRow {
	if channelOrgID == "" || adapter == "" {
		return nil
	}
	var rows []instanceRow
	if err := s.db.WithContext(ctx).Where("channel_org_id = ? AND adapter = ? AND enabled = ?", channelOrgID, adapter, true).
		Order("sort_order, created_at").Find(&rows).Error; err != nil {
		return nil
	}
	for i := range rows {
		set, _ := parseCredMeta(rows[i].CredentialsMeta)
		if len(s.missingFor(adapter, set)) == 0 {
			return &rows[i]
		}
	}
	return nil
}

func settingsView(row settingsRow, bps int64) *SettingsView {
	amounts := []int64{100, 300, 500, 1000}
	if len(row.QuickAmountsJSON) > 0 {
		_ = json.Unmarshal(row.QuickAmountsJSON, &amounts)
	}
	return &SettingsView{
		ChannelOrgID: row.ChannelOrgID, QuickAmounts: amounts,
		MinPayMajor: row.MinPayMajor, MaxPayMajor: row.MaxPayMajor,
		TimeoutMinutes: row.TimeoutMinutes, HelpText: row.HelpText, HelpImageURL: row.HelpImageURL,
		ProductPrefix: row.ProductPrefix, ProductSuffix: row.ProductSuffix,
		MaxPendingOrders: row.MaxPendingOrders, CancelRateLimit: row.CancelRateLimit,
		FeeBPS: row.FeeBPS, OnlineDisabled: row.OnlineDisabled,
		IssueRatioBPS: bps, FenPerUSD: FenPerUSD,
	}
}

func cleanQuickAmounts(in []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, n := range in {
		if n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		if len(out) >= 8 {
			break
		}
	}
	if len(out) < 3 {
		return []int64{100, 300, 500, 1000}
	}
	return out
}

func WebhookURL(origin, adapter string) string {
	return strings.TrimRight(origin, "/") + "/v1/payments/" + adapter + "/webhook"
}

func CallbackOrigin(publicBase, apiDomain string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	apiDomain = strings.TrimSpace(apiDomain)
	if apiDomain == "" || strings.Contains(apiDomain, "localhost") && (base == "" || strings.Contains(base, "localhost")) {
		if base == "" {
			return "http://localhost"
		}
		return base
	}
	if strings.HasPrefix(apiDomain, "http://") || strings.HasPrefix(apiDomain, "https://") {
		return strings.TrimRight(apiDomain, "/")
	}
	scheme := "https"
	if strings.HasPrefix(base, "http://") {
		scheme = "http"
	}
	return scheme + "://" + apiDomain
}
