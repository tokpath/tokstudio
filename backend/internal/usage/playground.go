package usage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
)

// PlaygroundKey is an authenticated-session capability. Its random token is
// discarded and the API-key authenticator excludes this kind entirely.
func (s *Service) PlaygroundKey(ctx context.Context, customer, timezone string) (Key, error) {
	var out Key
	at := s.wallNow()
	if _, e := ledger.BudgetWindow(at, at, ledger.Lifetime, timezone); e != nil {
		return out, e
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "playground-key:"+customer).Error; e != nil {
			return e
		}
		e := t.First(&out, "customer_id=? AND kind='playground'", customer).Error
		if e == nil {
			if out.Status != "active" {
				return ErrKey
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		c, e := s.Organization.GetCustomer(ctx, t, customer)
		if e != nil {
			return e
		}
		if c.Status != "active" {
			return ErrKey
		}
		secret, e := platformcrypto.RandomToken("web_")
		if e != nil {
			return e
		}
		now, e := s.businessTime(t)
		if e != nil {
			return e
		}
		rpm, concurrency := int64(30), int64(3)
		out = Key{ID: uuid.NewString(), CustomerID: customer, BrandID: c.BrandID, Name: "网页体验", Kind: "playground", Prefix: "web", TokenHash: platformcrypto.HashToken(secret), Status: "active", ModelIDs: json.RawMessage("null"), Period: ledger.Lifetime, Timezone: timezone, RateLimitRPM: &rpm, ConcurrencyLimit: &concurrency, Version: 1, CreatedAt: now, UpdatedAt: now}
		return t.Create(&out).Error
	})
	return out, e
}
func (s *Service) GetKey(ctx context.Context, customer, id string) (Key, error) {
	var out Key
	e := s.DB.WithContext(ctx).First(&out, "id=? AND customer_id=? AND kind='api'", id, customer).Error
	return out, e
}
