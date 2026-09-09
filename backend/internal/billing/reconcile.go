package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DiffMatch    = "match"
	DiffMismatch = "mismatch"
	ScopeUser    = "user"
	ScopeChannel = "channel"
)

// ReconcileInput 只走 TokenHub billing 接口：Balance / ChannelQuota / QueryUsage / ListChargesByRequest / ListLedger。
type ReconcileInput struct {
	UserID        string
	ChannelOrgID  string
	Scope         string
	APIKeyID      string
	PublicModelID string
	Since         time.Time
	Until         time.Time
	Limit         int
}

// FlagPendingInput 把差异行送进 pending_reconciliation。禁止估算扣款。
type FlagPendingInput struct {
	Key          string
	UserID       string
	ChannelOrgID string
}

// BucketView 是三桶：余额 / 冻结 / 可提现。
type BucketView struct {
	AvailableMinor    int64 `json:"available_minor"`
	ReservedMinor     int64 `json:"reserved_minor"`
	WithdrawableMinor int64 `json:"withdrawable_minor"`
}

// WindowUsageTotals 与三桶同一窗口的 usage 合计。
type WindowUsageTotals struct {
	Requests      int64 `json:"requests"`
	CustomerMinor int64 `json:"customer_minor"`
	ChargeMinor   int64 `json:"charge_minor"`
	ReservedMinor int64 `json:"reserved_minor"`
	PendingCount  int64 `json:"pending_count"`
}

// DiffRow 是 usage ↔ charge/预授权 的一行。
type DiffRow struct {
	RequestID      string    `json:"request_id"`
	UsageID        string    `json:"usage_id,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
	PublicModelID  string    `json:"public_model_id,omitempty"`
	APIKeyID       string    `json:"api_key_id,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	ChannelOrgID   string    `json:"channel_org_id,omitempty"`
	State          string    `json:"state"`
	UsageMinor     int64     `json:"usage_minor"`
	ChargeMinor    int64     `json:"charge_minor"`
	LedgerDebit    int64     `json:"ledger_debit_minor"`
	ReservedMinor  int64     `json:"reserved_minor"`
	Match          bool      `json:"match"`
	Status         string    `json:"status"`
	AlreadyPending bool      `json:"already_pending"`
	MissingUsage   bool      `json:"missing_usage"`
	ChargeCount    int       `json:"charge_count"`
}

// ReconcileView 是用户台 / 渠道台同构对账页的账本真相。
type ReconcileView struct {
	Buckets     BucketView        `json:"buckets"`
	UsageTotals WindowUsageTotals `json:"usage_totals"`
	Items       []DiffRow         `json:"items"`
	Pending     []UsageGapView    `json:"pending"`
}

type diffInput struct {
	RequestID     string
	UsageID       string
	OccurredAt    time.Time
	PublicModelID string
	APIKeyID      string
	UserID        string
	ChannelOrgID  string
	State         string
	UsageMinor    int64
	ChargeMinor   int64
	LedgerDebit   int64
	ReservedMinor int64
	MissingUsage  bool
	ChargeCount   int
	AuthStatus    string
}

func classifyDiff(in diffInput) DiffRow {
	row := DiffRow{
		RequestID:      in.RequestID,
		UsageID:        in.UsageID,
		OccurredAt:     in.OccurredAt,
		PublicModelID:  in.PublicModelID,
		APIKeyID:       in.APIKeyID,
		UserID:         in.UserID,
		ChannelOrgID:   in.ChannelOrgID,
		State:          in.State,
		UsageMinor:     in.UsageMinor,
		ChargeMinor:    in.ChargeMinor,
		LedgerDebit:    in.LedgerDebit,
		ReservedMinor:  in.ReservedMinor,
		MissingUsage:   in.MissingUsage || in.State == UsagePending,
		AlreadyPending: in.State == UsagePending || in.AuthStatus == AuthPendingReconciliation,
		ChargeCount:    in.ChargeCount,
		Status:         DiffMismatch,
	}
	if row.MissingUsage || row.AlreadyPending {
		return row
	}
	if in.ChargeCount > 1 {
		return row
	}
	if in.State == UsageConfirmed && in.ChargeCount == 1 && in.ChargeMinor == in.UsageMinor && in.ReservedMinor == 0 {
		row.Match = true
		row.Status = DiffMatch
	}
	return row
}

