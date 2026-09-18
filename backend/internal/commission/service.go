package commission

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	IndirectBPS    int       `gorm:"column:indirect_bps"`
	OverrideBPS    int       `gorm:"column:override_bps"`
	ChannelBPS     int       `gorm:"column:channel_bps"`
	TeamBPS        int       `gorm:"column:team_bps"`
	CapBPS         int       `gorm:"column:cap_bps"`
	TotalBPS       int       `gorm:"column:total_bps"`
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

type CashBook interface {
	PayoutCommissionTx(tx *gorm.DB, entryID, settlementID string, amount int64) error
	CreditCommissionTx(tx *gorm.DB, userID, entryID string, amount int64) error
	ReverseCommissionTx(tx *gorm.DB, entryID string) error
}

type RoleUsers interface {
	UserIDForRoleTx(tx *gorm.DB, roleID string) (string, error)
}

type Service struct {
	db     *gorm.DB
	outbox *outbox.Service
	cash   CashBook
	roles  RoleUsers
}

func New(db *gorm.DB, publisher *outbox.Service) *Service {
	return &Service{db: db, outbox: publisher}
}

func (s *Service) SetCashier(c CashBook) {
	s.cash = c
}

func (s *Service) SetRoles(r RoleUsers) {
	s.roles = r
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
		DirectBPS: DefaultDirect, IndirectBPS: DefaultIndirect, OverrideBPS: DefaultOver, ChannelBPS: DefaultChan, TeamBPS: DefaultTeam,
		CapBPS: DefaultCap, TotalBPS: DefaultTotal, FreezeDays: FreezeDays, MinSettleMinor: billing.MinorPerUSD, Status: "active",
		CreatedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Where("id = ?", row.ID).FirstOrCreate(&row).Error
}

func (s *Service) ActivePolicy(ctx context.Context) (*PolicyView, error) {
	return s.PolicyFor(ctx, "")
}

