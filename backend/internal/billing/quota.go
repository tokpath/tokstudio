package billing

import (
	"context"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AllocActive    = "active"
	AllocExhausted = "exhausted"
	AllocReclaimed = "reclaimed"
)

func skipChannelQuota(channelOrgID string) bool {
	return channelOrgID == "" || channelOrgID == identity.OfficialChannelID
}

// reserveChannelQuota 只做风险帽检查：渠道 remaining 必须盖住本次预授权。
// 额度已在用户充值发放时从 available 扣过，这里不再 available→reserved，避免同一请求双扣。
func (s *Service) reserveChannelQuota(tx *gorm.DB, userID, channelOrgID, requestID string, amount, allocationNeed int64) error {
	if skipChannelQuota(channelOrgID) {
		return nil
	}
	if err := checkAllocationRemaining(tx, userID, channelOrgID, allocationNeed); err != nil {
		return err
	}
	var quota quotaRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if quota.AvailableMinor < amount {
		return ErrInsufficientQuota
	}
	return nil
}

// settleChannelQuota 只按 FIFO 增加用户 allocation.consumed，不再扣渠道 available。
func (s *Service) settleChannelQuota(tx *gorm.DB, userID, channelOrgID, requestID string, customer int64) error {
	return consumeAllocations(tx, userID, channelOrgID, requestID, customer)
}

func (s *Service) releaseChannelQuota(tx *gorm.DB, channelOrgID, requestID string, amount int64) error {
	// 发放时已扣渠道；预授权失败/过期只需放开用户钱包，渠道库存不动。
	return nil
}

func (s *Service) ChannelQuota(ctx context.Context, channelOrgID string) (*QuotaView, error) {
	var quota quotaRow
	err := s.db.WithContext(ctx).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		return nil, ErrNotFound
	}
	view := &QuotaView{
		OwnerID: quota.OwnerID, AvailableMinor: quota.AvailableMinor,
		ReservedMinor: quota.ReservedMinor, UnitType: quota.UnitType,
		IssueRatioBPS: loadIssueRatioBPS(s.db.WithContext(ctx), channelOrgID),
	}
	var sum struct {
		Issued   int64
		Consumed int64
		Count    int64
	}
	_ = s.db.WithContext(ctx).Model(&allocationRow{}).
		Select("COALESCE(SUM(granted_minor),0) AS issued, COALESCE(SUM(consumed_minor),0) AS consumed, COUNT(*) AS count").
		Where("channel_org_id = ?", channelOrgID).
		Scan(&sum).Error
	view.IssuedMinor = sum.Issued
	view.ConsumedMinor = sum.Consumed
	view.AllocationCount = sum.Count
	return view, nil
}

func (s *Service) ListAllocations(ctx context.Context, channelOrgID string, limit int) ([]AllocationView, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []allocationRow
	q := s.db.WithContext(ctx).Order("created_at DESC").Limit(limit)
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AllocationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, allocationView(row))
	}
	return out, nil
}

func (s *Service) GrantChannelQuota(ctx context.Context, channelOrgID string, amount int64, actor string) (*QuotaView, error) {
	if amount == 0 || channelOrgID == "" {
		return nil, ErrInvalidAmount
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var quota quotaRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
			First(&quota).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			quota = quotaRow{
				ID: id.New("qta"), OwnerType: "channel", OwnerID: channelOrgID, UnitType: "usd_credit",
			}
			if err := tx.Create(&quota).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		next := quota.AvailableMinor + amount
		if next < 0 {
			return ErrInsufficientQuota
		}
		quota.AvailableMinor = next
		quota.Version++
		if err := tx.Save(&quota).Error; err != nil {
			return err
		}
		kind := "platform_grant"
		if amount < 0 {
			kind = "quota_reclaim"
		}
		return writeQuotaLedger(tx, quota.ID, kind, amount, "admin", actor, "qgrant:"+channelOrgID+":"+id.New("gen"))
	})
	if err != nil {
		return nil, err
	}
	return s.ChannelQuota(ctx, channelOrgID)
}

func loadIssueRatioBPS(db *gorm.DB, channelOrgID string) int64 {
	if channelOrgID == "" {
		return DefaultIssueRatioBPS
	}
	var row issueRuleRow
	if err := db.Where("channel_org_id = ?", channelOrgID).First(&row).Error; err != nil {
		return DefaultIssueRatioBPS
	}
	if ValidateIssueRatioBPS(row.IssueRatioBPS) != nil {
		return DefaultIssueRatioBPS
	}
	return row.IssueRatioBPS
}

