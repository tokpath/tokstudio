package plans

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type planRow struct {
	ID               string    `gorm:"column:id;primaryKey"`
	OwnerType        string    `gorm:"column:owner_type"`
	OwnerID          string    `gorm:"column:owner_id"`
	Name             string    `gorm:"column:name"`
	Currency         string    `gorm:"column:currency"`
	PriceMinor       int64     `gorm:"column:price_minor"`
	BillingPeriod    string    `gorm:"column:billing_period"`
	Status           string    `gorm:"column:status"`
	PolicyVersion    string    `gorm:"column:policy_version"`
	AutoRenewAllowed bool      `gorm:"column:auto_renew_allowed"`
	ReviewReason     *string   `gorm:"column:review_reason"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (planRow) TableName() string { return "plans_product_plans" }

type itemRow struct {
	ID                string  `gorm:"column:id;primaryKey"`
	PlanID            string  `gorm:"column:plan_id"`
	PublicModelID     *string `gorm:"column:public_model_id"`
	UnitType          string  `gorm:"column:unit_type"`
	IncludedAmount    int64   `gorm:"column:included_amount"`
	OveragePriceMinor int64   `gorm:"column:overage_price_minor"`
	ExpiresInSeconds  int     `gorm:"column:expires_in_seconds"`
}

func (itemRow) TableName() string { return "plans_plan_items" }

type subRow struct {
	ID                 string     `gorm:"column:id;primaryKey"`
	UserID             string     `gorm:"column:user_id"`
	ChannelOrgID       *string    `gorm:"column:channel_org_id"`
	PlanID             string     `gorm:"column:plan_id"`
	Status             string     `gorm:"column:status"`
	CurrentPeriodStart *time.Time `gorm:"column:current_period_start"`
	CurrentPeriodEnd   *time.Time `gorm:"column:current_period_end"`
	RenewalPolicy      string     `gorm:"column:renewal_policy"`
	PaymentAdapter     *string    `gorm:"column:payment_adapter"`
	PaymentMethodRef   *string    `gorm:"column:payment_method_ref"`
	RetryCount         int        `gorm:"column:retry_count"`
	NextRetryAt        *time.Time `gorm:"column:next_retry_at"`
	GraceUntil         *time.Time `gorm:"column:grace_until"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (subRow) TableName() string { return "plans_subscriptions" }

type entRow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	UserID        string     `gorm:"column:user_id"`
	SourceType    string     `gorm:"column:source_type"`
	SourceID      string     `gorm:"column:source_id"`
	UnitType      string     `gorm:"column:unit_type"`
	PublicModelID *string    `gorm:"column:public_model_id"`
	Granted       int64      `gorm:"column:granted"`
	Consumed      int64      `gorm:"column:consumed"`
	ExpiresAt     *time.Time `gorm:"column:expires_at"`
	Status        string     `gorm:"column:status"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

func (entRow) TableName() string { return "plans_entitlement_accounts" }

type ledRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	AccountID      string    `gorm:"column:account_id"`
	EventType      string    `gorm:"column:event_type"`
	Amount         int64     `gorm:"column:amount"`
	RequestID      *string   `gorm:"column:request_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	OccurredAt     time.Time `gorm:"column:occurred_at"`
}

func (ledRow) TableName() string { return "plans_entitlement_ledger" }

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
	return "plans", sub
}