func (s *Service) PolicyFor(ctx context.Context, channelID string) (*PolicyView, error) {
	var row policyRow
	if channelID != "" {
		err := s.db.WithContext(ctx).Where("status = ? AND scope_type = ? AND scope_id = ?", "active", "channel", channelID).
			Order("created_at DESC").First(&row).Error
		if err == nil {
			return policyView(row), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("status = ? AND scope_type = ? AND scope_id = ?", "active", "platform", "*").
		Order("created_at DESC").First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return policyView(row), nil
}

func policyView(row policyRow) *PolicyView {
	return &PolicyView{
		ID: row.ID, Version: row.Version, DirectBPS: row.DirectBPS, IndirectBPS: row.IndirectBPS, OverrideBPS: row.OverrideBPS,
		ChannelBPS: row.ChannelBPS, TeamBPS: row.TeamBPS, CapBPS: row.CapBPS, TotalBPS: row.TotalBPS,
		FreezeDays: row.FreezeDays, MinSettleMinor: row.MinSettleMinor,
	}
}

func validatePolicy(in PolicyView) error {
	total := in.TotalBPS
	if total <= 0 {
		total = in.CapBPS
	}
	indirect := in.IndirectBPS
	if indirect == 0 {
		indirect = in.OverrideBPS
	}
	if in.DirectBPS < 0 || indirect < 0 {
		return ErrInvalid
	}
	if total <= 0 || in.DirectBPS+indirect > total {
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
	if err := s.db.WithContext(ctx).Where("status = ? AND scope_type = ? AND scope_id = ?", "active", "platform", "*").
		Order("created_at DESC").First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	row.DirectBPS = in.DirectBPS
	if in.IndirectBPS > 0 {
		row.IndirectBPS = in.IndirectBPS
	} else {
		row.IndirectBPS = in.OverrideBPS
	}
	row.OverrideBPS = row.IndirectBPS
	row.ChannelBPS = 0
	row.TeamBPS = 0
	if in.TotalBPS > 0 {
		row.TotalBPS = in.TotalBPS
		row.CapBPS = in.TotalBPS
	} else if in.CapBPS > 0 {
		row.CapBPS = in.CapBPS
		row.TotalBPS = in.CapBPS
	}
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

func (s *Service) UpdateChannelPolicy(ctx context.Context, channelID string, in PolicyView) (*PolicyView, error) {
	if channelID == "" {
		return nil, ErrInvalid
	}
	if err := validatePolicy(in); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var row policyRow
	err := s.db.WithContext(ctx).Where("status = ? AND scope_type = ? AND scope_id = ?", "active", "channel", channelID).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = policyRow{
			ID: id.New("plc"), ScopeType: "channel", ScopeID: channelID, Status: "active", CreatedAt: now,
		}
	} else if err != nil {
		return nil, err
	}
	row.DirectBPS = in.DirectBPS
	if in.IndirectBPS > 0 {
		row.IndirectBPS = in.IndirectBPS
	} else {
		row.IndirectBPS = in.OverrideBPS
	}
	row.OverrideBPS = row.IndirectBPS
	row.ChannelBPS = 0
	row.TeamBPS = 0
	if in.TotalBPS > 0 {
		row.TotalBPS = in.TotalBPS
		row.CapBPS = in.TotalBPS
	} else if in.CapBPS > 0 {
		row.CapBPS = in.CapBPS
		row.TotalBPS = in.CapBPS
	}
	if in.FreezeDays > 0 {
		row.FreezeDays = in.FreezeDays
	}
	row.MinSettleMinor = in.MinSettleMinor
	if in.Version != "" {
		row.Version = in.Version
	} else if row.Version == "" {
		row.Version = "c-" + channelID
	}
	if row.ID == "" {
		row.ID = id.New("plc")
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return policyView(row), nil
}

func (s *Service) RecordSignupCredit(ctx context.Context, channelID, userID string, amount int64) error {
	if channelID == "" || userID == "" || amount <= 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return writeMarketing(tx, channelID, MarketingKindCredit, MarketingIssued, -amount, "", "", "signup_gift", userID, "mkt-gift:"+userID, nil)
	})
}

func (s *Service) Accrue(ctx context.Context, in AccrueInput) (int64, error) {
	if in.UsageEventID == "" || in.WholesaleMinor <= 0 || in.RoleID == "" || !in.CanCommission {
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
	policy, err := s.PolicyFor(ctx, in.PolicyChannelID)
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
			if in.ChannelOrgID != "" {
				if err := writeMarketing(tx, in.ChannelOrgID, MarketingKindCommission, MarketingFrozen, -part.Amount, in.UsageEventID, row.ID, "usage", in.UsageEventID, "mkt-frz:"+row.ID, nil); err != nil {
					return err
				}
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
	indirectBPS := policy.IndirectBPS
	if indirectBPS == 0 {
		indirectBPS = policy.OverrideBPS
	}
	totalBPS := policy.TotalBPS
	if totalBPS <= 0 {
		totalBPS = policy.CapBPS
	}
	direct := base * int64(policy.DirectBPS) / 10000
	indirect := int64(0)
	if in.ParentRoleID != "" {
		indirect = base * int64(indirectBPS) / 10000
	}
	parts := []split{
		{Kind: KindIndirect, Beneficiary: in.ParentRoleID, Raw: indirect, Amount: indirect},
		{Kind: KindDirect, Beneficiary: in.RoleID, Raw: direct, Amount: direct},
	}
	capAmt := base * int64(totalBPS) / 10000
	sum := parts[0].Amount + parts[1].Amount
	for i := 0; i < len(parts) && sum > capAmt; i++ {
		cut := sum - capAmt
		if cut > parts[i].Amount {
			cut = parts[i].Amount
		}
		parts[i].Amount -= cut
		sum -= cut
	}
	out := make([]split, 0, 2)
	for _, p := range parts {
		if p.Amount > 0 && p.Beneficiary != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) Reverse(ctx context.Context, usageEventID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.ReverseTx(tx, usageEventID) })
}

// ReverseTx participates in the caller's charge refund transaction.
func (s *Service) ReverseTx(tx *gorm.DB, usageEventID string) error {
	if err := lockSettlementLifecycle(tx); err != nil {
		return err
	}
	if usageEventID == "" {
		return nil
	}

	var rows []entryRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").Where("usage_event_id = ? AND status <> ?", usageEventID, StatusReversed).Find(&rows).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range rows {
		origStatus := rows[i].Status
		if rows[i].SettlementID != nil {
			if err := s.cancelUnpaidSettlementTx(tx, *rows[i].SettlementID, rows[i].ID); err != nil {
				return err
			}
		}
		origAmt := rows[i].AmountMinor
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
		ch := ""
		if rows[i].ChannelOrgID != nil {
			ch = *rows[i].ChannelOrgID
		}
		st := MarketingFrozen
		if origStatus == StatusAvailable || origStatus == StatusPaid || origStatus == StatusSettled {
			st = MarketingIssued
		} else if origStatus == StatusHeld {
			// A hold does not undo a prior unfreeze. Preserve its original marketing bucket.
			var issued int64
			if err := tx.Model(&marketingRow{}).Where("idempotency_key = ?", "mkt-unfrz-iss:"+rows[i].ID).Count(&issued).Error; err != nil {
				return err
			}
			if issued > 0 {
				st = MarketingIssued
			}
		}
		if err := writeMarketing(tx, ch, MarketingKindCommission, st, origAmt, usageEventID, rev.ID, "reversal", rows[i].ID, "mkt-rev:"+rows[i].ID, &rows[i].ID); err != nil {
			return err
		}
		if s.cash != nil && (origStatus == StatusAvailable || origStatus == StatusPaid || origStatus == StatusSettled || origStatus == StatusHeld) {
			if err := s.cash.ReverseCommissionTx(tx, rows[i].ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Unfreeze(ctx context.Context, now time.Time) (int, error) {
	return s.UnfreezeUsage(ctx, now, "")
}
func (s *Service) UnfreezeUsage(ctx context.Context, now time.Time, usageEventID string) (int, error) {
	n := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		var err error
		n, err = s.unfreezeTx(tx, now, usageEventID, nil)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Caller holds the lifecycle lock; status, marketing transfer and cash credit commit together.
func (s *Service) unfreezeTx(tx *gorm.DB, now time.Time, usageEventID string, entryIDs []string) (int, error) {
	n := 0
	var rows []entryRow
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").Where("status = ? AND available_at IS NOT NULL AND available_at <= ?", StatusFrozen, now)
	if usageEventID != "" {
		q = q.Where("usage_event_id = ?", usageEventID)
	}
	if len(entryIDs) > 0 {
		q = q.Where("id IN ?", entryIDs)
	}
	if err := q.Find(&rows).Error; err != nil {
		return n, err
	}
	for i := range rows {
		rows[i].Status = StatusAvailable
		if err := tx.Save(&rows[i]).Error; err != nil {
			return n, err
		}
		ch := ""
		if rows[i].ChannelOrgID != nil {
			ch = *rows[i].ChannelOrgID
		}
		if err := writeMarketing(tx, ch, MarketingKindCommission, MarketingFrozen, rows[i].AmountMinor, rows[i].UsageEventID, rows[i].ID, "unfreeze", rows[i].ID, "mkt-unfrz-frz:"+rows[i].ID, nil); err != nil {
			return n, err
		}
		if err := writeMarketing(tx, ch, MarketingKindCommission, MarketingIssued, -rows[i].AmountMinor, rows[i].UsageEventID, rows[i].ID, "unfreeze", rows[i].ID, "mkt-unfrz-iss:"+rows[i].ID, nil); err != nil {
			return n, err
		}
		if s.cash != nil && rows[i].AmountMinor > 0 {
			uid, err := s.beneficiaryUser(tx, rows[i])
			if err != nil {
				return n, err
			}
			if uid != "" {
				if err := s.cash.CreditCommissionTx(tx, uid, rows[i].ID, rows[i].AmountMinor); err != nil {
					return n, err
				}
			}
		}
		n++
	}

	return n, nil
}

// SetUserHoldTx participates in the same transaction as the identity status change.
func (s *Service) SetUserHoldTx(tx *gorm.DB, userID string, held bool) error {
	scoped := *s
	scoped.db = tx
	if held {
		_, err := scoped.HoldUnsettledForUser(tx.Statement.Context, userID)
		return err
	}
	_, err := scoped.ReleaseHeldForUser(tx.Statement.Context, userID)
	return err
}

// HoldUnsettledForUser 把该用户产生的未结算佣金标成 held，封禁后不再进入结算。
func (s *Service) HoldUnsettledForUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	n := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		res := tx.Model(&entryRow{}).Where("user_id = ? AND status IN ?", userID, []string{StatusFrozen, StatusAvailable}).Update("status", StatusHeld)
		n = int(res.RowsAffected)
		return res.Error
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Release only the held rows, then run the normal due-date and accounting transition.
// Existing unfreeze/credit keys make an already credited hold safe to release again.
func (s *Service) ReleaseHeldForUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}
	n := 0
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		var rows []entryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND status = ?", userID, StatusHeld).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if err := tx.Model(&entryRow{}).Where("id IN ?", ids).Update("status", StatusFrozen).Error; err != nil {
			return err
		}
		if _, err := s.unfreezeTx(tx, time.Now().UTC(), "", ids); err != nil {
			return err
		}
		n = len(rows)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Service) ForceAvailableAt(ctx context.Context, usageEventID string, at time.Time) error {
	return s.db.WithContext(ctx).Model(&entryRow{}).Where("usage_event_id = ?", usageEventID).
		Update("available_at", at).Error
}

// Settlement transitions share a transaction-scoped lock. Monthly runs and manual
// payout registration are infrequent; serialization also excludes concurrent refunds.
func lockSettlementLifecycle(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "commission-settlement-lifecycle").Error
}

func (s *Service) CreateMonthlySettlement(ctx context.Context, now time.Time, ignoreMinimum bool) ([]SettlementView, error) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	policy, err := s.ActivePolicy(ctx)
	if err != nil {
		return nil, err
	}
	out := []SettlementView{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		var entries []entryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status = ?", StatusAvailable).Order("id").Find(&entries).Error; err != nil {
			return err
		}
		type group struct {
			channel, role *string
			amount        int64
			ids           []string
		}
		groups := map[[2]string]*group{}
		order := [][2]string{}
		for _, e := range entries {
			key := [2]string{}
			if e.ChannelOrgID != nil {
				key[0] = *e.ChannelOrgID
			}
			if e.BeneficiaryRoleID != nil {
				key[1] = *e.BeneficiaryRoleID
			}
			if groups[key] == nil {
				groups[key] = &group{channel: e.ChannelOrgID, role: e.BeneficiaryRoleID}
				order = append(order, key)
			}
			g := groups[key]
			g.amount += e.AmountMinor
			g.ids = append(g.ids, e.ID)
		}
		for _, key := range order {
			g := groups[key]
			if g.amount <= 0 || (!ignoreMinimum && g.amount < policy.MinSettleMinor) {
				continue
			}
			row := settleRow{ID: id.New("csl"), PeriodStart: start, PeriodEnd: end, AmountMinor: g.amount, Status: StatusSettled, PolicyVersion: policy.Version, CreatedAt: now, ChannelOrgID: g.channel, BeneficiaryRoleID: g.role}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Model(&entryRow{}).Where("id IN ?", g.ids).Updates(map[string]any{"status": StatusSettled, "settlement_id": row.ID}).Error; err != nil {
				return err
			}
			if _, err := s.outbox.EnqueueTx(tx, "commission.settlement.created", "commission_settlement", row.ID, map[string]any{"entry_ids": g.ids, "amount_minor": g.amount}); err != nil {
				return err
			}
			out = append(out, *settleView(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) Payout(ctx context.Context, settlementID, method, reference, actor string) (*SettlementView, error) {
	method, reference = strings.TrimSpace(method), strings.TrimSpace(reference)
	if method != "manual" || reference == "" || len(reference) > 200 {
		return nil, ErrInvalid
	}
	var out *SettlementView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSettlementLifecycle(tx); err != nil {
			return err
		}
		var row settleRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", settlementID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.Status == StatusPaid {
			var paid payoutRow
			if err := tx.Where("settlement_id = ?", settlementID).First(&paid).Error; err != nil {
				return err
			}
			if paid.Method != method || paid.Reference == nil || *paid.Reference != reference {
				return ErrConflict
			}
			out = settleView(row)
			out.PayoutReference = reference
			out.PayoutMethod = method
			return nil
		}
		if row.Status != StatusSettled {
			return ErrSettlementChanged
		}
		var entries []entryRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("settlement_id = ?", settlementID).Order("id").Find(&entries).Error; err != nil {
			return err
		}
		var total int64
		for _, e := range entries {
			if e.Status != StatusSettled {
				return ErrSettlementChanged
			}
			total += e.AmountMinor
		}
		if len(entries) == 0 || total <= 0 || total != row.AmountMinor {
			return ErrSettlementChanged
		}
		if s.cash != nil {
			for _, entry := range entries {
				if err := s.cash.PayoutCommissionTx(tx, entry.ID, settlementID, entry.AmountMinor); err != nil {
					if errors.Is(err, billing.ErrNotFound) || errors.Is(err, billing.ErrInsufficientBalance) || errors.Is(err, billing.ErrConflict) {
						return ErrWalletMismatch
					}
					return err
				}
			}
		}
		payout := payoutRow{ID: id.New("cpo"), SettlementID: settlementID, Method: method, Reference: &reference, Status: StatusPaid, CreatedAt: time.Now().UTC()}
		if actor != "" {
			payout.ActorUserID = &actor
		}
		if err := tx.Create(&payout).Error; err != nil {
			return err
		}
		row.Status = StatusPaid
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&entryRow{}).Where("settlement_id = ? AND status = ?", settlementID, StatusSettled).Update("status", StatusPaid).Error; err != nil {
			return err
		}
		out = settleView(row)
		out.PayoutReference = reference
		out.PayoutMethod = method
		return nil
	})
	return out, err
}

// Cancel the entire unpaid snapshot; unaffected entries can be settled afresh.
// The creation/cancellation events retain original membership for reconciliation.
func (s *Service) cancelUnpaidSettlementTx(tx *gorm.DB, settlementID, reversedEntryID string) error {
	var row settleRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", settlementID).First(&row).Error; err != nil {
		return err
	}
	if row.Status != StatusSettled {
		return nil
	}
	var ids []string
	if err := tx.Model(&entryRow{}).Where("settlement_id = ? AND status = ?", settlementID, StatusSettled).Pluck("id", &ids).Error; err != nil {
		return err
	}
	row.Status = StatusCancelled
	if err := tx.Save(&row).Error; err != nil {
		return err
	}
	if err := tx.Model(&entryRow{}).Where("settlement_id = ? AND status = ?", settlementID, StatusSettled).Updates(map[string]any{"status": StatusAvailable, "settlement_id": nil}).Error; err != nil {
		return err
	}
	_, err := s.outbox.EnqueueTx(tx, "commission.settlement.cancelled", "commission_settlement", row.ID, map[string]any{"entry_ids": ids, "reversed_entry_id": reversedEntryID, "reason": "commission_reversal"})
	return err
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
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var payouts []payoutRow
	if err := s.db.WithContext(ctx).Where("settlement_id IN ?", ids).Find(&payouts).Error; err != nil {
		return nil, err
	}
	paidByID := make(map[string]payoutRow, len(payouts))
	for _, paid := range payouts {
		paidByID[paid.SettlementID] = paid
	}
	var reversals []struct {
		SettlementID string
		AmountMinor  int64
	}
	if err := s.db.WithContext(ctx).Model(&entryRow{}).Where("settlement_id IN ? AND status = ? AND reversal_of IS NULL", ids, StatusReversed).Select("settlement_id, SUM(amount_minor) AS amount_minor").Group("settlement_id").Scan(&reversals).Error; err != nil {
		return nil, err
	}
	reversedByID := make(map[string]int64, len(reversals))
	for _, reversal := range reversals {
		reversedByID[reversal.SettlementID] = reversal.AmountMinor
	}
	for _, row := range rows {
		view := settleView(row)
		if paid, ok := paidByID[row.ID]; ok {
			view.PayoutMethod = paid.Method
			if paid.Reference != nil {
				view.PayoutReference = *paid.Reference
			}
		}
		view.ReversedMinor = reversedByID[row.ID]
		out = append(out, *view)
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

func (s *Service) beneficiaryUser(tx *gorm.DB, row entryRow) (string, error) {
	if s.roles == nil || row.BeneficiaryRoleID == nil {
		return "", nil
	}
	return s.roles.UserIDForRoleTx(tx, *row.BeneficiaryRoleID)
}

func entryView(row entryRow) *EntryView {
	view := &EntryView{
		ID: row.ID, UsageEventID: row.UsageEventID, Kind: row.Kind,
		AmountMinor: row.AmountMinor, RawAmountMinor: row.RawAmountMinor,
		Status: row.Status, PolicyVersion: row.PolicyVersion, AvailableAt: row.AvailableAt,
	}
	if row.ReversalOf != nil {
		view.ReversalOf = *row.ReversalOf
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

// Totals separates outstanding liability from expense, which still includes paid commissions.
func (s *Service) Totals(ctx context.Context) (liability, expense int64, err error) {
	var totals struct {
		Liability int64
		Expense   int64
	}
	err = s.db.WithContext(ctx).Model(&entryRow{}).Select(`
 COALESCE(SUM(CASE WHEN status IN ('frozen','available','held','settled') THEN amount_minor ELSE 0 END),0) AS liability,
 COALESCE(SUM(CASE WHEN status <> 'reversed' THEN amount_minor ELSE 0 END),0) AS expense
 `).Scan(&totals).Error
	return totals.Liability, totals.Expense, err
}
