package payments

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type AssetNetwork struct {
	ID                   string    `json:"id"`
	VersionID            string    `json:"version_id" gorm:"primaryKey"`
	Version              int64     `json:"version"`
	Driver               string    `json:"driver"`
	Asset                string    `json:"asset"`
	Network              string    `json:"network"`
	TokenContract        string    `json:"token_contract"`
	TokenDecimals        int       `json:"token_decimals"`
	MinimumConfirmations int64     `json:"minimum_confirmations"`
	Enabled              bool      `json:"enabled"`
	EffectiveAt          time.Time `json:"effective_at"`
	EvidenceReference    string    `json:"evidence_reference"`
	PublishedBy          string    `json:"published_by"`
	PublishedAt          time.Time `json:"published_at"`
}

func (AssetNetwork) TableName() string { return "payments_asset_network_versions" }

type SupportedAsset struct {
	ID                   string    `json:"id"`
	VersionID            string    `json:"version_id"`
	Version              int64     `json:"version"`
	Driver               string    `json:"driver"`
	Asset                string    `json:"asset"`
	Network              string    `json:"network"`
	TokenContract        string    `json:"token_contract"`
	TokenDecimals        int       `json:"token_decimals"`
	MinimumConfirmations int64     `json:"minimum_confirmations"`
	EffectiveAt          time.Time `json:"effective_at"`
}

func (s *Service) requirePlatformAssetManager(ctx context.Context, tx *gorm.DB, actor, platform string) error {
	if s.Authorize == nil {
		return ErrNotConfigured
	}
	h, ok := s.Authorize.(interface {
		GetOperator(context.Context, *gorm.DB, string) (organization.Operator, error)
	})
	if !ok {
		return ErrNotConfigured
	}
	op, e := h.GetOperator(ctx, tx, platform)
	if e != nil {
		return e
	}
	if op.Kind != organization.Platform {
		return ErrForbidden
	}
	return s.Authorize.RequirePermission(ctx, tx, actor, platform, "providers.manage")
}
func normalizeAssetNetwork(a AssetNetwork) (AssetNetwork, error) {
	a.Asset = strings.ToUpper(strings.TrimSpace(a.Asset))
	a.Network = strings.ToLower(strings.TrimSpace(a.Network))
	a.TokenContract = strings.TrimSpace(a.TokenContract)
	if a.Driver != "evm_wallet" && a.Driver != "nowpayments" || a.EvidenceReference == "" || len(a.EvidenceReference) > 2048 || a.Network == "" || len(a.Network) > 64 || a.TokenContract == "" || len(a.TokenContract) > 256 || a.TokenDecimals < 0 || a.TokenDecimals > 18 || a.MinimumConfirmations < 0 || a.MinimumConfirmations > 100000 || (Money{Currency: a.Asset, Amount: ledger.FromInt64(1)}).Validate() != nil {
		return a, ErrInvalid
	}
	if a.Driver == "evm_wallet" {
		n, ok := new(big.Int).SetString(a.Network, 10)
		if !ok || n.Sign() <= 0 || !validHex(a.TokenContract, 20) || a.MinimumConfirmations < 1 {
			return a, ErrInvalid
		}
		a.Network = n.String()
		a.TokenContract = strings.ToLower(a.TokenContract)
	}
	return a, nil
}
func (s *Service) PublishAssetNetwork(ctx context.Context, actor, platform string, a AssetNetwork, expected int64) (AssetNetwork, error) {
	if expected < 0 {
		return a, ErrInvalid
	}
	var e error
	a, e = normalizeAssetNetwork(a)
	if e != nil {
		return a, e
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if _, e = uuid.Parse(a.ID); e != nil {
		return a, ErrInvalid
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.requirePlatformAssetManager(ctx, tx, actor, platform); e != nil {
			return e
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "payment-asset:"+a.ID).Error; e != nil {
			return e
		}
		var old AssetNetwork
		q := tx.Where("id=?", a.ID).Order("version DESC").Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if old.Version != expected {
			return ErrConflict
		}
		if expected > 0 && (old.Driver != a.Driver || old.Asset != a.Asset || old.Network != a.Network || old.TokenContract != a.TokenContract || old.TokenDecimals != a.TokenDecimals) {
			return ErrConflict
		}
		now, e := databaseTime(ctx, tx)
		if e != nil {
			return e
		}
		if a.EffectiveAt.IsZero() {
			a.EffectiveAt = now
		}
		if a.EffectiveAt.Before(now) || expected > 0 && a.EffectiveAt.Before(old.EffectiveAt) {
			return ErrInvalid
		}
		a.VersionID = uuid.NewString()
		a.Version = expected + 1
		a.PublishedBy = actor
		a.PublishedAt = now
		return tx.Create(&a).Error
	})
	return a, e
}
func (s *Service) AdminAssetNetworks(ctx context.Context, actor, platform string) ([]AssetNetwork, error) {
	out := []AssetNetwork{}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.requirePlatformAssetManager(ctx, tx, actor, platform); e != nil {
			return e
		}
		return tx.Order("published_at DESC,version_id").Limit(1000).Find(&out).Error
	})
	return out, e
}
func (s *Service) ListSupportedAssets(ctx context.Context) ([]SupportedAsset, error) {
	out := []SupportedAsset{}
	e := s.DB.WithContext(ctx).Raw(`SELECT id,version_id,version,driver,asset,network,token_contract,token_decimals,minimum_confirmations,effective_at FROM(SELECT DISTINCT ON(id) * FROM payments_asset_network_versions WHERE effective_at<=clock_timestamp() ORDER BY id,effective_at DESC,version DESC)a WHERE enabled ORDER BY driver,asset,network,id`).Scan(&out).Error
	return out, e
}
func (s *Service) assetVersion(ctx context.Context, tx *gorm.DB, versionID string, current bool) (AssetNetwork, error) {
	var a AssetNetwork
	if _, e := uuid.Parse(versionID); e != nil {
		return a, ErrNotConfigured
	}
	if e := tx.WithContext(ctx).First(&a, "version_id=?", versionID).Error; e != nil {
		return a, ErrNotConfigured
	}
	if !current {
		return a, nil
	}
	if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "payment-asset:"+a.ID).Error; e != nil {
		return a, e
	}
	var active AssetNetwork
	q := tx.Where("id=? AND effective_at<=clock_timestamp()", a.ID).Order("effective_at DESC,version DESC").Limit(1).Find(&active)
	if q.Error != nil {
		return a, q.Error
	}
	if q.RowsAffected == 0 || !active.Enabled || active.VersionID != a.VersionID {
		return a, ErrNotConfigured
	}
	return a, nil
}

