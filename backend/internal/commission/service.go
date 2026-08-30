package commission

import (
	"context"
	"embed"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type policyRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	ScopeType      string    `gorm:"column:scope_type"`
	ScopeID        string    `gorm:"column:scope_id"`
	Version        string    `gorm:"column:version"`
	DirectBPS      int       `gorm:"column:direct_bps"`
	OverrideBPS    int       `gorm:"column:override_bps"`
	ChannelBPS     int       `gorm:"column:channel_bps"`
	TeamBPS        int       `gorm:"column:team_bps"`
	CapBPS         int       `gorm:"column:cap_bps"`
	FreezeDays     int       `gorm:"column:freeze_days"`
	MinSettleMinor int64     `gorm:"column:min_settle_minor"`
	Status         string    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (policyRow) TableName() string { return "commission_policies" }

type entryRow struct {
	ID                string     `gorm:"column:id;primaryKey"`
	UsageEventID      string     `gorm:"column:usage_event_id"`
	RequestID         *string    `gorm:"column:request_id"`
	UserID            *string    `gorm:"column:user_id"`
	ChannelOrgID      *string    `gorm:"column:channel_org_id"`
	BeneficiaryRoleID *string    `gorm:"column:beneficiary_role_id"`
	Kind              string     `gorm:"column:kind"`
	PolicyVersion     string     `gorm:"column:policy_version"`
	BaseAmountMinor   int64      `gorm:"column:base_amount_minor"`
	RawAmountMinor    int64      `gorm:"column:raw_amount_minor"`
	AmountMinor       int64      `gorm:"column:amount_minor"`
	Status            string     `gorm:"column:status"`
	AvailableAt       *time.Time `gorm:"column:available_at"`
	SettlementID      *string    `gorm:"column:settlement_id"`
	SourceEntryID     *string    `gorm:"column:source_entry_id"`
	ReversalOf        *string    `gorm:"column:reversal_of"`
	IdempotencyKey    string     `gorm:"column:idempotency_key"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
}

func (entryRow) TableName() string { return "commission_entries" }

type settleRow struct {
	ID                string    `gorm:"column:id;primaryKey"`
	PeriodStart       time.Time `gorm:"column:period_start"`
	PeriodEnd         time.Time `gorm:"column:period_end"`
	ChannelOrgID      *string   `gorm:"column:channel_org_id"`
	BeneficiaryRoleID *string   `gorm:"column:beneficiary_role_id"`
	AmountMinor       int64     `gorm:"column:amount_minor"`
	Status            string    `gorm:"column:status"`
	PolicyVersion     string    `gorm:"column:policy_version"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (settleRow) TableName() string { return "commission_settlements" }

type payoutRow struct {
	ID           string    `gorm:"column:id;primaryKey"`
	SettlementID string    `gorm:"column:settlement_id"`
	Method       string    `gorm:"column:method"`
	Reference    *string   `gorm:"column:reference"`
	ActorUserID  *string   `gorm:"column:actor_user_id"`
	Status       string    `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (payoutRow) TableName() string { return "commission_payouts" }

type Service struct {
	db     *gorm.DB
	outbox *outbox.Service
}

func New(db *gorm.DB, publisher *outbox.Service) *Service {
	return &Service{db: db, outbox: publisher}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "commission", sub
}

func (s *Service) Seed(ctx context.Context) error {
	row := policyRow{
		ID: "plc_m6_default", ScopeType: "platform", ScopeID: "*", Version: PolicyM6,
		DirectBPS: DefaultDirect, OverrideBPS: DefaultOver, ChannelBPS: DefaultChan, TeamBPS: DefaultTeam,
		CapBPS: DefaultCap, FreezeDays: FreezeDays, MinSettleMinor: billing.MinorPerUSD, Status: "active",
		CreatedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Where("id = ?", row.ID).FirstOrCreate(&row).Error
}

func (s *Service) ActivePolicy(ctx context.Context) (*PolicyView, error) {
	var row policyRow
	if err := s.db.WithContext(ctx).Where("status = ?", "active").Order("created_at DESC").First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return policyView(row), nil
}

func policyView(row policyRow) *PolicyView {
	return &PolicyView{
		ID: row.ID, Version: row.Version, DirectBPS: row.DirectBPS, OverrideBPS: row.OverrideBPS,
		ChannelBPS: row.ChannelBPS, TeamBPS: row.TeamBPS, CapBPS: row.CapBPS,
		FreezeDays: row.FreezeDays, MinSettleMinor: row.MinSettleMinor,
	}
}

func validatePolicy(in PolicyView) error {
	if in.DirectBPS < 0 || in.OverrideBPS < 0 || in.ChannelBPS < 0 || in.TeamBPS < 0 {
		return ErrInvalid
	}
	if in.CapBPS <= 0 || in.DirectBPS+in.OverrideBPS+in.ChannelBPS+in.TeamBPS > in.CapBPS {
		return ErrInvalid
	}
	if in.FreezeDays < 0 || in.FreezeDays > 90 {
		return ErrInvalid
	}
	if in.MinSettleMinor < 0 {
		return ErrInvalid
	}
	return nil
}

func (s *Service) UpdatePolicy(ctx context.Context, in PolicyView) (*PolicyView, error) {
	if err := validatePolicy(in); err != nil {
		return nil, err
	}
	var row policyRow
	if err := s.db.WithContext(ctx).Where("status = ?", "active").Order("created_at DESC").First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	row.DirectBPS = in.DirectBPS
	row.OverrideBPS = in.OverrideBPS
	row.ChannelBPS = in.ChannelBPS
	row.TeamBPS = in.TeamBPS
	row.CapBPS = in.CapBPS
	if in.FreezeDays > 0 {
		row.FreezeDays = in.FreezeDays
	}
	row.MinSettleMinor = in.MinSettleMinor
	if in.Version != "" {
		row.Version = in.Version
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return policyView(row), nil
}

func (s *Service) Accrue(ctx context.Context, in AccrueInput) (int64, error) {
	if in.UsageEventID == "" || in.WholesaleMinor <= 0 {
		return 0, nil
	}
	var existing []entryRow
	if err := s.db.WithContext(ctx).Where("usage_event_id = ? AND status <> ?", in.UsageEventID, StatusReversed).Find(&existing).Error; err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		var sum int64
		for _, row := range existing {
			sum += row.AmountMinor
		}
		return sum, nil
	}
	policy, err := s.ActivePolicy(ctx)
	if err != nil {
		return 0, err
	}
	splits := splitCommission(in, policy)
	now := time.Now().UTC()
	avail := now.AddDate(0, 0, policy.FreezeDays)
	var total int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, part := range splits {
			if part.Amount == 0 && part.Raw == 0 {
				continue
			}
			row := entryRow{
				ID: id.New("cme"), UsageEventID: in.UsageEventID, Kind: part.Kind,
				PolicyVersion: policy.Version, BaseAmountMinor: in.WholesaleMinor,
				RawAmountMinor: part.Raw, AmountMinor: part.Amount, Status: StatusFrozen,
				AvailableAt: &avail, IdempotencyKey: "cme:" + in.UsageEventID + ":" + part.Kind,
				CreatedAt: now,
			}
			if in.RequestID != "" {
				row.RequestID = &in.RequestID
			}
			if in.UserID != "" {
				row.UserID = &in.UserID
			}
			if in.ChannelOrgID != "" {
				row.ChannelOrgID = &in.ChannelOrgID
			}
			if part.Beneficiary != "" {
				row.BeneficiaryRoleID = &part.Beneficiary
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			total += part.Amount
		}
		if s.outbox != nil && total > 0 {
			_, err := s.outbox.EnqueueTx(tx, "commission.accrual.frozen", "usage_event", in.UsageEventID, map[string]any{
				"amount_minor": total, "policy_version": policy.Version,
			})
			return err
		}
		return nil
	})
	return total, err
}

type split struct {
	Kind        string
	Beneficiary string
	Raw         int64
	Amount      int64
}

func splitCommission(in AccrueInput, policy *PolicyView) []split {
	base := in.WholesaleMinor
	direct := base * int64(policy.DirectBPS) / 10000
	over := base * int64(policy.OverrideBPS) / 10000
	channel := base * int64(policy.ChannelBPS) / 10000
	team := base * int64(policy.TeamBPS) / 10000
	if in.RoleID == "" {
		// 无推广角色时保持 M3 口径：批发价 10% 记到渠道。
		channel = base * int64(billing.CommissionRateBPS) / 10000
		direct, over, team = 0, 0, 0
	}
	parts := []split{
		{Kind: KindTeam, Raw: team, Amount: team},
		{Kind: KindChannel, Beneficiary: "", Raw: channel, Amount: channel},
		{Kind: KindOverride, Beneficiary: in.ParentRoleID, Raw: over, Amount: over},
		{Kind: KindDirect, Beneficiary: in.RoleID, Raw: direct, Amount: direct},
	}
	if in.RoleType == AcqTypeAgent || in.ParentRoleID == "" {
		parts[2].Amount = 0
		parts[2].Raw = 0
	}
	cap := base * int64(policy.CapBPS) / 10000
	total := parts[0].Amount + parts[1].Amount + parts[2].Amount + parts[3].Amount
	for i := 0; i < len(parts) && total > cap; i++ {
		cut := total - cap
		if cut > parts[i].Amount {
			cut = parts[i].Amount
		}
		parts[i].Amount -= cut
		total -= cut
	}
	return parts
}

const AcqTypeAgent = "agent"

func (s *Service) Reverse(ctx context.Context, usageEventID string) error {
	if usageEventID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []entryRow
		if err := tx.Where("usage_event_id = ? AND status <> ?", usageEventID, StatusReversed).Find(&rows).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for i := range rows {
			rev := entryRow{
				ID: id.New("cme"), UsageEventID: usageEventID, Kind: rows[i].Kind,
				PolicyVersion: rows[i].PolicyVersion, BaseAmountMinor: rows[i].BaseAmountMinor,
				RawAmountMinor: -rows[i].RawAmountMinor, AmountMinor: -rows[i].AmountMinor,
				Status: StatusReversed, ReversalOf: &rows[i].ID,
				IdempotencyKey: "cme-rev:" + rows[i].ID, CreatedAt: now,
				ChannelOrgID: rows[i].ChannelOrgID, BeneficiaryRoleID: rows[i].BeneficiaryRoleID,
				RequestID: rows[i].RequestID, UserID: rows[i].UserID,
			}
			if err := tx.Where("idempotency_key = ?", rev.IdempotencyKey).FirstOrCreate(&rev).Error; err != nil {
				return err
			}
			rows[i].Status = StatusReversed
			if err := tx.Save(&rows[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) Unfreeze(ctx context.Context, now time.Time) (int, error) {
	res := s.db.WithContext(ctx).Model(&entryRow{}).
		Where("status = ? AND available_at IS NOT NULL AND available_at <= ?", StatusFrozen, now).
		Updates(map[string]any{"status": StatusAvailable})
	return int(res.RowsAffected), res.Error
}

// HoldUnsettledForUser 把该用户产生的未结算佣金标成 held，封禁后不再进入结算。
func (s *Service) HoldUnsettledForUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	res := s.db.WithContext(ctx).Model(&entryRow{}).
		Where("user_id = ? AND status IN ?", userID, []string{StatusFrozen, StatusAvailable}).
		Updates(map[string]any{"status": StatusHeld})
	return int(res.RowsAffected), res.Error
}

// ReleaseHeldForUser 解封后按冻结截止时间把 held 恢复成 frozen 或 available。
func (s *Service) ReleaseHeldForUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	now := time.Now().UTC()
	avail := s.db.WithContext(ctx).Model(&entryRow{}).
		Where("user_id = ? AND status = ? AND available_at IS NOT NULL AND available_at <= ?", userID, StatusHeld, now).
		Updates(map[string]any{"status": StatusAvailable})
	if avail.Error != nil {
		return 0, avail.Error
	}
	frozen := s.db.WithContext(ctx).Model(&entryRow{}).
		Where("user_id = ? AND status = ?", userID, StatusHeld).
		Updates(map[string]any{"status": StatusFrozen})
	return int(avail.RowsAffected + frozen.RowsAffected), frozen.Error
}

func (s *Service) ForceAvailableAt(ctx context.Context, usageEventID string, at time.Time) error {
	return s.db.WithContext(ctx).Model(&entryRow{}).Where("usage_event_id = ?", usageEventID).
		Update("available_at", at).Error
}

func (s *Service) CreateMonthlySettlement(ctx context.Context, now time.Time, ignoreMinimum bool) ([]SettlementView, error) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	policy, err := s.ActivePolicy(ctx)
	if err != nil {
		return nil, err
	}
	type group struct {
		ChannelOrgID      *string
		BeneficiaryRoleID *string
		Total             int64
	}
	var groups []group
	if err := s.db.WithContext(ctx).Model(&entryRow{}).
		Select("channel_org_id, beneficiary_role_id, COALESCE(SUM(amount_minor),0) AS total").
		Where("status = ?", StatusAvailable).
		Group("channel_org_id, beneficiary_role_id").
		Scan(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]SettlementView, 0)
	for _, g := range groups {
		if !ignoreMinimum && g.Total < policy.MinSettleMinor {
			continue
		}
		row := settleRow{
			ID: id.New("csl"), PeriodStart: start, PeriodEnd: end, AmountMinor: g.Total,
			Status: StatusSettled, PolicyVersion: policy.Version, CreatedAt: now,
			ChannelOrgID: g.ChannelOrgID, BeneficiaryRoleID: g.BeneficiaryRoleID,
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			q := tx.Model(&entryRow{}).Where("status = ?", StatusAvailable)
			if g.ChannelOrgID != nil {
				q = q.Where("channel_org_id = ?", *g.ChannelOrgID)
			} else {
				q = q.Where("channel_org_id IS NULL")
			}
			if g.BeneficiaryRoleID != nil {
				q = q.Where("beneficiary_role_id = ?", *g.BeneficiaryRoleID)
			} else {
				q = q.Where("beneficiary_role_id IS NULL")
			}
			return q.Updates(map[string]any{"status": StatusSettled, "settlement_id": row.ID}).Error
		}); err != nil {
			return nil, err
		}
		out = append(out, *settleView(row))
	}
	return out, nil
}

func (s *Service) Payout(ctx context.Context, settlementID, method, reference, actor string) (*SettlementView, error) {
	var row settleRow
	if err := s.db.WithContext(ctx).Where("id = ?", settlementID).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	if row.Status == StatusPaid {
		return settleView(row), nil
	}
	if row.Status != StatusSettled {
		return nil, ErrInvalid
	}
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row.Status = StatusPaid
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		payout := payoutRow{
			ID: id.New("cpo"), SettlementID: settlementID, Method: method, Status: StatusPaid, CreatedAt: now,
		}
		if reference != "" {
			payout.Reference = &reference
		}
		if actor != "" {
			payout.ActorUserID = &actor
		}
		if err := tx.Create(&payout).Error; err != nil {
			return err
		}
		return tx.Model(&entryRow{}).Where("settlement_id = ?", settlementID).Update("status", StatusPaid).Error
	})
	if err != nil {
		return nil, err
	}
	return settleView(row), nil
}

func (s *Service) ListEntries(ctx context.Context, channelID string, roleIDs []string, usageEventID string) ([]EntryView, error) {
	q := s.db.WithContext(ctx).Model(&entryRow{}).Order("created_at DESC").Limit(200)
	if usageEventID != "" {
		q = q.Where("usage_event_id = ?", usageEventID)
	}
	if channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	if len(roleIDs) > 0 {
		q = q.Where("beneficiary_role_id IN ?", roleIDs)
	}
	var rows []entryRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EntryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *entryView(row))
	}
	return out, nil
}

func (s *Service) ListSettlements(ctx context.Context, channelID string, roleIDs []string) ([]SettlementView, error) {
	q := s.db.WithContext(ctx).Model(&settleRow{}).Order("created_at DESC").Limit(100)
	if channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	if len(roleIDs) > 0 {
		q = q.Where("beneficiary_role_id IN ?", roleIDs)
	}
	var rows []settleRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SettlementView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *settleView(row))
	}
	return out, nil
}

func (s *Service) RunUnfreeze(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.Unfreeze(ctx, time.Now().UTC())
		}
	}
}

func entryView(row entryRow) *EntryView {
	view := &EntryView{
		ID: row.ID, UsageEventID: row.UsageEventID, Kind: row.Kind,
		AmountMinor: row.AmountMinor, RawAmountMinor: row.RawAmountMinor,
		Status: row.Status, PolicyVersion: row.PolicyVersion, AvailableAt: row.AvailableAt,
	}
	if row.RequestID != nil {
		view.RequestID = *row.RequestID
	}
	if row.BeneficiaryRoleID != nil {
		view.BeneficiaryRoleID = *row.BeneficiaryRoleID
	}
	if row.ChannelOrgID != nil {
		view.ChannelOrgID = *row.ChannelOrgID
	}
	if row.SettlementID != nil {
		view.SettlementID = *row.SettlementID
	}
	return view
}

func settleView(row settleRow) *SettlementView {
	view := &SettlementView{
		ID: row.ID, PeriodStart: row.PeriodStart, PeriodEnd: row.PeriodEnd,
		AmountMinor: row.AmountMinor, Status: row.Status, PolicyVersion: row.PolicyVersion,
	}
	if row.ChannelOrgID != nil {
		view.ChannelOrgID = *row.ChannelOrgID
	}
	if row.BeneficiaryRoleID != nil {
		view.BeneficiaryRoleID = *row.BeneficiaryRoleID
	}
	return view
}
