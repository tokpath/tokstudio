package commission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorkflowScope struct {
	OwnerID           string
	ActorUserID       string
	PolicyChannelID   string
	Channels          []string
	ReferenceChannels []string
}

func (scope WorkflowScope) valid() bool {
	return scope.OwnerID != "" && scope.ActorUserID != "" && len(scope.Channels) > 0
}
func canonicalChannels(channels []string) []string {
	copy := append([]string{}, channels...)
	sort.Strings(copy)
	return copy
}
func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

type SettlementGroup struct {
	ChannelOrgID      string `json:"channel_org_id"`
	BeneficiaryRoleID string `json:"beneficiary_role_id"`
	EntryCount        int    `json:"entry_count"`
	AmountMinor       int64  `json:"amount_minor"`
}
type ExcludedCommission struct {
	EntryCount  int64 `json:"entry_count"`
	AmountMinor int64 `json:"amount_minor"`
}
type SettlementPreview struct {
	ID              string                        `json:"id"`
	OwnerID         string                        `json:"owner_id"`
	Channels        []string                      `json:"channel_ids"`
	PeriodStart     time.Time                     `json:"period_start"`
	PeriodEnd       time.Time                     `json:"period_end"`
	FactStart       *time.Time                    `json:"fact_start,omitempty"`
	FactEnd         *time.Time                    `json:"fact_end,omitempty"`
	EntryCount      int                           `json:"entry_count"`
	RecipientCount  int                           `json:"recipient_count"`
	SettlementCount int                           `json:"settlement_count"`
	AmountMinor     int64                         `json:"amount_minor"`
	MinSettleMinor  int64                         `json:"min_settle_minor"`
	IgnoreMinimum   bool                          `json:"ignore_minimum"`
	PolicyVersion   string                        `json:"policy_version"`
	Groups          []SettlementGroup             `json:"groups"`
	Excluded        map[string]ExcludedCommission `json:"excluded"`
	CreatedAt       time.Time                     `json:"created_at"`
	ExpiresAt       time.Time                     `json:"expires_at"`
}
type workflowPreviewRow struct {
	ID, OwnerID, ActorUserID, Fingerprint string
	ChannelsJSON                          json.RawMessage `gorm:"column:channels_json"`
	IgnoreMinimum                         bool
	PreviewJSON                           json.RawMessage `gorm:"column:preview_json"`
	CreatedAt, ExpiresAt                  time.Time
}

func (workflowPreviewRow) TableName() string { return "commission_workflow_previews" }

type workflowOperationRow struct {
	ID, OwnerID, ActorUserID, OperationID, Kind, PayloadHash string
	ChannelsJSON                                             json.RawMessage `gorm:"column:channels_json"`
	ResultJSON                                               json.RawMessage `gorm:"column:result_json"`
	CreatedAt                                                time.Time
}

func (workflowOperationRow) TableName() string { return "commission_workflow_operations" }

type WorkflowOperation struct {
	ID          string          `json:"id"`
	OperationID string          `json:"operation_id"`
	Kind        string          `json:"kind"`
	Result      json.RawMessage `json:"result"`
	CreatedAt   time.Time       `json:"created_at"`
}