func (s *Service) Seed(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		plan := planRow{
			ID: "pln_echo_month", OwnerType: OwnerPlatform, OwnerID: identity.OfficialChannelID,
			Name: "Echo Monthly", Currency: "USD", PriceMinor: 10 * billingMinor(),
			BillingPeriod: PeriodMonthly, Status: StatusPublished, PolicyVersion: "m5-v1",
			AutoRenewAllowed: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := tx.Where("id = ?", plan.ID).FirstOrCreate(&plan).Error; err != nil {
			return err
		}
		items := []itemRow{
			{ID: "pli_echo_usd", PlanID: plan.ID, UnitType: UnitUSDCredit, IncludedAmount: 5 * billingMinor(), ExpiresInSeconds: 30 * 24 * 3600},
			{ID: "pli_echo_tok", PlanID: plan.ID, UnitType: UnitToken, IncludedAmount: 100000, ExpiresInSeconds: 30 * 24 * 3600, PublicModelID: strPtr(catalog.EchoModelID)},
			{ID: "pli_echo_vid", PlanID: plan.ID, UnitType: UnitVideoSecond, IncludedAmount: 60, ExpiresInSeconds: 30 * 24 * 3600, PublicModelID: strPtr(catalog.SeedanceModelID)},
		}
		for i := range items {
			if err := tx.Where("id = ?", items[i].ID).FirstOrCreate(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func billingMinor() int64 { return 1_000_000 }

func strPtr(s string) *string { return &s }

func (s *Service) CreatePlan(ctx context.Context, in CreatePlanInput) (*PlanView, error) {
	if in.Name == "" || in.PriceMinor <= 0 || len(in.Items) == 0 {
		return nil, ErrInvalidPlan
	}
	if in.OwnerType == "" {
		in.OwnerType = OwnerPlatform
	}
	if in.BillingPeriod == "" {
		in.BillingPeriod = PeriodMonthly
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	status, reason := classifyPlan(in)
	now := time.Now().UTC()
	row := planRow{
		ID: id.New("pln"), OwnerType: in.OwnerType, OwnerID: in.OwnerID, Name: in.Name,
		Currency: in.Currency, PriceMinor: in.PriceMinor, BillingPeriod: in.BillingPeriod,
		Status: status, PolicyVersion: "m5-v1", AutoRenewAllowed: in.AutoRenew,
		CreatedAt: now, UpdatedAt: now,
	}
	if reason != "" {
		row.ReviewReason = &reason
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, item := range in.Items {
			if !validUnit(item.UnitType) || item.Included <= 0 {
				return ErrInvalidPlan
			}
			expires := item.ExpiresIn
			if expires <= 0 {
				expires = 30 * 24 * 3600
			}
			ir := itemRow{
				ID: id.New("pli"), PlanID: row.ID, UnitType: item.UnitType,
				IncludedAmount: item.Included, OveragePriceMinor: item.OverageMinor, ExpiresInSeconds: expires,
			}
			if item.PublicModelID != "" {
				ir.PublicModelID = &item.PublicModelID
			}
			if err := tx.Create(&ir).Error; err != nil {
				return err
			}
		}
		_, err := s.outbox.EnqueueTx(tx, "plans.plan.created", "product_plan", row.ID, map[string]any{
			"status": status, "owner_type": in.OwnerType,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetPlan(ctx, row.ID)
}

func classifyPlan(in CreatePlanInput) (status, reason string) {
	if in.OwnerType == OwnerPlatform || in.OwnerType == "" {
		return StatusPublished, ""
	}
	if in.PriceMinor < PriceFloor {
		return StatusPendingReview, "below_price_floor"
	}
	for _, item := range in.Items {
		if item.Included > 10_000_000 && item.UnitType != UnitToken {
			return StatusPendingReview, "exceeds_entitlement_cap"
		}
		if item.UnitType == UnitVideoSecond && item.Included > 3600 {
			return StatusPendingReview, "high_risk_media_quota"
		}
	}
	return StatusPublished, ""
}

func validUnit(u string) bool {
	switch u {
	case UnitUSDCredit, UnitToken, UnitVideoSecond, UnitImageCount:
		return true
	}
	return false
}

func (s *Service) GetPlan(ctx context.Context, id string) (*PlanView, error) {
	var row planRow
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return s.viewPlan(ctx, row)
}

func (s *Service) ListPlans(ctx context.Context, channelOrgID, status string, publishedOnly bool) ([]PlanView, error) {
	var rows []planRow
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if publishedOnly {
		q = q.Where("status = ?", StatusPublished)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if channelOrgID != "" {
		q = q.Where("(owner_type = ? AND owner_id = ?) OR owner_type = ?", OwnerChannel, channelOrgID, OwnerPlatform)
	} else if publishedOnly {
		// 未登录公共目录只展示平台套餐，避免泄漏其他渠道的销售组合。
		q = q.Where("owner_type = ?", OwnerPlatform)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PlanView, 0, len(rows))
	for _, row := range rows {
		view, err := s.viewPlan(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *Service) ReviewPlan(ctx context.Context, planID, action, reason string) (*PlanView, error) {
	var row planRow
	if err := s.db.WithContext(ctx).Where("id = ?", planID).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	if row.Status != StatusPendingReview {
		return nil, ErrNotPending
	}
	switch action {
	case "approve":
		row.Status = StatusPublished
	case "reject":
		row.Status = StatusRejected
	default:
		return nil, ErrInvalidPlan
	}
	row.UpdatedAt = time.Now().UTC()
	if reason != "" {
		row.ReviewReason = &reason
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return s.viewPlan(ctx, row)
}

func (s *Service) ArchivePlan(ctx context.Context, planID string) (*PlanView, error) {
	res := s.db.WithContext(ctx).Model(&planRow{}).Where("id = ?", planID).Updates(map[string]any{
		"status": StatusArchived, "updated_at": time.Now().UTC(),
	})
	if res.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	return s.GetPlan(ctx, planID)
}

func (s *Service) viewPlan(ctx context.Context, row planRow) (*PlanView, error) {
	var items []itemRow
	if err := s.db.WithContext(ctx).Where("plan_id = ?", row.ID).Find(&items).Error; err != nil {
		return nil, err
	}
	view := &PlanView{
		ID: row.ID, OwnerType: row.OwnerType, OwnerID: row.OwnerID, Name: row.Name,
		PriceMinor: row.PriceMinor, Currency: row.Currency, BillingPeriod: row.BillingPeriod, Status: row.Status,
		AutoRenewAllowed: row.AutoRenewAllowed,
	}
	if row.ReviewReason != nil {
		view.ReviewReason = *row.ReviewReason
	}
	for _, item := range items {
		in := PlanItemInput{UnitType: item.UnitType, Included: item.IncludedAmount, OverageMinor: item.OveragePriceMinor, ExpiresIn: item.ExpiresInSeconds}
		if item.PublicModelID != nil {
			in.PublicModelID = *item.PublicModelID
		}
		view.Items = append(view.Items, in)
	}
	return view, nil
}

func (s *Service) CreateSubscription(ctx context.Context, userID, channelID, planID, adapter, methodRef string) (*SubscriptionView, error) {
	plan, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != StatusPublished {
		return nil, ErrNotPublished
	}
	if plan.OwnerType == OwnerChannel && channelID != "" && plan.OwnerID != channelID {
		return nil, ErrNotFound
	}
	renew := RenewManual
	if adapter == "stripe" {
		renew = RenewAuto
	}
	now := time.Now().UTC()
	row := subRow{
		ID: id.New("sub"), UserID: userID, PlanID: planID, Status: SubPending,
		RenewalPolicy: renew, CreatedAt: now, UpdatedAt: now,
	}
	if channelID != "" {
		row.ChannelOrgID = &channelID
	}
	if adapter != "" {
		row.PaymentAdapter = &adapter
	}
	if methodRef != "" {
		row.PaymentMethodRef = &methodRef
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return subView(row), nil
}

func (s *Service) GetSubscription(ctx context.Context, id, userID string) (*SubscriptionView, error) {
	var row subRow
	q := s.db.WithContext(ctx).Where("id = ?", id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return subView(row), nil
}

func (s *Service) ListSubscriptions(ctx context.Context, userID string) ([]SubscriptionView, error) {
	var rows []subRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SubscriptionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *subView(row))
	}
	return out, nil
}

func (s *Service) ActivatePaid(ctx context.Context, subID string, now time.Time) (*SubscriptionView, error) {
	var view *SubscriptionView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row subRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", subID).First(&row).Error; err != nil {
			return ErrNotFound
		}
		if row.Status == SubActive && row.CurrentPeriodEnd != nil && row.CurrentPeriodEnd.After(now) {
			view = subView(row)
			return nil
		}
		end := now.AddDate(0, 1, 0)
		row.Status = SubActive
		row.CurrentPeriodStart = &now
		row.CurrentPeriodEnd = &end
		row.RetryCount = 0
		row.NextRetryAt = nil
		row.GraceUntil = nil
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := grantPlanEntitlements(tx, row, now); err != nil {
			return err
		}
		view = subView(row)
		return nil
	})
	return view, err
}

func grantPlanEntitlements(tx *gorm.DB, sub subRow, now time.Time) error {
	var items []itemRow
	if err := tx.Where("plan_id = ?", sub.PlanID).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		exp := now.Add(time.Duration(item.ExpiresInSeconds) * time.Second)
		if sub.CurrentPeriodEnd != nil {
			exp = *sub.CurrentPeriodEnd
		}
		idem := grantIdemKey(sub, item.ID)
		var existing ledRow
		if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
			continue
		}
		ent := entRow{
			ID: id.New("ent"), UserID: sub.UserID, SourceType: SourcePlan, SourceID: sub.ID,
			UnitType: item.UnitType, Granted: item.IncludedAmount, Status: EntActive,
			ExpiresAt: &exp, CreatedAt: now, PublicModelID: item.PublicModelID,
		}
		if err := tx.Create(&ent).Error; err != nil {
			return err
		}
		if err := writeEntLedger(tx, ent.ID, EventGrant, item.IncludedAmount, "", idem); err != nil {
			return err
		}
	}
	return nil
}

func grantIdemKey(sub subRow, itemID string) string {
	key := "grant:" + sub.ID + ":" + itemID
	if sub.CurrentPeriodEnd != nil {
		key += ":" + strconv.FormatInt(sub.CurrentPeriodEnd.Unix(), 10)
	}
	return key
}

func (s *Service) Cancel(ctx context.Context, subID, userID string) (*SubscriptionView, error) {
	var row subRow
	q := s.db.WithContext(ctx).Where("id = ?", subID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	if row.Status == SubCancelled {
		return subView(row), nil
	}
	row.Status = SubCancelAtPeriodEnd
	row.RenewalPolicy = RenewManual
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return subView(row), nil
}

// GrantBonus uses a deterministic primary key scoped to the actor and operation.
// INSERT ON CONFLICT serializes concurrent retries in PostgreSQL; the entitlement
// and ledger commit together. A fingerprint rejects changed payloads on replay.
func (s *Service) GrantBonus(ctx context.Context, actorID, key, userID, unitType string, amount int64, expiresIn time.Duration) (*EntitlementView, error) {
	if actorID == "" || key == "" || amount <= 0 || !validUnit(unitType) || expiresIn <= 0 {
		return nil, ErrInvalidPlan
	}
	operation, _ := json.Marshal([]string{actorID, key})
	payload, _ := json.Marshal([]string{userID, unitType, strconv.FormatInt(amount, 10), strconv.FormatInt(int64(expiresIn), 10)})
	entID := fmt.Sprintf("ent_bonus_%x", sha256.Sum256(operation))
	fingerprint := fmt.Sprintf("bonus:%x", sha256.Sum256(append(operation, payload...)))
	now := time.Now().UTC().Truncate(time.Microsecond)
	exp := now.Add(expiresIn)
	ent := entRow{ID: entID, UserID: userID, SourceType: SourceBonus, SourceID: fingerprint,
		UnitType: unitType, Granted: amount, Status: EntActive, ExpiresAt: &exp, CreatedAt: now}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&ent)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where("id = ?", entID).First(&ent).Error; err != nil {
				return err
			}
			if ent.SourceID != fingerprint {
				return ErrBonusConflict
			}
			return nil
		}
		return writeEntLedger(tx, ent.ID, EventBonus, amount, "", "bonus:"+ent.ID)
	}); err != nil {
		return nil, err
	}
	return entView(ent), nil
}

func (s *Service) ListEntitlements(ctx context.Context, userID string) ([]EntitlementView, error) {
	var rows []entRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("expires_at ASC NULLS LAST").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EntitlementView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *entView(row))
	}
	return out, nil
}

// AvailableUSD 是给 billing.Reserve 看的权益覆盖能力，不读 billing 表。
func (s *Service) AvailableUSD(ctx context.Context, userID string) (int64, error) {
	return s.available(ctx, userID, UnitUSDCredit)
}

func (s *Service) available(ctx context.Context, userID, unit string) (int64, error) {
	var rows []entRow
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND unit_type = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", userID, unit, EntActive, now).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	var sum int64
	for _, row := range rows {
		left := row.Granted - row.Consumed
		if left > 0 {
			sum += left
		}
	}
	return sum, nil
}

// ConsumeUSD 按「赠送即将到期 -> 套餐即将到期」扣 usd_credit。
func (s *Service) ConsumeUSD(ctx context.Context, userID, requestID string, amount int64) (int64, error) {
	return s.consume(ctx, userID, requestID, UnitUSDCredit, amount)
}

func (s *Service) consume(ctx context.Context, userID, requestID, unit string, amount int64) (int64, error) {
	if amount <= 0 {
		return 0, nil
	}
	var took int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if requestID != "" {
			net, err := netConsumedTx(tx, requestID)
			if err != nil {
				return err
			}
			if net > 0 {
				took = net
				return nil
			}
		}
		var rows []entRow
		now := time.Now().UTC()
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND unit_type = ? AND status = ? AND granted > consumed AND (expires_at IS NULL OR expires_at > ?)",
				userID, unit, EntActive, now).
			Order("CASE source_type WHEN 'bonus' THEN 0 ELSE 1 END, expires_at ASC NULLS LAST").
			Find(&rows).Error; err != nil {
			return err
		}
		remain := amount
		for i := range rows {
			if remain <= 0 {
				break
			}
			left := rows[i].Granted - rows[i].Consumed
			if left <= 0 {
				continue
			}
			use := left
			if use > remain {
				use = remain
			}
			rows[i].Consumed += use
			if rows[i].Consumed >= rows[i].Granted {
				rows[i].Status = EntExhausted
			}
			if err := tx.Save(&rows[i]).Error; err != nil {
				return err
			}
			if err := writeEntLedger(tx, rows[i].ID, EventDebit, -use, requestID, "debit:"+requestID+":"+rows[i].ID); err != nil {
				return err
			}
			remain -= use
			took += use
		}
		return nil
	})
	return took, err
}

