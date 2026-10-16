package commerce

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) PublishPackage(ctx context.Context, tx *gorm.DB, in PublishPackageInput) (out PackageQuote, err error) {
	if !validID(in.OperatorID) || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.TermsReference) == "" || in.PriceUSD.Sign() <= 0 || in.PaidATG.Sign() <= 0 || in.GiftATG.Sign() < 0 || !in.PriceUSD.Valid() || !in.PaidATG.Valid() || !in.GiftATG.Valid() {
		return out, ErrInvalidInput
	}
	if _, err = ledger.PeriodEnd(s.now(), in.Period, in.Timezone); err != nil {
		return out, err
	}
	if in.Period == ledger.Lifetime && in.RenewalEnabled {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "sales.manage"); err != nil {
			return err
		}
		var p packageRow
		var before any
		version := int64(1)
		if in.PackageID == "" {
			if in.ExpectedVersion != 0 {
				return ErrConflict
			}
			p = packageRow{ID: uuid.NewString(), OperatorID: in.OperatorID, Name: strings.TrimSpace(in.Name), CreatedAt: s.now()}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
		} else {
			if !validID(in.PackageID) {
				return ErrInvalidInput
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, "id=?", in.PackageID).Error; err != nil {
				return err
			}
			if p.OperatorID != in.OperatorID {
				return ErrScope
			}
			var old quoteRow
			if p.CurrentQuoteID == nil {
				return ErrConflict
			}
			if err := tx.First(&old, "id=?", *p.CurrentQuoteID).Error; err != nil {
				return err
			}
			if old.Version != in.ExpectedVersion {
				return ErrConflict
			}
			version = old.Version + 1
			before = PackageQuote(old)
		}
		q := quoteRow{ID: uuid.NewString(), PackageID: p.ID, OperatorID: in.OperatorID, Version: version, PriceUSD: in.PriceUSD, PaidATG: in.PaidATG, GiftATG: in.GiftATG, Period: in.Period, Timezone: in.Timezone, TermsReference: in.TermsReference, CreatedAt: s.now()}
		if err := tx.Create(&q).Error; err != nil {
			return err
		}
		if err := tx.Model(&p).Updates(map[string]any{"name": strings.TrimSpace(in.Name), "current_quote_id": q.ID, "new_purchase_enabled": in.NewPurchaseEnabled, "renewal_enabled": in.RenewalEnabled}).Error; err != nil {
			return err
		}
		out = PackageQuote(q)
		return s.event(tx, p.OperatorID, p.ID, "package.published", map[string]any{"actor_id": in.ActorStaffID, "evidence": in.TermsReference, "before": before, "after": out})
	})
	return
}
func (s *Service) GetQuote(ctx context.Context, tx *gorm.DB, id string) (PackageQuote, error) {
	if !validID(id) {
		return PackageQuote{}, ErrInvalidInput
	}
	var r quoteRow
	err := s.dbFor(ctx, tx).First(&r, "id=?", id).Error
	return PackageQuote(r), classify(err)
}
func (s *Service) GetPackage(ctx context.Context, tx *gorm.DB, operator, id string) (Package, error) {
	if !validID(id) {
		return Package{}, ErrInvalidInput
	}
	var r packageRow
	err := s.dbFor(ctx, tx).First(&r, "id=? AND operator_id=?", id, operator).Error
	return Package(r), classify(err)
}

// RequiresRenewalConfirmation treats mixed better/worse changes as worse. A
// higher USD debit always needs consent, even when the ATG quantity increases.
func RequiresRenewalConfirmation(old, next PackageQuote) bool {
	rank := map[ledger.Period]int{ledger.Month: 1, ledger.Quarter: 3, ledger.Year: 12, ledger.Lifetime: 100000}
	return old.PackageID != next.PackageID || old.OperatorID != next.OperatorID || next.PriceUSD.Cmp(old.PriceUSD) > 0 || next.PaidATG.Add(next.GiftATG).Cmp(old.PaidATG.Add(old.GiftATG)) < 0 || next.PaidATG.Cmp(old.PaidATG) < 0 || rank[next.Period] < rank[old.Period] || old.Timezone != next.Timezone || old.TermsReference != next.TermsReference
}
func validity(d time.Duration) time.Duration {
	if d == 0 {
		return 30 * time.Minute
	}
	return d
}