// ReconcileWindow 用 billing 自己的 Balance / ChannelQuota / QueryUsage / ListChargesByRequest / ListLedger 对账。
// 不读 commission / identity / gateway 表。
func (s *Service) ReconcileWindow(ctx context.Context, in ReconcileInput) (*ReconcileView, error) {
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	view := &ReconcileView{Items: []DiffRow{}, Pending: []UsageGapView{}}
	if in.Scope == ScopeChannel {
		quota, err := s.ChannelQuota(ctx, in.ChannelOrgID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if quota != nil {
			view.Buckets = BucketView{
				AvailableMinor: quota.AvailableMinor,
				ReservedMinor:  quota.ReservedMinor,
			}
		}
	} else {
		bal, err := s.Balance(ctx, in.UserID, in.ChannelOrgID)
		if err != nil {
			return nil, err
		}
		view.Buckets = BucketView{
			AvailableMinor:    bal.AvailableMinor,
			ReservedMinor:     bal.ReservedMinor,
			WithdrawableMinor: bal.CommissionAvailableMinor,
		}
	}

	usages, err := s.QueryUsage(ctx, QueryUsageInput{
		UserID:        in.UserID,
		ChannelOrgID:  in.ChannelOrgID,
		APIKeyID:      in.APIKeyID,
		PublicModelID: in.PublicModelID,
		Since:         in.Since,
		Until:         in.Until,
		Limit:         in.Limit,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(usages))
	for _, usage := range usages {
		ids = append(ids, usage.RequestID)
	}
	charges, err := s.chargesByRequests(ctx, ids)
	if err != nil {
		return nil, err
	}
	debits, err := s.ledgerDebitsByRequests(ctx, ids)
	if err != nil {
		return nil, err
	}
	auths, err := s.authsByRequests(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, usage := range usages {
		chargeRows := charges[usage.RequestID]
		var chargeMinor int64
		for _, charge := range chargeRows {
			if charge.State == ChargeReversed {
				continue
			}
			chargeMinor += charge.AmountMinor
		}
		auth := auths[usage.RequestID]
		reserved := int64(0)
		if auth.ID != "" && (auth.Status == AuthReserved || auth.Status == AuthPendingReconciliation) {
			reserved = auth.AmountMinor
		}
		row := classifyDiff(diffInput{
			RequestID:     usage.RequestID,
			UsageID:       usage.ID,
			OccurredAt:    usage.OccurredAt,
			PublicModelID: usage.PublicModelID,
			APIKeyID:      usage.APIKeyID,
			UserID:        usage.UserID,
			ChannelOrgID:  usage.ChannelOrgID,
			State:         usage.State,
			UsageMinor:    usage.CustomerMinor,
			ChargeMinor:   chargeMinor,
			LedgerDebit:   debits[usage.RequestID],
			ReservedMinor: reserved,
			MissingUsage:  usageMissing(usage.UnitUsage) || usage.State == UsagePending,
			ChargeCount:   len(chargeRows),
			AuthStatus:    auth.Status,
		})
		view.Items = append(view.Items, row)
		view.UsageTotals.Requests++
		view.UsageTotals.CustomerMinor += row.UsageMinor
		view.UsageTotals.ChargeMinor += row.ChargeMinor
		view.UsageTotals.ReservedMinor += row.ReservedMinor
		if !row.Match {
			view.UsageTotals.PendingCount++
		}
	}

	pendingIn := QueryUsageInput{
		UserID: in.UserID, ChannelOrgID: in.ChannelOrgID, APIKeyID: in.APIKeyID,
		PublicModelID: in.PublicModelID, Since: in.Since, Until: in.Until, Limit: in.Limit,
	}
	pending, err := s.ListPendingReconciliation(ctx, pendingIn)
	if err != nil {
		return nil, err
	}
	view.Pending = pending
	return view, nil
}

// FlagPending 把差异送进 pending_reconciliation。匹配行拒绝；已在队列则幂等返回。永不写 charge / usage_debit。
func (s *Service) FlagPending(ctx context.Context, in FlagPendingInput) (*UsageGapView, error) {
	if in.Key == "" {
		return nil, ErrInvalidAmount
	}
	var row usageRow
	if err := s.db.WithContext(ctx).Where("id = ? OR request_id = ?", in.Key, in.Key).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if in.UserID != "" && row.UserID != in.UserID {
		return nil, ErrNotFound
	}
	if in.ChannelOrgID != "" && (row.ChannelOrgID == nil || *row.ChannelOrgID != in.ChannelOrgID) {
		return nil, ErrNotFound
	}
	target, err := s.diffOne(ctx, row)
	if err != nil {
		return nil, err
	}
	if target.Match {
		return nil, ErrAlreadyMatched
	}
	return s.enqueuePending(ctx, target.RequestID, target.UsageID)
}

func (s *Service) diffOne(ctx context.Context, row usageRow) (DiffRow, error) {
	views := usageViews([]usageRow{row})
	if len(views) == 0 {
		return DiffRow{}, ErrNotFound
	}
	usage := views[0]
	charges, err := s.ListChargesByRequest(ctx, usage.RequestID)
	if err != nil {
		return DiffRow{}, err
	}
	debits, err := s.ledgerDebitsByRequests(ctx, []string{usage.RequestID})
	if err != nil {
		return DiffRow{}, err
	}
	auths, err := s.authsByRequests(ctx, []string{usage.RequestID})
	if err != nil {
		return DiffRow{}, err
	}
	chargeMinor := int64(0)
	live := 0
	for _, charge := range charges {
		if charge.State == ChargeReversed {
			continue
		}
		chargeMinor += charge.AmountMinor
		live++
	}
	auth := auths[usage.RequestID]
	reserved := int64(0)
	if auth.ID != "" && (auth.Status == AuthReserved || auth.Status == AuthPendingReconciliation) {
		reserved = auth.AmountMinor
	}
	return classifyDiff(diffInput{
		RequestID:     usage.RequestID,
		UsageID:       usage.ID,
		OccurredAt:    usage.OccurredAt,
		PublicModelID: usage.PublicModelID,
		APIKeyID:      usage.APIKeyID,
		UserID:        usage.UserID,
		ChannelOrgID:  usage.ChannelOrgID,
		State:         usage.State,
		UsageMinor:    usage.CustomerMinor,
		ChargeMinor:   chargeMinor,
		LedgerDebit:   debits[usage.RequestID],
		ReservedMinor: reserved,
		MissingUsage:  usageMissing(row.UnitUsage) || usage.State == UsagePending,
		ChargeCount:   live,
		AuthStatus:    auth.Status,
	}), nil
}

func (s *Service) enqueuePending(ctx context.Context, requestID, usageID string) (*UsageGapView, error) {
	var out *UsageGapView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row usageRow
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"})
		if usageID != "" {
			q = q.Where("id = ? OR request_id = ?", usageID, requestID)
		} else {
			q = q.Where("request_id = ?", requestID)
		}
		if err := q.First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.State != UsagePending {
			row.State = UsagePending
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			if _, err := s.outbox.EnqueueTx(tx, "usage.reconciliation.flagged", "usage_event", row.ID, map[string]any{
				"request_id": row.RequestID,
			}); err != nil {
				return err
			}
		}
		var auth authRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", row.RequestID).First(&auth).Error; err == nil {
			if auth.Status == AuthReserved {
				auth.Status = AuthPendingReconciliation
				if err := tx.Save(&auth).Error; err != nil {
					return err
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		out = gapFrom(row, auth)
		return nil
	})
	return out, err
}

func (s *Service) chargesByRequests(ctx context.Context, ids []string) (map[string][]Settlement, error) {
	out := map[string][]Settlement{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []chargeRow
	if err := s.db.WithContext(ctx).Where("request_id IN ?", ids).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, charge := range rows {
		out[charge.RequestID] = append(out[charge.RequestID], Settlement{
			ChargeID: charge.ID, UsageEventID: charge.UsageEventID,
			AmountMinor: charge.AmountMinor, State: charge.Status, Currency: CurrencyUSD,
		})
	}
	return out, nil
}

func (s *Service) ledgerDebitsByRequests(ctx context.Context, ids []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []ledgerRow
	if err := s.db.WithContext(ctx).
		Where("event_type = ? AND reference_id IN ?", EventUsageDebit, ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ReferenceID] += row.AmountMinor
	}
	return out, nil
}

func (s *Service) authsByRequests(ctx context.Context, ids []string) (map[string]authRow, error) {
	out := map[string]authRow{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []authRow
	if err := s.db.WithContext(ctx).Where("request_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RequestID] = row
	}
	return out, nil
}
