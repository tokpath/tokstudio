package commerce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) PublishPackage(ctx context.Context, tx *gorm.DB, in PublishPackageInput) (out PackageQuote, err error) {
	if !validID(in.OperatorID) || strings.TrimSpace(in.Name) == "" || len(in.TermsSummary) > 10000 || in.PriceUSD.Sign() <= 0 || in.PaidATG.Sign() <= 0 || in.GiftATG.Sign() < 0 || !in.PriceUSD.Valid() || !in.PaidATG.Valid() || !in.GiftATG.Valid() {
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
		terms := strings.TrimSpace(in.TermsSummary)
		if terms == "" {
			terms = DefaultPackageTerms
		} else if !strings.HasPrefix(terms, DefaultPackageTerms) {
			terms = DefaultPackageTerms + "\n" + terms
		}
		quoteID := uuid.NewString()
		q := quoteRow{ID: quoteID, PackageID: p.ID, OperatorID: in.OperatorID, Version: version, PriceUSD: in.PriceUSD, PaidATG: in.PaidATG, GiftATG: in.GiftATG, Period: in.Period, Timezone: in.Timezone, TermsReference: fmt.Sprintf("package-terms:%s:v%d", p.ID, version), TermsSummary: terms, CreatedAt: s.now()}
		if err := tx.Create(&q).Error; err != nil {
			return err
		}
		if err := tx.Model(&p).Updates(map[string]any{"name": strings.TrimSpace(in.Name), "current_quote_id": q.ID, "new_purchase_enabled": in.NewPurchaseEnabled, "renewal_enabled": in.RenewalEnabled}).Error; err != nil {
			return err
		}
		out = PackageQuote(q)
		return s.event(tx, p.OperatorID, p.ID, "package.published", map[string]any{"actor_id": in.ActorStaffID, "evidence": q.TermsReference, "before": before, "after": out})
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
	return old.PackageID != next.PackageID || old.OperatorID != next.OperatorID || next.PriceUSD.Cmp(old.PriceUSD) > 0 || next.PaidATG.Add(next.GiftATG).Cmp(old.PaidATG.Add(old.GiftATG)) < 0 || next.PaidATG.Cmp(old.PaidATG) < 0 || rank[next.Period] < rank[old.Period] || old.Timezone != next.Timezone || packageTermsChanged(old, next)
}
func validity(d time.Duration) time.Duration {
	if d == 0 {
		return 30 * time.Minute
	}
	return d
}

// DefaultPackageTerms states the fixed business policy; a display reference
// supplied by a client never becomes the legal rule or a renewal comparison.
const DefaultPackageTerms = "通用ATG额度可用于当前经营方已上架模型，模型价格按调用时生效报价计算。先到期的额度先使用，同一订单内购买与各来源赠送额度按比例消耗。购买与套餐赠送额度按成交期限有效，到期余量不结转。套餐不支持一般性退款，可停止未来续费；重复或错误交易、未正确交付及无法履约按异常流程办理。"

func packageTermsChanged(old, next PackageQuote) bool {
	if old.TermsSummary == "" || next.TermsSummary == "" {
		// Existing accepted quotes without readable content cannot silently
		// acquire new legal terms from a later publication.
		return old.TermsSummary != next.TermsSummary || old.TermsReference != next.TermsReference
	}
	return old.TermsSummary != next.TermsSummary
}

func PackageTermsSummary(q PackageQuote) string {
	if strings.TrimSpace(q.TermsSummary) == "" {
		return DefaultPackageTerms
	}
	return q.TermsSummary
}
