package billing

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

// Service 是账务模块的唯一对外入口。网关只能调用 Reserve/Settle/Release。
type Service struct {
	db           *gorm.DB
	outbox       *outbox.Service
	coverer      EntitlementCoverer
	commissioner Commissioner
	pool         ChannelPool
	qualifier    Qualifier
}

func (s *Service) SetCoverer(c EntitlementCoverer) {
	s.coverer = c
}

func (s *Service) SetCommissioner(c Commissioner) {
	s.commissioner = c
}

func (s *Service) SetPool(p ChannelPool) {
	s.pool = p
}

func (s *Service) SetQualifier(q Qualifier) {
	s.qualifier = q
}

// Credit 是支付模块入账现金钱包的公开入口，不暴露内部表。
func (s *Service) Credit(ctx context.Context, userID, idempotencyKey string, amount int64, memo string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if idempotencyKey == "" {
		return ErrInvalidAmount
	}
	if memo == "" {
		memo = "payment credit"
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize the operation key across wallets as well as concurrent retries.
		// The ledger alone cannot deduplicate a balance update that already happened.
		key := "pay:" + idempotencyKey
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "billing-credit:"+key).Error; err != nil {
			return err
		}
		var existing ledgerRow
		if err := tx.Where("idempotency_key = ?", key).First(&existing).Error; err == nil {
			var wallet walletRow
			if err := tx.Where("id = ?", existing.WalletID).First(&wallet).Error; err != nil {
				return err
			}
			if wallet.UserID != userID || existing.AmountMinor != amount || existing.EventType != EventTopup || existing.ReferenceType != "payment" || existing.ReferenceID != idempotencyKey {
				return ErrConflict
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return creditWallet(tx, userID, amount, EventTopup, "payment", idempotencyKey, "pay:"+idempotencyKey)
	})
	if err != nil {
		return err
	}
	s.considerEligibility(ctx, userID, "", amount)
	return nil
}

func New(db *gorm.DB, publisher *outbox.Service) *Service {
	return &Service{db: db, outbox: publisher}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "billing", sub
}