func (s *Service) ReverseByRequest(ctx context.Context, requestID string) error {
	return s.ReverseKeep(ctx, requestID, 0)
}

// ReverseByRequestTx restores consumed entitlements in the charge refund transaction.
func (s *Service) ReverseByRequestTx(tx *gorm.DB, requestID string) error {
	if requestID == "" {
		return nil
	}
	net, err := netConsumedTx(tx, requestID)
	if err != nil {
		return err
	}
	if net <= 0 {
		return nil
	}
	return reverseAmountTx(tx, requestID, net)
}

// ReverseKeep 把该 request 上尚未冲正的权益扣减，恢复到只保留 keep 这么多。
func (s *Service) ReverseKeep(ctx context.Context, requestID string, keep int64) error {
	if requestID == "" {
		return nil
	}
	if keep < 0 {
		keep = 0
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		net, err := netConsumedTx(tx, requestID)
		if err != nil {
			return err
		}
		need := net - keep
		if need <= 0 {
			return nil
		}
		return reverseAmountTx(tx, requestID, need)
	})
}

func netConsumedTx(tx *gorm.DB, requestID string) (int64, error) {
	var leds []ledRow
	if err := tx.Where("request_id = ?", requestID).Find(&leds).Error; err != nil {
		return 0, err
	}
	var net int64
	for _, led := range leds {
		switch led.EventType {
		case EventDebit:
			net += -led.Amount
		case EventReversal:
			net -= led.Amount
		}
	}
	if net < 0 {
		net = 0
	}
	return net, nil
}