func channelsWithin(stored []byte, allowed []string) bool {
	var channels []string
	if json.Unmarshal(stored, &channels) != nil || len(channels) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, channel := range allowed {
		set[channel] = true
	}
	for _, channel := range channels {
		if !set[channel] {
			return false
		}
	}
	return true
}
func (s *Service) WorkflowOperation(ctx context.Context, scope WorkflowScope, operationID string) (*WorkflowOperation, error) {
	if !scope.valid() {
		return nil, ErrInvalid
	}
	var row workflowOperationRow
	if err := s.db.WithContext(ctx).Where("owner_id=? AND actor_user_id=? AND operation_id=?", scope.OwnerID, scope.ActorUserID, operationID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !channelsWithin(row.ChannelsJSON, scope.Channels) {
		return nil, ErrNotFound
	}
	return &WorkflowOperation{row.ID, row.OperationID, row.Kind, row.ResultJSON, row.CreatedAt}, nil
}
func loadWorkflowOperationTx(tx *gorm.DB, scope WorkflowScope, operationID, kind string, payload any, result any) (bool, error) {
	if !scope.valid() || strings.TrimSpace(operationID) == "" || len(operationID) > 200 {
		return false, ErrInvalid
	}
	if err := lockSettlementLifecycle(tx); err != nil {
		return false, err
	}
	var row workflowOperationRow
	err := tx.Where("owner_id=? AND actor_user_id=? AND operation_id=?", scope.OwnerID, scope.ActorUserID, operationID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Kind != kind || row.PayloadHash != fingerprint(payload) {
		return false, ErrConflict
	}
	if !channelsWithin(row.ChannelsJSON, scope.Channels) {
		return false, ErrNotFound
	}
	return true, json.Unmarshal(row.ResultJSON, result)
}
func saveWorkflowOperationTx(tx *gorm.DB, scope WorkflowScope, operationID, kind string, payload, result any) error {
	channels, _ := json.Marshal(canonicalChannels(scope.Channels))
	raw, _ := json.Marshal(result)
	return tx.Create(&workflowOperationRow{ID: id.New("cop"), OwnerID: scope.OwnerID, ActorUserID: scope.ActorUserID, OperationID: operationID, Kind: kind, PayloadHash: fingerprint(payload), ChannelsJSON: channels, ResultJSON: raw, CreatedAt: time.Now().UTC()}).Error
}

type settlementGroupRows struct {
	view    SettlementGroup
	entries []entryRow
}
type preparedSettlement struct {
	preview SettlementPreview
	entries []entryRow
	groups  []settlementGroupRows
	policy  *PolicyView
}

func (s *Service) prepareSettlementTx(tx *gorm.DB, now time.Time, ignoreMinimum bool, policyChannel string, channels []string) (*preparedSettlement, error) {
	service := *s
	service.db = tx
	policy, err := service.PolicyFor(tx.Statement.Context, policyChannel)
	if err != nil {
		return nil, err
	}
	prepared := &preparedSettlement{policy: policy}
	preview := &prepared.preview
	preview.PeriodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	preview.PeriodEnd = preview.PeriodStart.AddDate(0, 1, 0)
	preview.PolicyVersion, preview.MinSettleMinor, preview.IgnoreMinimum = policy.Version, policy.MinSettleMinor, ignoreMinimum
	preview.Groups = []SettlementGroup{}
	preview.Excluded = map[string]ExcludedCommission{}
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status=?", StatusAvailable).Order("id")
	if channels != nil {
		q = q.Where("COALESCE(channel_org_id,'') IN ?", channels)
	}
	if err := q.Find(&prepared.entries).Error; err != nil {
		return nil, err
	}
	var excluded []struct {
		Status      string
		EntryCount  int64
		AmountMinor int64
	}
	q = tx.Model(&entryRow{}).Where("status<>?", StatusAvailable).Select("status,COUNT(*) AS entry_count,COALESCE(SUM(amount_minor),0) AS amount_minor").Group("status")
	if channels != nil {
		q = q.Where("COALESCE(channel_org_id,'') IN ?", channels)
	}
	if err := q.Scan(&excluded).Error; err != nil {
		return nil, err
	}
	for _, row := range excluded {
		preview.Excluded[row.Status] = ExcludedCommission{row.EntryCount, row.AmountMinor}
	}
	groups := map[[2]string]*settlementGroupRows{}
	order := [][2]string{}
	for _, entry := range prepared.entries {
		key := [2]string{}
		if entry.ChannelOrgID != nil {
			key[0] = *entry.ChannelOrgID
		}
		if entry.BeneficiaryRoleID != nil {
			key[1] = *entry.BeneficiaryRoleID
		}
		if groups[key] == nil {
			groups[key] = &settlementGroupRows{view: SettlementGroup{ChannelOrgID: key[0], BeneficiaryRoleID: key[1]}}
			order = append(order, key)
		}
		g := groups[key]
		g.entries = append(g.entries, entry)
		g.view.EntryCount++
		g.view.AmountMinor += entry.AmountMinor
	}
	recipients := map[string]bool{}
	for _, key := range order {
		group := groups[key]
		if group.view.AmountMinor <= 0 || (!ignoreMinimum && group.view.AmountMinor < policy.MinSettleMinor) {
			excluded := preview.Excluded["below_minimum"]
			excluded.EntryCount += int64(group.view.EntryCount)
			excluded.AmountMinor += group.view.AmountMinor
			preview.Excluded["below_minimum"] = excluded
			continue
		}
		prepared.groups = append(prepared.groups, *group)
		preview.Groups = append(preview.Groups, group.view)
		preview.EntryCount += group.view.EntryCount
		preview.AmountMinor += group.view.AmountMinor
		recipients[group.view.BeneficiaryRoleID] = true
		for _, entry := range group.entries {
			at := entry.CreatedAt
			if preview.FactStart == nil || at.Before(*preview.FactStart) {
				copy := at
				preview.FactStart = &copy
			}
			if preview.FactEnd == nil || at.After(*preview.FactEnd) {
				copy := at
				preview.FactEnd = &copy
			}
		}
	}
	preview.RecipientCount = len(recipients)
	preview.SettlementCount = len(prepared.groups)
	return prepared, nil
}
func settlementFingerprint(prepared *preparedSettlement, scope WorkflowScope) string {
	return fingerprint(struct {
		Entries       []entryRow
		Policy        *PolicyView
		Channels      []string
		IgnoreMinimum bool
	}{prepared.entries, prepared.policy, canonicalChannels(scope.Channels), prepared.preview.IgnoreMinimum})
}
func (s *Service) PreviewSettlement(ctx context.Context, scope WorkflowScope, now time.Time, ignoreMinimum bool) (*SettlementPreview, error) {
	if !scope.valid() {
		return nil, ErrInvalid
	}
	var out *SettlementPreview
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		prepared, err := s.prepareSettlementTx(tx, now, ignoreMinimum, scope.PolicyChannelID, scope.Channels)
		if err != nil {
			return err
		}
		preview := prepared.preview
		preview.ID = id.New("cpr")
		preview.OwnerID = scope.OwnerID
		preview.Channels = canonicalChannels(scope.Channels)
		preview.CreatedAt = now.UTC()
		preview.ExpiresAt = now.UTC().Add(10 * time.Minute)
		channels, _ := json.Marshal(preview.Channels)
		raw, _ := json.Marshal(preview)
		if err := tx.Create(&workflowPreviewRow{ID: preview.ID, OwnerID: scope.OwnerID, ActorUserID: scope.ActorUserID, ChannelsJSON: channels, IgnoreMinimum: ignoreMinimum, Fingerprint: settlementFingerprint(prepared, scope), PreviewJSON: raw, CreatedAt: preview.CreatedAt, ExpiresAt: preview.ExpiresAt}).Error; err != nil {
			return err
		}
		out = &preview
		return nil
	})
	return out, err
}

