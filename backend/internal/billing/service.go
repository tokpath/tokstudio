package billing

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
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
			{ID: "qta_reseller", OwnerType: "channel", OwnerID: identity.ResellerChannelID, UnitType: "usd_credit", AvailableMinor: 1_000_000 * MinorPerUSD},
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
	view := &BalanceView{
		UserID: userID, Currency: wallet.Currency,
		AvailableMinor: wallet.AvailableMinor, ReservedMinor: wallet.ReservedMinor,
		AvailableUSD: MinorToUSDString(wallet.AvailableMinor),
		ReservedUSD:  MinorToUSDString(wallet.ReservedMinor),
	}
	if channelOrgID != "" && channelOrgID != identity.OfficialChannelID {
		var quota quotaRow
		if err := s.db.WithContext(ctx).Where("owner_type = ? AND owner_id = ?", "channel", channelOrgID).First(&quota).Error; err == nil {
			view.ChannelQuota = quota.AvailableMinor
		}
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
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []usageRow
	q := s.db.WithContext(ctx).Order("occurred_at DESC").Limit(limit)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return usageViews(rows), nil
}

func usageViews(rows []usageRow) []UsageView {
	out := make([]UsageView, 0, len(rows))
	for _, row := range rows {
		view := UsageView{
			ID: row.ID, RequestID: row.RequestID, PublicModelID: row.PublicModelID,
			UnitUsage: row.UnitUsage, UnitPrices: row.UnitPrices,
			CustomerMinor: row.CustomerAmountMinor, UpstreamMinor: row.UpstreamCostMinor,
			WholesaleMinor: row.WholesaleAmountMinor, State: row.State, OccurredAt: row.OccurredAt,
		}
		if row.AttemptID != nil {
			view.AttemptID = *row.AttemptID
		}
		if row.ProviderID != nil {
			view.ProviderID = *row.ProviderID
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
		var existing authRow
		if err := tx.Where("request_id = ?", in.RequestID).First(&existing).Error; err == nil {
			out = &Reservation{ID: existing.ID, RequestID: existing.RequestID, AmountMinor: existing.AmountMinor, Status: existing.Status, Currency: existing.Currency}
			return nil
		}
		wallet, err := lockWallet(tx, in.UserID)
		if err != nil {
			return err
		}
		if wallet.AvailableMinor < in.ReserveMinor {
			return ErrInsufficientBalance
		}
		if err := s.reserveChannelQuota(tx, in.ChannelOrgID, in.RequestID, in.ReserveMinor); err != nil {
			return err
		}
		now := time.Now().UTC()
		wallet.AvailableMinor -= in.ReserveMinor
		wallet.ReservedMinor += in.ReserveMinor
		wallet.Version++
		wallet.UpdatedAt = now
		if err := tx.Save(wallet).Error; err != nil {
			return err
		}
		auth := authRow{
			ID: id.New("aut"), WalletID: wallet.ID, UserID: in.UserID, RequestID: in.RequestID,
			AmountMinor: in.ReserveMinor, Currency: CurrencyUSD, Status: AuthReserved,
			UnitPrices: in.UnitPrices, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now, UpdatedAt: now,
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
		if err := writeLedger(tx, wallet, EventAuthorization, -in.ReserveMinor, "authorization", auth.ID, "auth:"+in.RequestID); err != nil {
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
	return out, err
}

func (s *Service) Settle(ctx context.Context, in SettleInput) (*Settlement, error) {
	if in.RequestID == "" {
		return nil, ErrInvalidAmount
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = "usage:" + in.RequestID
	}
	var out *Settlement
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing chargeRow
		if err := tx.Where("request_id = ?", in.RequestID).First(&existing).Error; err == nil {
			out = &Settlement{ChargeID: existing.ID, UsageEventID: existing.UsageEventID, AmountMinor: existing.AmountMinor, State: UsageConfirmed, Currency: CurrencyUSD}
			return nil
		}
		var auth authRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", in.RequestID).First(&auth).Error; err != nil {
			return err
		}
		if auth.Status == AuthSettled {
			var charge chargeRow
			_ = tx.Where("request_id = ?", in.RequestID).First(&charge)
			out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: UsageConfirmed, Currency: CurrencyUSD}
			return nil
		}
		if auth.Status != AuthReserved && auth.Status != AuthPendingReconciliation {
			return ErrAuthNotReserved
		}
		if in.MissingUsage {
			now := time.Now().UTC()
			auth.Status = AuthPendingReconciliation
			auth.UpdatedAt = now
			if err := tx.Save(&auth).Error; err != nil {
				return err
			}
			usageJSON, _ := json.Marshal(map[string]any{"missing": true})
			prices := auth.UnitPrices
			if len(in.UnitPrices) > 0 {
				prices = in.UnitPrices
			}
			row := usageRow{
				ID: id.New("usg"), RequestID: in.RequestID, UserID: auth.UserID,
				PublicModelID: in.PublicModelID, UnitUsage: usageJSON, UnitPrices: prices,
				Currency: CurrencyUSD, State: UsagePending, IdempotencyKey: in.IdempotencyKey + ":pending",
				OccurredAt: now,
			}
			copySettleRefs(&row, in)
			if err := tx.Where("idempotency_key = ?", row.IdempotencyKey).FirstOrCreate(&row).Error; err != nil {
				return err
			}
			if _, err := s.outbox.EnqueueTx(tx, "usage.reconciliation.requested", "usage_event", row.ID, map[string]any{
				"request_id": in.RequestID,
			}); err != nil {
				return err
			}
			out = &Settlement{UsageEventID: row.ID, State: UsagePending, Currency: CurrencyUSD}
			return nil
		}
		quote, err := ParseQuote(firstNonEmpty(in.PriceVersionID, stringPtr(auth.PriceVersionID)), firstBytes(in.UnitPrices, auth.UnitPrices))
		if err != nil {
			return err
		}
		prompt := in.Usage["prompt_tokens"]
		completion := in.Usage["completion_tokens"]
		customer := quote.CustomerMinor(prompt, completion)
		if customer > auth.AmountMinor {
			customer = auth.AmountMinor
		}
		wallet, err := lockWalletByID(tx, auth.WalletID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		release := auth.AmountMinor - customer
		wallet.ReservedMinor -= auth.AmountMinor
		wallet.AvailableMinor += release
		wallet.Version++
		wallet.UpdatedAt = now
		if err := tx.Save(wallet).Error; err != nil {
			return err
		}
		if err := writeLedger(tx, wallet, EventUsageDebit, -customer, "request", in.RequestID, in.IdempotencyKey); err != nil {
			return err
		}
		if release > 0 {
			if err := writeLedger(tx, wallet, EventRelease, release, "authorization", auth.ID, "release:"+in.RequestID); err != nil {
				return err
			}
		}
		if err := s.settleChannelQuota(tx, in.ChannelOrgID, in.RequestID, auth.AmountMinor, customer); err != nil {
			return err
		}
		usageJSON, _ := json.Marshal(in.Usage)
		row := usageRow{
			ID: id.New("usg"), RequestID: in.RequestID, UserID: auth.UserID,
			PublicModelID: in.PublicModelID, UnitUsage: usageJSON, UnitPrices: quote.Raw,
			PriceVersionID: auth.PriceVersionID, CustomerAmountMinor: customer,
			UpstreamCostMinor:    quote.CostMinor(prompt, completion),
			WholesaleAmountMinor: quote.WholesaleMinor(prompt, completion),
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
			AuthorizationID: &auth.ID, AmountMinor: customer, PriceVersionID: row.PriceVersionID,
			Status: ChargeCommitted, CreatedAt: now,
		}
		if err := tx.Create(&charge).Error; err != nil {
			return err
		}
		if in.AttemptID != "" && in.ProviderID != "" {
			_ = tx.Where("attempt_id = ?", in.AttemptID).FirstOrCreate(&costRow{
				ID: id.New("cst"), RequestID: in.RequestID, AttemptID: in.AttemptID, ProviderID: in.ProviderID,
				AmountMinor: quote.CostMinor(prompt, completion), Currency: CurrencyUSD,
				UnitUsage: usageJSON, UnitPrices: quote.Raw, CreatedAt: now,
			}).Error
		}
		if err := s.accrueCommission(tx, row); err != nil {
			return err
		}
		auth.Status = AuthSettled
		auth.SettledMinor = customer
		auth.UpdatedAt = now
		if err := tx.Save(&auth).Error; err != nil {
			return err
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.charge.committed", "customer_charge", charge.ID, map[string]any{
			"request_id": in.RequestID, "amount_minor": customer, "usage_event_id": row.ID,
		}); err != nil {
			return err
		}
		out = &Settlement{ChargeID: charge.ID, UsageEventID: row.ID, AmountMinor: customer, State: UsageConfirmed, Currency: CurrencyUSD}
		return nil
	})
	return out, err
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
		if auth.Status == AuthReleased || auth.Status == AuthSettled || auth.Status == AuthReversed {
			return nil
		}
		wallet, err := lockWalletByID(tx, auth.WalletID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		wallet.ReservedMinor -= auth.AmountMinor
		wallet.AvailableMinor += auth.AmountMinor
		wallet.Version++
		wallet.UpdatedAt = now
		if err := tx.Save(wallet).Error; err != nil {
			return err
		}
		if err := writeLedger(tx, wallet, EventRelease, auth.AmountMinor, "authorization", auth.ID, "release:"+requestID); err != nil {
			return err
		}
		if auth.ChannelOrgID != nil {
			_ = s.releaseChannelQuota(tx, *auth.ChannelOrgID, requestID, auth.AmountMinor)
		}
		auth.Status = AuthReleased
		auth.UpdatedAt = now
		return tx.Save(&auth).Error
	})
}

func (s *Service) ReapExpired(ctx context.Context) (int, error) {
	var rows []authRow
	if err := s.db.WithContext(ctx).Where("status = ? AND expires_at < ?", AuthReserved, time.Now().UTC()).Limit(100).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if err := s.Release(ctx, row.RequestID); err == nil {
			n++
		}
	}
	return n, nil
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