func reverseAmountTx(tx *gorm.DB, requestID string, amount int64) error {
	var leds []ledRow
	if err := tx.Where("request_id = ? AND event_type = ?", requestID, EventDebit).
		Order("occurred_at DESC").Find(&leds).Error; err != nil {
		return err
	}
	remain := amount
	for _, led := range leds {
		if remain <= 0 {
			break
		}
		var already int64
		if err := tx.Model(&ledRow{}).
			Where("account_id = ? AND request_id = ? AND event_type = ?", led.AccountID, requestID, EventReversal).
			Select("COALESCE(SUM(amount),0)").Scan(&already).Error; err != nil {
			return err
		}
		left := -led.Amount - already
		if left <= 0 {
			continue
		}
		back := left
		if back > remain {
			back = remain
		}
		var ent entRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", led.AccountID).First(&ent).Error; err != nil {
			return err
		}
		ent.Consumed -= back
		if ent.Consumed < 0 {
			ent.Consumed = 0
		}
		if ent.Status == EntExhausted && ent.Consumed < ent.Granted {
			ent.Status = EntActive
		}
		if err := tx.Save(&ent).Error; err != nil {
			return err
		}
		idem := "reverse:" + led.IdempotencyKey
		if already > 0 {
			idem = "reverse:" + led.IdempotencyKey + ":" + strings.ReplaceAll(led.ID, ":", "")
		}
		if err := writeEntLedger(tx, ent.ID, EventReversal, back, requestID, idem); err != nil {
			return err
		}
		remain -= back
	}
	if remain > 0 {
		return errors.New("entitlement reversal is incomplete")
	}
	return nil
}