func (s *Service) Seed(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		codes := []redeemRow{
			{ID: "rc_the2e", Code: RedeemE2E, AmountMinor: 10 * MinorPerUSD, Currency: CurrencyUSD, MaxRedemptions: 100000, Status: "active"},
			{ID: "rc_10", Code: RedeemCredit10, AmountMinor: 10 * MinorPerUSD, Currency: CurrencyUSD, MaxRedemptions: 100000, Status: "active"},
		}
		for i := range codes {
			if err := tx.Where("code = ?", codes[i].Code).FirstOrCreate(&codes[i]).Error; err != nil {
				return err
			}
		}
		quotas := []quotaRow{
			{ID: "qta_oem", OwnerType: "channel", OwnerID: identity.OEMChannelID, UnitType: "usd_credit", AvailableMinor: 1_000_000 * MinorPerUSD},
		}
		for i := range quotas {
			if err := tx.Where("owner_type = ? AND owner_id = ? AND unit_type = ?", quotas[i].OwnerType, quotas[i].OwnerID, quotas[i].UnitType).
				FirstOrCreate(&quotas[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) EnsureWallet(ctx context.Context, userID string) (*walletRow, error) {
	var row walletRow
	err := s.db.WithContext(ctx).Where("user_id = ? AND currency = ?", userID, CurrencyUSD).First(&row).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	row = walletRow{
		ID: id.New("wal"), UserID: userID, Currency: CurrencyUSD,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		_ = s.db.WithContext(ctx).Where("user_id = ? AND currency = ?", userID, CurrencyUSD).First(&row)
		if row.ID != "" {
			return &row, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *Service) Balance(ctx context.Context, userID, channelOrgID string) (*BalanceView, error) {
	wallet, err := s.EnsureWallet(ctx, userID)
	if err != nil {
		return nil, err
	}
	gift := wallet.GiftMinor
	view := &BalanceView{
		UserID: userID, Currency: wallet.Currency,
		AvailableMinor:           wallet.AvailableMinor,
		GiftMinor:                gift,
		PurchasedMinor:           wallet.AvailableMinor - gift,
		CommissionAvailableMinor: wallet.CommissionAvailableMinor,
		ReservedMinor:            wallet.ReservedMinor,
		AvailableUSD:             MinorToUSDString(wallet.AvailableMinor),
		ReservedUSD:              MinorToUSDString(wallet.ReservedMinor),
	}
	if err := s.db.WithContext(ctx).Model(&commissionRecoveryRow{}).Where("wallet_id = ? AND status = ?", wallet.ID, "pending").Select("COALESCE(SUM(amount_minor - recovered_minor),0)").Scan(&view.CommissionRecoveryMinor).Error; err != nil {
		return nil, err
	}
	poolID, err := s.quotaOwner(ctx, channelOrgID)
	if err != nil {
		return nil, err
	}
	if !skipChannelQuota(poolID) {
		var quota quotaRow
		if err := s.db.WithContext(ctx).Where("owner_type = ? AND owner_id = ?", "channel", poolID).First(&quota).Error; err == nil {
			view.ChannelQuota = quota.AvailableMinor
		}
		view.AllocationRemaining = allocationRemaining(s.db.WithContext(ctx), userID, channelOrgID)
	}
	return view, nil
}

func (s *Service) ListLedger(ctx context.Context, userID string, limit int) ([]LedgerView, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	wallet, err := s.EnsureWallet(ctx, userID)
	if err != nil {
		return nil, err
	}
	var rows []ledgerRow
	if err := s.db.WithContext(ctx).Where("wallet_id = ?", wallet.ID).Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]LedgerView, 0, len(rows))
	for _, row := range rows {
		out = append(out, LedgerView{
			ID: row.ID, EventType: row.EventType, AmountMinor: row.AmountMinor,
			BalanceAfter: row.BalanceAfterMinor, ReservedAfter: row.ReservedAfterMinor,
			ReferenceType: row.ReferenceType, ReferenceID: row.ReferenceID,
			IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) ListUsage(ctx context.Context, userID string, limit int) ([]UsageView, error) {
	return s.QueryUsage(ctx, QueryUsageInput{UserID: userID, Limit: limit})
}

func (s *Service) QueryUsage(ctx context.Context, in QueryUsageInput) ([]UsageView, error) {
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	var rows []usageRow
	q := applyUsageFilters(s.db.WithContext(ctx).Model(&usageRow{}), in).Order("occurred_at DESC,id DESC")
	if !in.Unlimited {
		q = q.Limit(in.Limit)
	}
	if in.State != "" {
		q = q.Where("state = ?", in.State)
	}
	q, err := usageCursor(q, in.Cursor)
	if err != nil {
		return nil, err
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return usageViews(rows), nil
}

func usageViews(rows []usageRow) []UsageView {
	out := make([]UsageView, 0, len(rows))
	for _, row := range rows {
		prompt, completion, reasoning := ParseUnitUsage(row.UnitUsage)
		view := UsageView{
			ID: row.ID, RequestID: row.RequestID, UserID: row.UserID, PublicModelID: row.PublicModelID,
			PromptTokens: prompt, CompletionTokens: completion, ReasoningTokens: reasoning,
			UnitUsage: row.UnitUsage, UnitPrices: row.UnitPrices,
			CustomerMinor: row.CustomerAmountMinor, UpstreamMinor: row.UpstreamCostMinor,
			WholesaleMinor: row.WholesaleAmountMinor, State: row.State, OccurredAt: row.OccurredAt,
		}
		if row.AttemptID != nil {
			view.AttemptID = *row.AttemptID
		}
		if row.APIKeyID != nil {
			view.APIKeyID = *row.APIKeyID
		}
		if row.ChannelOrgID != nil {
			view.ChannelOrgID = *row.ChannelOrgID
		}
		if row.ProviderID != nil {
			view.ProviderID = *row.ProviderID
		}
		if row.UpstreamModelID != nil {
			view.UpstreamModelID = *row.UpstreamModelID
		}
		if row.FactSource != nil {
			view.FactSource = *row.FactSource
		}
		if row.PriceVersionID != nil {
			view.PriceVersionID = *row.PriceVersionID
		}
		out = append(out, view)
	}
	return out
}

func (s *Service) Reserve(ctx context.Context, in ReserveInput) (*Reservation, error) {
	if in.RequestID == "" || in.UserID == "" {
		return nil, ErrInvalidAmount
	}
	if in.ReserveMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	var out *Reservation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize the logical operation before checking existence or consuming
		// entitlements, including requests made with a different Key.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "billing.reserve:"+in.RequestID).Error; err != nil {
			return err
		}
		var existing authRow
		findErr := tx.Where("request_id = ?", in.RequestID).First(&existing).Error
		if findErr == nil {
			if existing.UserID != in.UserID || stringPtr(existing.APIKeyID) != in.APIKeyID || existing.PublicModelID != in.PublicModelID || stringPtr(existing.ChannelOrgID) != in.ChannelOrgID || existing.AmountMinor != in.ReserveMinor || stringPtr(existing.PriceVersionID) != in.PriceVersionID || !sameJSON(existing.UnitPrices, in.UnitPrices) || existing.Status != AuthReserved {
				return ErrConflict
			}
			out = &Reservation{Replayed: true, ID: existing.ID, RequestID: existing.RequestID, AmountMinor: existing.AmountMinor, Status: existing.Status, Currency: existing.Currency}
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if err := identity.ReserveAPIKeyBudgetTx(tx, in.APIKeyID, in.UserID, in.PublicModelID, in.ReserveMinor); err != nil {
			return err
		}
		walletNeed := in.ReserveMinor
		wallet, err := lockWallet(tx, in.UserID)
		if err != nil {
			return err
		}
		var covered int64
		if s.coverer != nil {
			avail, err := s.coverer.AvailableUSDTx(tx, in.UserID)
			if err != nil {
				return err
			}
			// Existing cash debt participates in the same account total. Do not
			// clamp it to zero before checking valid entitlement coverage.
			if wallet.AvailableMinor < in.ReserveMinor-avail {
				return ErrInsufficientBalance
			}
			if avail > 0 {
				cover := avail
				if cover > in.ReserveMinor {
					cover = in.ReserveMinor
				}
				took, err := s.coverer.ConsumeUSDTx(tx, in.UserID, in.RequestID, cover)
				if err != nil {
					return err
				}
				covered = took
				if covered > in.ReserveMinor {
					covered = in.ReserveMinor
				}
				walletNeed = in.ReserveMinor - covered
			}
		} else if wallet.AvailableMinor < walletNeed {
			return ErrInsufficientBalance
		}
		if err := s.reserveChannelQuota(tx, in.UserID, in.ChannelOrgID, in.RequestID, in.ReserveMinor, walletNeed); err != nil {
			return err
		}
		now := time.Now().UTC()
		var giftTake int64
		if walletNeed > 0 {
			giftTake = wallet.GiftMinor
			if giftTake > walletNeed {
				giftTake = walletNeed
			}
			// 用原生 SQL 原子扣减，避免 GORM Updates 在并发下漏掉 WHERE 条件。
			exec := tx.Exec(
				`UPDATE billing_wallets
				 SET available_minor = available_minor - ?,
				     reserved_minor = reserved_minor + ?,
				     gift_minor = gift_minor - ?,
				     version = version + 1,
				     updated_at = ?
				 WHERE id = ? AND available_minor >= ? AND gift_minor >= ?`,
				walletNeed, walletNeed, giftTake, now, wallet.ID, walletNeed, giftTake,
			)
			if exec.Error != nil {
				return exec.Error
			}
			if exec.RowsAffected != 1 {
				return ErrInsufficientBalance
			}
			if err := tx.Where("id = ?", wallet.ID).First(wallet).Error; err != nil {
				return err
			}
		}
		auth := authRow{
			PublicModelID: in.PublicModelID, KeyReservedMinor: in.ReserveMinor, ID: id.New("aut"), WalletID: wallet.ID, UserID: in.UserID, RequestID: in.RequestID,
			AmountMinor: in.ReserveMinor, WalletReservedMinor: walletNeed, GiftReservedMinor: giftTake,
			Currency: CurrencyUSD, Status: AuthReserved,
			UnitPrices: in.UnitPrices, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now, UpdatedAt: now,
		}
		if in.APIKeyID != "" {
			auth.APIKeyID = &in.APIKeyID
		}
		if in.ChannelOrgID != "" {
			auth.ChannelOrgID = &in.ChannelOrgID
		}
		if in.PriceVersionID != "" {
			auth.PriceVersionID = &in.PriceVersionID
		}
		if err := tx.Create(&auth).Error; err != nil {
			return err
		}
		if err := writeLedger(tx, wallet, EventAuthorization, -walletNeed, "authorization", auth.ID, "auth:"+in.RequestID); err != nil {
			return err
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.authorization.reserved", "authorization", auth.ID, map[string]any{
			"request_id": in.RequestID, "amount_minor": in.ReserveMinor, "user_id": in.UserID,
		}); err != nil {
			return err
		}
		out = &Reservation{ID: auth.ID, RequestID: in.RequestID, AmountMinor: in.ReserveMinor, Status: AuthReserved, Currency: CurrencyUSD}
		return nil
	})
	if err != nil {
		s.notePreauthFailure(ctx, in, err)
	}
	return out, err
}

type preauthFailRow struct {
	ID        string    `gorm:"column:id;primaryKey"`
	UserID    string    `gorm:"column:user_id"`
	RequestID string    `gorm:"column:request_id"`
	Reason    string    `gorm:"column:reason"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (preauthFailRow) TableName() string { return "billing_preauth_failures" }

func (s *Service) notePreauthFailure(ctx context.Context, in ReserveInput, err error) {
	reason := ""
	switch {
	case errors.Is(err, ErrInsufficientBalance):
		reason = "insufficient_balance"
	case errors.Is(err, ErrInsufficientQuota):
		reason = "insufficient_quota"
	default:
		return
	}
	_ = s.db.WithContext(ctx).Create(&preauthFailRow{
		ID: id.New("paf"), UserID: in.UserID, RequestID: in.RequestID, Reason: reason, CreatedAt: time.Now().UTC(),
	}).Error
}

// AuthorizationQuote is an internal service read of the original admitted
// terms. Media completion must not depend on a currently published catalog.
func (s *Service) AuthorizationQuote(ctx context.Context, requestID string) (Quote, error) {
	var auth authRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&auth).Error; err != nil {
		return Quote{}, err
	}
	return ParseQuote(stringPtr(auth.PriceVersionID), auth.UnitPrices)
}

func (s *Service) Settle(ctx context.Context, in SettleInput) (*Settlement, error) {
	if in.RequestID == "" {
		return nil, ErrInvalidAmount
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = "usage:" + in.RequestID
	}
	var out *Settlement
	var settleUser, settleChannel string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if s.commissioner != nil {
			if err := s.commissioner.LockLifecycleTx(tx); err != nil {
				return err
			}
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "billing.settle:"+in.RequestID).Error; err != nil {
			return err
		}
		var existing chargeRow
		existingErr := tx.Where("request_id = ?", in.RequestID).First(&existing).Error
		if existingErr == nil {
			if !in.MissingUsage && len(in.Usage) > 0 {
				var fact usageRow
				if err := tx.Where("id = ?", existing.UsageEventID).First(&fact).Error; err != nil {
					return err
				}
				actual, _ := json.Marshal(in.Usage)
				if !sameJSON(fact.UnitUsage, actual) {
					return ErrConflict
				}
			}
			out = &Settlement{ChargeID: existing.ID, UsageEventID: existing.UsageEventID, AmountMinor: existing.AmountMinor, State: UsageConfirmed, Currency: CurrencyUSD}
			var prior authRow
			if tx.Where("request_id = ?", in.RequestID).First(&prior).Error == nil {
				settleUser = prior.UserID
				settleChannel = firstNonEmpty(stringPtr(prior.ChannelOrgID), in.ChannelOrgID)
				keep := entitlementKeep(prior, existing.AmountMinor)
				if s.coverer != nil {
					if err := s.coverer.ReverseKeepTx(tx, in.RequestID, keep); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		var auth authRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", in.RequestID).First(&auth).Error; err != nil {
			return err
		}
		if auth.Status == AuthSettled {
			var charge chargeRow
			if err := tx.Where("request_id = ?", in.RequestID).First(&charge).Error; err != nil {
				return err
			}
			out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: UsageConfirmed, Currency: CurrencyUSD}
			settleUser = auth.UserID
			settleChannel = firstNonEmpty(stringPtr(auth.ChannelOrgID), in.ChannelOrgID)
			return nil
		}
		if auth.Status != AuthReserved && auth.Status != AuthPendingReconciliation {
			return ErrAuthNotReserved
		}
		in.UserID = auth.UserID
		if auth.ChannelOrgID != nil {
			in.ChannelOrgID = *auth.ChannelOrgID
		}
		if auth.APIKeyID != nil {
			in.APIKeyID = *auth.APIKeyID
		}
		if auth.PublicModelID != "" {
			in.PublicModelID = auth.PublicModelID
		}
		settleUser = auth.UserID
		settleChannel = firstNonEmpty(stringPtr(auth.ChannelOrgID), in.ChannelOrgID)
		if in.MissingUsage {
			var measured usageRow
			measuredErr := tx.Where("idempotency_key = ?", "usage:"+in.RequestID+":over-reserve").First(&measured).Error
			if measuredErr == nil {
				out = &Settlement{UsageEventID: measured.ID, AmountMinor: measured.CustomerAmountMinor, State: UsagePending, Currency: CurrencyUSD}
				return nil
			}
			if !errors.Is(measuredErr, gorm.ErrRecordNotFound) {
				return measuredErr
			}
			now := time.Now().UTC()
			auth.Status = AuthPendingReconciliation
			auth.UpdatedAt = now
			if err := tx.Save(&auth).Error; err != nil {
				return err
			}
			usageJSON, _ := json.Marshal(map[string]any{"missing": true})
			prices := auth.UnitPrices
			row := usageRow{
				ID: id.New("usg"), RequestID: in.RequestID, UserID: auth.UserID,
				PublicModelID: in.PublicModelID, UnitUsage: usageJSON, UnitPrices: prices,
				Currency: CurrencyUSD, State: UsagePending, IdempotencyKey: "usage:" + in.RequestID + ":pending",
				OccurredAt: now,
			}
			copySettleRefs(&row, in)
			inserted := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&row)
			if inserted.Error != nil {
				return inserted.Error
			}
			var saved usageRow
			if err := tx.Where("idempotency_key = ?", row.IdempotencyKey).First(&saved).Error; err != nil {
				return err
			}
			row = saved
			if inserted.RowsAffected > 0 {
				if _, err := s.outbox.EnqueueTx(tx, "usage.reconciliation.requested", "usage_event", row.ID, map[string]any{
					"request_id": in.RequestID,
				}); err != nil {
					return err
				}
			}
			out = &Settlement{UsageEventID: row.ID, State: UsagePending, Currency: CurrencyUSD}
			return nil
		}
		prices, err := authorizationSettlementPrices(auth.UnitPrices, in.UnitPrices)
		if err != nil {
			return err
		}
		quote, err := ParseQuote(stringPtr(auth.PriceVersionID), prices)
		if err != nil {
			return err
		}
		prompt := in.Usage["prompt_tokens"]
		completion := in.Usage["completion_tokens"]
		customer := quote.Charge(in.Usage, in.Resolution)
		if customer == 0 {
			customer = quote.CustomerMinor(prompt, completion)
		}
		var measured usageRow
		measuredErr := tx.Where("idempotency_key = ?", "usage:"+in.RequestID+":over-reserve").First(&measured).Error
		if measuredErr == nil {
			usageJSON, _ := json.Marshal(in.Usage)
			if measured.CustomerAmountMinor != customer || !sameJSON(measured.UnitUsage, usageJSON) {
				return ErrConflict
			}
			// Previously deferred measured excess can now settle, preserving its
			// first actual fact. A conflicting later report remains a conflict.
			prices = measured.UnitPrices
			quote, err = ParseQuote(stringPtr(auth.PriceVersionID), prices)
			if err != nil {
				return err
			}
		}
		if measuredErr != nil && !errors.Is(measuredErr, gorm.ErrRecordNotFound) {
			return measuredErr
		}
		walletReserved := auth.WalletReservedMinor
		if walletReserved > auth.AmountMinor {
			walletReserved = auth.AmountMinor
		}
		covered := auth.AmountMinor - walletReserved
		if covered < 0 {
			covered = 0
		}
		wallet, err := lockWalletByID(tx, auth.WalletID)
		if err != nil {
			return err
		}
		planLeft := int64(0)
		wipePlan := false
		if s.coverer != nil {
			planLeft, err = s.coverer.PurchasedUSDRemainingTx(tx, auth.UserID)
			if err != nil {
				return err
			}
			// 实际费用超过购买套餐的剩余额度时，套餐归零，账户现金不扣。
			if planLeft > 0 && customer > covered+planLeft {
				wipePlan = true
				zeroed, zeroErr := s.coverer.ZeroPurchasedUSDTx(tx, auth.UserID, in.RequestID)
				if zeroErr != nil {
					return zeroErr
				}
				covered += zeroed
			} else if customer > covered {
				covered, err = s.coverer.ExtendUSDUsageTx(tx, auth.UserID, in.RequestID, customer)
				if err != nil {
					return err
				}
			}
		}
		entitlementUsed := covered
		if !wipePlan && entitlementUsed > customer {
			entitlementUsed = customer
		}
		walletAvailable := wallet.AvailableMinor
		if walletAvailable < 0 {
			walletAvailable = 0
		}
		walletUsed := int64(0)
		if !wipePlan {
			walletUsed = customer - entitlementUsed
			if walletUsed < 0 {
				walletUsed = 0
			}
			// 现金最多扣到零，不能把可用余额扣成负数。
			maxWallet := walletReserved + walletAvailable
			if walletUsed > maxWallet {
				walletUsed = maxWallet
			}
		}
		billed := entitlementUsed + walletUsed
		if err := identity.AdjustAPIKeyBudgetTx(tx, stringPtr(auth.APIKeyID), billed, -auth.KeyReservedMinor); err != nil {
			return err
		}
		walletRelease := walletReserved - walletUsed
		giftReserved := auth.GiftReservedMinor
		if giftReserved > walletReserved {
			giftReserved = walletReserved
		}
		giftUsed := walletUsed
		if giftUsed > giftReserved+wallet.GiftMinor {
			giftUsed = giftReserved + wallet.GiftMinor
		}
		giftRestore := giftReserved - giftUsed
		now := time.Now().UTC()
		wallet.ReservedMinor -= walletReserved
		wallet.AvailableMinor += walletRelease
		wallet.GiftMinor += giftRestore
		wallet.Version++
		wallet.UpdatedAt = now
		if err := tx.Save(wallet).Error; err != nil {
			return err
		}
		if walletUsed > 0 {
			if err := writeLedger(tx, wallet, EventUsageDebit, -walletUsed, "request", in.RequestID, in.IdempotencyKey); err != nil {
				return err
			}
		}
		if walletRelease > 0 {
			if err := writeLedger(tx, wallet, EventRelease, walletRelease, "authorization", auth.ID, "release:"+in.RequestID); err != nil {
				return err
			}
		}
		if err := s.settleChannelQuota(tx, auth.UserID, firstNonEmpty(stringPtr(auth.ChannelOrgID), in.ChannelOrgID), in.RequestID, walletUsed); err != nil {
			return err
		}
		usageJSON, _ := json.Marshal(in.Usage)
		row := usageRow{
			ID: id.New("usg"), RequestID: in.RequestID, UserID: auth.UserID,
			PublicModelID: in.PublicModelID, UnitUsage: usageJSON, UnitPrices: quote.Raw,
			PriceVersionID: auth.PriceVersionID, CustomerAmountMinor: billed,
			UpstreamCostMinor:    quote.MediaCost(in.Usage, in.Resolution),
			WholesaleAmountMinor: quote.WholesaleCharge(in.Usage, in.Resolution),
			Currency:             CurrencyUSD, State: UsageConfirmed, IdempotencyKey: in.IdempotencyKey,
			OccurredAt: now,
		}
		if quote.VersionID != "" {
			row.PriceVersionID = &quote.VersionID
		}
		copySettleRefs(&row, in)
		if err := tx.Create(&row).Error; err != nil {
			var dup usageRow
			if findErr := tx.Where("idempotency_key = ?", in.IdempotencyKey).First(&dup).Error; findErr == nil {
				out = &Settlement{UsageEventID: dup.ID, AmountMinor: dup.CustomerAmountMinor, State: dup.State, Currency: CurrencyUSD}
				return nil
			}
			return err
		}
		charge := chargeRow{
			ID: id.New("chg"), RequestID: in.RequestID, UsageEventID: row.ID,
			AuthorizationID: &auth.ID, AmountMinor: billed, PriceVersionID: row.PriceVersionID,
			Status: ChargeCommitted, CreatedAt: now,
		}
		if err := tx.Create(&charge).Error; err != nil {
			return err
		}
		// Preserve prior unknown/legacy measured facts as history. The reliable
		// confirmed charge removes this request from the pending work queue.
		if err := tx.Model(&usageRow{}).Where("request_id = ? AND state = ?", in.RequestID, UsagePending).Update("state", UsageVoided).Error; err != nil {
			return err
		}
		if in.AttemptID != "" && in.ProviderID != "" {
			if err := tx.Where("attempt_id = ?", in.AttemptID).FirstOrCreate(&costRow{
				ID: id.New("cst"), RequestID: in.RequestID, AttemptID: in.AttemptID, ProviderID: in.ProviderID,
				AmountMinor: quote.MediaCost(in.Usage, in.Resolution), Currency: CurrencyUSD,
				UnitUsage: usageJSON, UnitPrices: quote.Raw, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		if err := s.accrueCommission(tx, row); err != nil {
			return err
		}
		auth.Status = AuthSettled
		auth.SettledMinor = billed
		auth.GiftSettledMinor = giftUsed
		auth.EntitlementSettledMinor = entitlementUsed
		auth.UpdatedAt = now
		if err := tx.Save(&auth).Error; err != nil {
			return err
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.charge.committed", "customer_charge", charge.ID, map[string]any{
			"request_id": in.RequestID, "amount_minor": billed, "usage_event_id": row.ID,
		}); err != nil {
			return err
		}
		out = &Settlement{ChargeID: charge.ID, UsageEventID: row.ID, AmountMinor: billed, State: UsageConfirmed, Currency: CurrencyUSD}
		if s.coverer != nil {
			if err := s.coverer.ReverseKeepTx(tx, in.RequestID, entitlementUsed); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && out != nil && out.State == UsageConfirmed {
		s.considerEligibility(ctx, firstNonEmpty(settleUser, in.UserID), settleChannel, 0)
	}
	return out, err
}

func entitlementKeep(auth authRow, customer int64) int64 {
	if auth.Status == AuthSettled || auth.Status == AuthReversed {
		return auth.EntitlementSettledMinor
	}
	walletReserved := auth.WalletReservedMinor
	if walletReserved > auth.AmountMinor {
		walletReserved = auth.AmountMinor
	}
	covered := auth.AmountMinor - walletReserved
	if covered < 0 {
		covered = 0
	}
	if customer > covered {
		return covered
	}
	if customer < 0 {
		return 0
	}
	return customer
}

func (s *Service) Release(ctx context.Context, requestID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var auth authRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestID).First(&auth).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		return s.releaseAuthTx(ctx, tx, &auth)
	})
}

func (s *Service) releaseAuthTx(ctx context.Context, tx *gorm.DB, auth *authRow) error {
	if auth.Status == AuthReleased || auth.Status == AuthSettled || auth.Status == AuthReversed {
		return nil
	}
	if err := identity.AdjustAPIKeyBudgetTx(tx, stringPtr(auth.APIKeyID), 0, -auth.KeyReservedMinor); err != nil {
		return err
	}
	wallet, err := lockWalletByID(tx, auth.WalletID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	walletPart := auth.WalletReservedMinor
	if walletPart > auth.AmountMinor {
		walletPart = auth.AmountMinor
	}
	giftPart := auth.GiftReservedMinor
	if giftPart > walletPart {
		giftPart = walletPart
	}
	wallet.ReservedMinor -= walletPart
	wallet.AvailableMinor += walletPart
	wallet.GiftMinor += giftPart
	wallet.Version++
	wallet.UpdatedAt = now
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	if walletPart > 0 {
		if err := writeLedger(tx, wallet, EventRelease, walletPart, "authorization", auth.ID, "release:"+auth.RequestID); err != nil {
			return err
		}
	}
	if auth.ChannelOrgID != nil {
		_ = s.releaseChannelQuota(tx, *auth.ChannelOrgID, auth.RequestID, auth.AmountMinor)
	}
	auth.Status = AuthReleased
	auth.UpdatedAt = now
	if err := tx.Save(auth).Error; err != nil {
		return err
	}
	if s.coverer != nil {
		if err := s.coverer.ReverseByRequestTx(tx, auth.RequestID); err != nil {
			return err
		}
	}
	return nil
}

// Expiry alone does not prove a request never reached its upstream. Preserve
// occupancy and make the unknown request visible in existing reconciliation.
func (s *Service) ReapExpired(ctx context.Context) (int, error) {
	var rows []authRow
	if err := s.db.WithContext(ctx).Where("status = ? AND expires_at < ?", AuthReserved, time.Now().UTC()).Limit(100).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if _, err := s.Settle(ctx, SettleInput{RequestID: row.RequestID, UserID: row.UserID, APIKeyID: stringPtr(row.APIKeyID), ChannelOrgID: stringPtr(row.ChannelOrgID), PublicModelID: row.PublicModelID, MissingUsage: true}); err == nil {
			n++
		}
	}
	return n, nil
}
func sameJSON(a, b []byte) bool {
	var x, y any
	if len(a) == 0 {
		a = []byte(`{}`)
	}
	if len(b) == 0 {
		b = []byte(`{}`)
	}
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func (s *Service) RunReaper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ReapExpired(ctx)
		}
	}
}

func lockWallet(tx *gorm.DB, userID string) (*walletRow, error) {
	var row walletRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND currency = ?", userID, CurrencyUSD).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = walletRow{ID: id.New("wal"), UserID: userID, Currency: CurrencyUSD, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := tx.Create(&row).Error; err != nil {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND currency = ?", userID, CurrencyUSD).First(&row).Error; err != nil {
				return nil, err
			}
			return &row, nil
		}
		return &row, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func lockWalletByID(tx *gorm.DB, walletID string) (*walletRow, error) {
	var row walletRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", walletID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func writeLedger(tx *gorm.DB, wallet *walletRow, event string, amount int64, refType, refID, idem string) error {
	var existing ledgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	}
	return tx.Create(&ledgerRow{
		ID: id.New("led"), WalletID: wallet.ID, EventType: event, AmountMinor: amount,
		Currency: wallet.Currency, BalanceAfterMinor: wallet.AvailableMinor, ReservedAfterMinor: wallet.ReservedMinor,
		ReferenceType: refType, ReferenceID: refID, IdempotencyKey: idem, CreatedAt: time.Now().UTC(),
	}).Error
}

func copySettleRefs(row *usageRow, in SettleInput) {
	if in.AttemptID != "" {
		row.AttemptID = &in.AttemptID
	}
	if in.APIKeyID != "" {
		row.APIKeyID = &in.APIKeyID
	}
	if in.ChannelOrgID != "" {
		row.ChannelOrgID = &in.ChannelOrgID
	}
	if in.ProviderID != "" {
		row.ProviderID = &in.ProviderID
	}
	if in.UpstreamModelID != "" {
		row.UpstreamModelID = &in.UpstreamModelID
	}
	if in.FactSource != "" {
		src := in.FactSource
		row.FactSource = &src
	}
	if in.UserID != "" {
		row.UserID = in.UserID
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func stringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func firstBytes(primary, fallback []byte) []byte {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func (s *Service) considerEligibility(ctx context.Context, userID, channelID string, topup int64) {
	if s.qualifier == nil || userID == "" {
		return
	}
	var spend int64
	_ = s.db.WithContext(ctx).Model(&usageRow{}).
		Where("user_id = ? AND state = ?", userID, UsageConfirmed).
		Select("COALESCE(SUM(customer_amount_minor),0)").Scan(&spend).Error
	_ = s.qualifier.Consider(ctx, userID, channelID, topup, spend)
}

// The original brand selling/wholesale terms are immutable. Only actual
// internal supplier costs can be attached from the successful attempt snapshot.
func authorizationSettlementPrices(original, actual json.RawMessage) (json.RawMessage, error) {
	var prices, costs map[string]json.RawMessage
	if err := json.Unmarshal(original, &prices); err != nil {
		return nil, err
	}
	if len(actual) > 0 {
		if err := json.Unmarshal(actual, &costs); err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"upstream_cost_input", "upstream_cost_output", "upstream_cost_reasoning", "upstream_cost", "image_count_cost", "video_second_cost", "audio_second_cost"} {
		if value, ok := costs[key]; ok {
			prices[key] = value
		}
	}
	return json.Marshal(prices)
}