func (s *Service) IssueRule(ctx context.Context, channelOrgID string) (*IssueRuleView, error) {
	if channelOrgID == "" {
		return nil, ErrNotFound
	}
	var row issueRuleRow
	err := s.db.WithContext(ctx).Where("channel_org_id = ?", channelOrgID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &IssueRuleView{ChannelOrgID: channelOrgID, IssueRatioBPS: DefaultIssueRatioBPS}, nil
	}
	if err != nil {
		return nil, err
	}
	return &IssueRuleView{
		ChannelOrgID: row.ChannelOrgID, IssueRatioBPS: row.IssueRatioBPS, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (s *Service) SetIssueRule(ctx context.Context, channelOrgID string, bps int64) (*IssueRuleView, error) {
	if channelOrgID == "" {
		return nil, ErrNotFound
	}
	if err := ValidateIssueRatioBPS(bps); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row issueRuleRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("channel_org_id = ?", channelOrgID).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row = issueRuleRow{
				ID: id.New("qir"), ChannelOrgID: channelOrgID, IssueRatioBPS: bps,
				UpdatedAt: now,
			}
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		row.IssueRatioBPS = bps
		row.Version++
		row.UpdatedAt = now
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return s.IssueRule(ctx, channelOrgID)
}

// issueAllocation 在 B/C 用户充值入账后按渠道换算比发放服务额度，并从渠道 available 扣减发放额。
// 无规则时默认 1:1。官方渠道跳过。同一 source（一笔 topup）只发放一次。
func (s *Service) issueAllocation(tx *gorm.DB, userID, channelOrgID, sourceType, sourceID string, amount int64) error {
	if skipChannelQuota(channelOrgID) || amount <= 0 || userID == "" {
		return nil
	}
	poolID := channelOrgID
	if s.pool != nil {
		if p, err := s.pool.ResolvePoolChannelID(context.Background(), channelOrgID); err == nil && p != "" {
			poolID = p
		}
	}
	var existing allocationRow
	if err := tx.Where("source_type = ? AND source_id = ?", sourceType, sourceID).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	grant, err := ConvertQuota(amount, loadIssueRatioBPS(tx, poolID))
	if err != nil {
		return err
	}
	var quota quotaRow
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", poolID, "usd_credit").
		First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInsufficientQuota
		}
		return err
	}
	if quota.AvailableMinor < grant {
		return ErrInsufficientQuota
	}
	quota.AvailableMinor -= grant
	quota.Version++
	if err := tx.Save(&quota).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	row := allocationRow{
		ID: id.New("qal"), UserID: userID, ChannelOrgID: channelOrgID,
		SourceType: sourceType, SourceID: sourceID, GrantedMinor: grant,
		Status: AllocActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	return writeQuotaLedger(tx, quota.ID, "quota_issue", -grant, sourceType, sourceID, "qissue:"+sourceType+":"+sourceID)
}

func checkAllocationRemaining(tx *gorm.DB, userID, channelOrgID string, amount int64) error {
	if skipChannelQuota(channelOrgID) || amount <= 0 || userID == "" {
		return nil
	}
	var n int64
	if err := tx.Model(&allocationRow{}).Where("user_id = ? AND channel_org_id = ?", userID, channelOrgID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		// 迁移前已入账、还没有发放记录的用户，只靠渠道风险帽拦住。
		return nil
	}
	var remaining int64
	if err := tx.Model(&allocationRow{}).
		Select("COALESCE(SUM(granted_minor - consumed_minor),0)").
		Where("user_id = ? AND channel_org_id = ? AND status = ?", userID, channelOrgID, AllocActive).
		Scan(&remaining).Error; err != nil {
		return err
	}
	if remaining < amount {
		return ErrInsufficientQuota
	}
	return nil
}

func consumeAllocations(tx *gorm.DB, userID, channelOrgID, requestID string, amount int64) error {
	if skipChannelQuota(channelOrgID) || amount <= 0 || userID == "" || requestID == "" {
		return nil
	}
	var prior consumeRow
	if err := tx.Where("request_id = ?", requestID).First(&prior).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var rows []allocationRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND channel_org_id = ? AND status = ?", userID, channelOrgID, AllocActive).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return err
	}
	left := amount
	now := time.Now().UTC()
	for i := range rows {
		if left <= 0 {
			break
		}
		room := rows[i].GrantedMinor - rows[i].ConsumedMinor
		if room <= 0 {
			continue
		}
		take := room
		if take > left {
			take = left
		}
		rows[i].ConsumedMinor += take
		if rows[i].ConsumedMinor >= rows[i].GrantedMinor {
			rows[i].Status = AllocExhausted
		}
		rows[i].UpdatedAt = now
		if err := tx.Save(&rows[i]).Error; err != nil {
			return err
		}
		if err := tx.Create(&consumeRow{
			ID: id.New("qcs"), AllocationID: rows[i].ID, RequestID: requestID,
			AmountMinor: take, IdempotencyKey: "qconsume:" + requestID + ":" + rows[i].ID,
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		left -= take
	}
	return nil
}

func reclaimAllocation(tx *gorm.DB, sourceType, sourceID string) error {
	var alloc allocationRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("source_type = ? AND source_id = ?", sourceType, sourceID).First(&alloc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if alloc.Status == AllocReclaimed {
		return nil
	}
	remaining := alloc.GrantedMinor - alloc.ConsumedMinor
	if remaining > 0 {
		var quota quotaRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", alloc.ChannelOrgID, "usd_credit").
			First(&quota).Error; err != nil {
			return err
		}
		quota.AvailableMinor += remaining
		quota.Version++
		if err := tx.Save(&quota).Error; err != nil {
			return err
		}
		if err := writeQuotaLedger(tx, quota.ID, "quota_reclaim", remaining, sourceType, sourceID, "qreclaim:"+sourceType+":"+sourceID); err != nil {
			return err
		}
	}
	alloc.GrantedMinor = alloc.ConsumedMinor
	alloc.Status = AllocReclaimed
	alloc.UpdatedAt = time.Now().UTC()
	return tx.Save(&alloc).Error
}

func reverseAllocationConsumes(tx *gorm.DB, requestID string) error {
	if requestID == "" {
		return nil
	}
	var consumes []consumeRow
	if err := tx.Where("request_id = ?", requestID).Find(&consumes).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range consumes {
		var alloc allocationRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", consumes[i].AllocationID).First(&alloc).Error; err != nil {
			continue
		}
		alloc.ConsumedMinor -= consumes[i].AmountMinor
		if alloc.ConsumedMinor < 0 {
			alloc.ConsumedMinor = 0
		}
		if alloc.Status == AllocExhausted && alloc.ConsumedMinor < alloc.GrantedMinor {
			alloc.Status = AllocActive
		}
		alloc.UpdatedAt = now
		if err := tx.Save(&alloc).Error; err != nil {
			return err
		}
		if err := tx.Delete(&consumes[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

func allocationRemaining(tx *gorm.DB, userID, channelOrgID string) int64 {
	if skipChannelQuota(channelOrgID) || userID == "" {
		return 0
	}
	var remaining int64
	_ = tx.Model(&allocationRow{}).
		Select("COALESCE(SUM(granted_minor - consumed_minor),0)").
		Where("user_id = ? AND channel_org_id = ? AND status = ?", userID, channelOrgID, AllocActive).
		Scan(&remaining).Error
	return remaining
}

func allocationView(row allocationRow) AllocationView {
	return AllocationView{
		ID: row.ID, UserID: row.UserID, ChannelOrgID: row.ChannelOrgID,
		SourceType: row.SourceType, SourceID: row.SourceID,
		GrantedMinor: row.GrantedMinor, ConsumedMinor: row.ConsumedMinor,
		RemainingMinor: row.GrantedMinor - row.ConsumedMinor, Status: row.Status,
		CreatedAt: row.CreatedAt,
	}
}

func writeQuotaLedger(tx *gorm.DB, accountID, event string, amount int64, refType, refID, idem string) error {
	var existing quotaLedgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	}
	return tx.Create(&quotaLedgerRow{
		ID: id.New("qld"), AccountID: accountID, EventType: event, AmountMinor: amount,
		ReferenceType: refType, ReferenceID: refID, IdempotencyKey: idem,
	}).Error
}