func (s *Service) ReverseSource(ctx context.Context, sourceID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.ReverseSourceTx(tx, sourceID) })
}

func (s *Service) ReverseSourceTx(tx *gorm.DB, sourceID string) error {
	var rows []entRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("source_id = ? AND status <> ?", sourceID, EntReversed).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		left := rows[i].Granted - rows[i].Consumed
		rows[i].Status = EntReversed
		if err := tx.Save(&rows[i]).Error; err != nil {
			return err
		}
		if left > 0 {
			if err := writeEntLedger(tx, rows[i].ID, EventReversal, left, "", "revsrc:"+sourceID+":"+rows[i].ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func retryOffsets() []time.Duration {
	return []time.Duration{0, 24 * time.Hour, 3 * 24 * time.Hour, 5 * 24 * time.Hour}
}

// ProcessRenewals 按文档：到期日、+1/+3/+5 天重试；失败 past_due；7 天宽限后 cancelled。
func (s *Service) ProcessRenewals(ctx context.Context, now time.Time, charger func(subID, adapter, methodRef string) error) (int, error) {
	var due []subRow
	// 不能只取前 100 条：共享库过期订阅多时，后创建的支付宝手动续费会被漏掉，一直停在 active。
	if err := s.db.WithContext(ctx).
		Where("status IN ? AND ((current_period_end IS NOT NULL AND current_period_end <= ?) OR (next_retry_at IS NOT NULL AND next_retry_at <= ?))",
			[]string{SubActive, SubPastDue, SubCancelAtPeriodEnd}, now, now).
		Order("id").Find(&due).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range due {
		if err := s.renewOne(ctx, row, now, charger); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) renewOne(ctx context.Context, row subRow, now time.Time, charger func(subID, adapter, methodRef string) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", row.ID).First(&row).Error; err != nil {
			return err
		}
		if row.Status == SubCancelAtPeriodEnd {
			row.Status = SubCancelled
			row.UpdatedAt = now
			return tx.Save(&row).Error
		}
		if row.Status == SubPastDue && row.GraceUntil != nil && !row.GraceUntil.After(now) {
			row.Status = SubCancelled
			row.UpdatedAt = now
			return tx.Save(&row).Error
		}
		if row.RenewalPolicy != RenewAuto {
			if row.Status == SubActive && row.CurrentPeriodEnd != nil && !row.CurrentPeriodEnd.After(now) {
				row.Status = SubPastDue
				grace := now.AddDate(0, 0, GraceDays)
				row.GraceUntil = &grace
				row.UpdatedAt = now
				return tx.Save(&row).Error
			}
			return nil
		}
		adapter := ""
		if row.PaymentAdapter != nil {
			adapter = *row.PaymentAdapter
		}
		method := ""
		if row.PaymentMethodRef != nil {
			method = *row.PaymentMethodRef
		}
		if adapter != "stripe" {
			// 支付宝/微信没有代扣能力时，不得伪造自动续费成功。
			row.Status = SubPastDue
			grace := now.AddDate(0, 0, GraceDays)
			row.GraceUntil = &grace
			row.UpdatedAt = now
			return tx.Save(&row).Error
		}
		if charger == nil {
			return s.markPastDue(tx, &row, now)
		}
		if err := charger(row.ID, adapter, method); err != nil {
			return s.markPastDue(tx, &row, now)
		}
		end := now.AddDate(0, 1, 0)
		row.Status = SubActive
		row.CurrentPeriodStart = &now
		row.CurrentPeriodEnd = &end
		row.RetryCount = 0
		row.NextRetryAt = nil
		row.GraceUntil = nil
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return grantPlanEntitlements(tx, row, now)
	})
}

func (s *Service) markPastDue(tx *gorm.DB, row *subRow, now time.Time) error {
	offsets := retryOffsets()
	row.RetryCount++
	if row.RetryCount < len(offsets) {
		next := now.Add(offsets[row.RetryCount] - offsets[row.RetryCount-1])
		if row.CurrentPeriodEnd != nil {
			next = row.CurrentPeriodEnd.Add(offsets[row.RetryCount])
		}
		row.NextRetryAt = &next
	}
	row.Status = SubPastDue
	if row.GraceUntil == nil {
		base := now
		if row.CurrentPeriodEnd != nil {
			base = *row.CurrentPeriodEnd
		}
		grace := base.AddDate(0, 0, GraceDays)
		row.GraceUntil = &grace
	}
	row.UpdatedAt = now
	return tx.Save(row).Error
}

func (s *Service) RunRenewal(ctx context.Context, charger func(subID, adapter, methodRef string) error) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ProcessRenewals(ctx, time.Now().UTC(), charger)
		}
	}
}