// applyAssetNetwork derives public asset metadata from the platform version.
// Operators configure their account/address, not arbitrary supported tokens.
func (s *Service) applyAssetNetwork(ctx context.Context, tx *gorm.DB, m *Merchant, current bool) error {
	if m.Driver != "evm_wallet" && m.Driver != "nowpayments" {
		return nil
	}
	a, e := s.assetVersion(ctx, tx, m.AssetNetworkVersionID, current)
	if e != nil {
		return e
	}
	if a.Driver != m.Driver {
		return ErrConflict
	}
	if m.Credentials == nil {
		m.Credentials = map[string]string{}
	}
	assign := func(key, value string, caseInsensitive bool) error {
		old := m.Credentials[key]
		if old != "" {
			same := old == value
			if caseInsensitive {
				same = strings.EqualFold(old, value)
			}
			if !same {
				return ErrConflict
			}
		}
		m.Credentials[key] = value
		return nil
	}
	if m.Driver == "evm_wallet" {
		for _, v := range []struct {
			k, v string
			fold bool
		}{{"chain_id", a.Network, false}, {"asset", a.Asset, false}, {"token_contract", a.TokenContract, true}, {"token_decimals", strconv.Itoa(a.TokenDecimals), false}} {
			if e = assign(v.k, v.v, v.fold); e != nil {
				return e
			}
		}
		if m.Credentials["min_confirmations"] == "" {
			m.Credentials["min_confirmations"] = strconv.FormatInt(a.MinimumConfirmations, 10)
		}
		n, e := strconv.ParseInt(m.Credentials["min_confirmations"], 10, 64)
		if e != nil || n < a.MinimumConfirmations || n > 100000 {
			return ErrInvalid
		}
	} else {
		for _, v := range []struct {
			k, v string
			fold bool
		}{{"pay_currency", strings.ToLower(a.Asset), true}, {"pay_network", a.Network, false}, {"pay_token_contract", a.TokenContract, false}} {
			if e = assign(v.k, v.v, v.fold); e != nil {
				return e
			}
		}
		m.Credentials["pay_asset_verification_reference"] = "platform-asset-version:" + a.VersionID
	}
	return nil
}

func (s *Service) historicalMerchant(ctx context.Context, m Merchant, o Order) (Merchant, error) {
	// Disabling new collections must not prevent recording existing payments or
	// already executed returns. Authentication credentials remain the current
	// rotated credentials; immutable asset metadata comes from the old checkout.
	m.Enabled = true
	if o.AssetNetworkVersionID != "" {
		m.AssetNetworkVersionID = o.AssetNetworkVersionID
		credentials := map[string]string{}
		for k, v := range m.Credentials {
			credentials[k] = v
		}
		m.Credentials = credentials
		if o.MinimumConfirmations != "" {
			m.Credentials["min_confirmations"] = o.MinimumConfirmations
		}
		if e := s.applyAssetNetwork(ctx, s.DB, &m, false); e != nil {
			return m, e
		}
	}
	return m, nil
}