type SettlementCreateInput struct {
	OperationID string `json:"operation_id"`
	PreviewID   string `json:"preview_id"`
}

func (s *Service) CreatePreviewedSettlementTx(tx *gorm.DB, scope WorkflowScope, in SettlementCreateInput) ([]SettlementView, bool, error) {
	out := []SettlementView{}
	payload := struct {
		SettlementCreateInput
		Channels []string
	}{in, canonicalChannels(scope.Channels)}
	replayed, err := loadWorkflowOperationTx(tx, scope, in.OperationID, "settle", payload, &out)
	if err != nil || replayed {
		return out, false, err
	}
	var stored workflowPreviewRow
	if err := tx.Where("id=? AND owner_id=? AND actor_user_id=?", in.PreviewID, scope.OwnerID, scope.ActorUserID).First(&stored).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	if !stored.ExpiresAt.After(time.Now().UTC()) || !channelsWithin(stored.ChannelsJSON, scope.Channels) || string(stored.ChannelsJSON) == "null" {
		return nil, false, ErrConflict
	}
	var preview SettlementPreview
	if err := json.Unmarshal(stored.PreviewJSON, &preview); err != nil {
		return nil, false, err
	}
	prepared, err := s.prepareSettlementTx(tx, preview.CreatedAt, stored.IgnoreMinimum, scope.PolicyChannelID, scope.Channels)
	if err != nil {
		return nil, false, err
	}
	if stored.Fingerprint != settlementFingerprint(prepared, scope) {
		return nil, false, ErrConflict
	}
	if len(prepared.groups) == 0 {
		return nil, false, ErrBelowMinimum
	}
	out, err = s.createPreparedSettlementTx(tx, preview.CreatedAt, prepared)
	if err != nil {
		return nil, false, err
	}
	if err := saveWorkflowOperationTx(tx, scope, in.OperationID, "settle", payload, out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (s *Service) SettlementIDsForChannels(ctx context.Context, channels []string) ([]string, error) {
	ids := []string{}
	if len(channels) == 0 {
		return ids, ErrInvalid
	}
	err := s.db.WithContext(ctx).Model(&settleRow{}).Where("COALESCE(channel_org_id,'') IN ?", channels).Pluck("id", &ids).Error
	return ids, err
}

type UnfreezeInput struct {
	OperationID  string `json:"operation_id"`
	UsageEventID string `json:"usage_event_id"`
}

func (s *Service) UnfreezeWorkflowTx(tx *gorm.DB, scope WorkflowScope, in UnfreezeInput) (int, bool, error) {
	result := struct {
		Unfrozen int `json:"unfrozen"`
	}{0}
	payload := struct {
		UnfreezeInput
		Channels []string
	}{in, canonicalChannels(scope.Channels)}
	replayed, err := loadWorkflowOperationTx(tx, scope, in.OperationID, "unfreeze", payload, &result)
	if err != nil || replayed {
		return result.Unfrozen, false, err
	}
	now := time.Now().UTC()
	ids := []string{}
	q := tx.Model(&entryRow{}).Where("COALESCE(channel_org_id,'') IN ? AND status=? AND available_at<=?", scope.Channels, StatusFrozen, now)
	if in.UsageEventID != "" {
		q = q.Where("usage_event_id=?", in.UsageEventID)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return 0, false, err
	}
	if len(ids) > 0 {
		n, err := s.unfreezeTx(tx, now, in.UsageEventID, ids)
		if err != nil {
			return 0, false, err
		}
		result.Unfrozen = n
	}
	if err := saveWorkflowOperationTx(tx, scope, in.OperationID, "unfreeze", payload, result); err != nil {
		return 0, false, err
	}
	return result.Unfrozen, true, nil
}
func (s *Service) SettlementDetail(ctx context.Context, settlementID string, channels []string) (*SettlementView, []EntryView, error) {
	if len(channels) == 0 {
		return nil, nil, ErrInvalid
	}
	var row settleRow
	if err := s.db.WithContext(ctx).Where("id=? AND COALESCE(channel_org_id,'') IN ?", settlementID, channels).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	views, err := s.settlementViews(ctx, []settleRow{row})
	if err != nil {
		return nil, nil, err
	}
	var rows []entryRow
	q := s.db.WithContext(ctx).Where("settlement_id=?", settlementID)
	if len(row.EntryIDsJSON) > 0 {
		var ids []string
		if err := json.Unmarshal(row.EntryIDsJSON, &ids); err != nil {
			return nil, nil, err
		}
		q = s.db.WithContext(ctx).Where("id IN ?", ids)
	}
	if err := q.Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	entries := []EntryView{}
	for _, entry := range rows {
		entries = append(entries, *entryView(entry))
	}
	return &views[0], entries, nil
}
func (s *Service) createPreparedSettlementTx(tx *gorm.DB, now time.Time, prepared *preparedSettlement) ([]SettlementView, error) {
	out := []SettlementView{}
	for _, group := range prepared.groups {
		row := settleRow{ID: id.New("csl"), PeriodStart: prepared.preview.PeriodStart, PeriodEnd: prepared.preview.PeriodEnd, AmountMinor: group.view.AmountMinor, Status: StatusSettled, PolicyVersion: prepared.policy.Version, CreatedAt: now}
		ids := []string{}
		for _, entry := range group.entries {
			ids = append(ids, entry.ID)
			row.ChannelOrgID = entry.ChannelOrgID
			row.BeneficiaryRoleID = entry.BeneficiaryRoleID
		}
		row.EntryIDsJSON, _ = json.Marshal(ids)
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&entryRow{}).Where("id IN ?", ids).Updates(map[string]any{"status": StatusSettled, "settlement_id": row.ID}).Error; err != nil {
			return nil, err
		}
		if _, err := s.outbox.EnqueueTx(tx, "commission.settlement.created", "commission_settlement", row.ID, map[string]any{"entry_ids": ids, "amount_minor": row.AmountMinor}); err != nil {
			return nil, err
		}
		out = append(out, *settleView(row))
	}
	return out, nil
}