func writeEntLedger(tx *gorm.DB, accountID, event string, amount int64, requestID, idem string) error {
	var existing ledRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	}
	row := ledRow{
		ID: id.New("pel"), AccountID: accountID, EventType: event, Amount: amount,
		IdempotencyKey: idem, OccurredAt: time.Now().UTC(),
	}
	if requestID != "" {
		row.RequestID = &requestID
	}
	return tx.Create(&row).Error
}

func subView(row subRow) *SubscriptionView {
	return &SubscriptionView{
		ID: row.ID, UserID: row.UserID, ChannelOrgID: deref(row.ChannelOrgID), PlanID: row.PlanID, Status: row.Status,
		PeriodStart: row.CurrentPeriodStart, PeriodEnd: row.CurrentPeriodEnd,
		RenewalPolicy: row.RenewalPolicy, Adapter: strings.TrimSpace(deref(row.PaymentAdapter)),
		GraceUntil: row.GraceUntil,
	}
}

func entView(row entRow) *EntitlementView {
	view := &EntitlementView{
		ID: row.ID, SourceType: row.SourceType, SourceID: row.SourceID, UnitType: row.UnitType,
		Granted: row.Granted, Consumed: row.Consumed, Remaining: row.Granted - row.Consumed, Status: row.Status,
		ExpiresAt: row.ExpiresAt,
	}
	if row.PublicModelID != nil {
		view.PublicModelID = *row.PublicModelID
	}
	return view
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Service) ForcePeriodEnd(ctx context.Context, subID string, end time.Time) error {
	res := s.db.WithContext(ctx).Model(&subRow{}).Where("id = ?", subID).Updates(map[string]any{
		"current_period_end": end, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}
