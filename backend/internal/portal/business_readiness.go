package portal

import (
	"context"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) Check(ctx context.Context, tx *gorm.DB, op organization.Operator, feature organization.BusinessFeature) ([]organization.Requirement, error) {
	requirements := []organization.Requirement{}
	add := func(code, description string, ready bool) {
		requirements = append(requirements, organization.Requirement{Code: code, Description: description, Ready: ready})
	}
	connection := tx
	if connection == nil {
		connection = a.DB
	}
	_, err := ledger.RequireMonetary(ctx, connection)
	if err != nil && !errors.Is(err, ledger.ErrMonetaryNotReady) {
		return nil, err
	}
	add("monetary_foundation", "平台清算币种和 ATG 基础比例已正式生效", err == nil)
	if feature == organization.Settlement {
		return requirements, nil
	}

	at := time.Now().UTC()
	if feature == organization.Registration {
		policy, err := a.Incentives.CurrentPolicy(ctx, tx, op.ID)
		if err != nil {
			return nil, err
		}
		add("verification_mail", "可发送验证邮件", a.Mail.AvailableForBrand(op.BrandID))
		reward := policy.InviteeATG.Add(policy.InviterATG)
		if reward.Sign() > 0 {
			fits, err := a.Cooperation.SaleFits(ctx, tx, op.ID, at, reward)
			if err != nil {
				return nil, err
			}
			add("reward_coverage", "奖励有真实覆盖承诺空间", fits)
		}
		return requirements, nil
	}

	switch feature {
	case organization.NewPurchase, organization.Renewal:
		quantity, err := a.minimumSaleATG(ctx, tx, op.ID, feature)
		if err != nil {
			return nil, err
		}
		hasPackage := quantity.Sign() > 0
		description := "已发布允许新购的套餐"
		if feature == organization.Renewal {
			description = "已发布允许续费的套餐"
		}
		add("package", description, hasPackage)
		if feature == organization.NewPurchase {
			methods, err := a.Payments.PublicMethods(ctx, op.ID)
			if err != nil {
				return nil, err
			}
			supported := false
			for _, method := range methods {
				supported = supported || method.SettlementSupported
			}
			add("payment", "至少有一种可用于当前结算币种的收款方式", supported)
		}
		if hasPackage {
			fits, err := a.Cooperation.SaleFits(ctx, tx, op.ID, at, quantity)
			if err != nil {
				return nil, err
			}
			add("coverage", "可售空间至少能覆盖一份符合条件的套餐", fits)
		}
	case organization.ModelUsage:
		directory := a.Catalog.WithDB(connection)
		directory.Operators = cooperation.CatalogDirectory{Organization: a.Organization, Tx: tx}
		models, err := directory.PublicCatalog(ctx, op.ID, at)
		if err != nil {
			return nil, err
		}
		ready := false
		for _, m := range models {
			ready = ready || m.Status == "available"
		}
		add("catalog_pricing", "公开模型、完整价格及经过验证的上游能力", ready)
		stock, err := a.hasProcurementStock(ctx, tx, op)
		if err != nil {
			return nil, err
		}
		add("coverage", "各供货层有可用的真实采购余额", stock)
	case organization.Activity:
		coverage, err := a.Cooperation.Coverage(ctx, tx, op.ID, at)
		if err != nil {
			return nil, err
		}
		add("coverage", "采购与已售责任覆盖可用", !coverage.HasUpstream || coverage.Space.Sign() > 0)
	}
	return requirements, nil
}

func (a *Application) minimumSaleATG(ctx context.Context, tx *gorm.DB, operator string, feature organization.BusinessFeature) (ledger.Decimal, error) {
	packages, err := a.Commerce.ListPackages(ctx, tx, operator)
	if err != nil {
		return ledger.Zero(), err
	}
	minimum := ledger.Zero()
	for _, p := range packages {
		if p.CurrentQuoteID == nil || feature == organization.NewPurchase && !p.NewPurchaseEnabled || feature == organization.Renewal && !p.RenewalEnabled {
			continue
		}
		quote, err := a.Commerce.GetQuote(ctx, tx, *p.CurrentQuoteID)
		if err != nil {
			return ledger.Zero(), err
		}
		if feature == organization.Renewal && quote.Period == ledger.Lifetime {
			continue
		}
		quantity := quote.PaidATG.Add(quote.GiftATG)
		if minimum.IsZero() || quantity.Cmp(minimum) < 0 {
			minimum = quantity
		}
	}
	return minimum, nil
}

func (a *Application) hasProcurementStock(ctx context.Context, tx *gorm.DB, op organization.Operator) (bool, error) {
	var buyers []string
	for op.Kind != organization.Platform {
		if op.ParentOperatorID == nil || len(buyers) == 2 {
			return false, organization.ErrInvalidRelationship
		}
		agreement, err := a.Organization.GetSupplyAgreement(ctx, tx, op.ID)
		if err != nil {
			return false, err
		}
		if agreement.Status != "active" {
			return false, nil
		}
		buyers = append(buyers, op.ID)
		op, err = a.Organization.GetOperator(ctx, tx, *op.ParentOperatorID)
		if err != nil {
			return false, err
		}
	}
	return a.Cooperation.HasProcurementStock(ctx, tx, buyers)
}

func businessUnavailable(err error) bool {
	return errors.Is(err, organization.ErrBusinessClosed) || errors.Is(err, organization.ErrFrozen) || errors.Is(err, ledger.ErrMonetaryNotReady)
}
