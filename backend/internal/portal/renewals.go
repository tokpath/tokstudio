package portal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type renewalNotice struct {
	SubscriptionID, QuoteID string
	CycleDueAt              time.Time
	NotificationID          string
	AcceptedAt              *time.Time
}

func (renewalNotice) TableName() string { return "portal_renewal_notices" }
func (a *Application) runRenewal(ctx context.Context, p workerPayload) error {
	var subscription commerce.Subscription
	e := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		s, e := a.Commerce.GetSubscription(ctx, tx, p.CustomerID, p.ID)
		if e != nil {
			return e
		}
		subscription = s
		if s.Status == "cancelled" || s.Status == "needs_confirmation" {
			return nil
		}
		pkg, e := a.Commerce.GetPackage(ctx, tx, s.OperatorID, s.PackageID)
		if e != nil {
			return e
		}
		if pkg.CurrentQuoteID == nil || !pkg.RenewalEnabled {
			return nil
		}
		quote, e := a.Commerce.GetQuote(ctx, tx, *pkg.CurrentQuoteID)
		if e != nil {
			return e
		}
		var notice renewalNotice
		q := tx.Where("subscription_id=? AND quote_id=? AND cycle_due_at=?", s.ID, quote.ID, s.NextDueAt).Limit(1).Find(&notice)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			n, e := a.Operations.Emit(ctx, tx, operations.Notification{OperatorID: s.OperatorID, RecipientID: s.CustomerID, BusinessKey: "renewal-notice:" + s.ID + ":" + s.NextDueAt.UTC().Format(time.RFC3339Nano) + ":" + quote.ID, Category: "mandatory", Severity: "info", Title: "套餐续费条款与资金提醒", Body: "下次续费于 " + s.NextDueAt.UTC().Format(time.RFC3339) + " 到期，价格 " + quote.PriceUSD.String() + " USD，购买 " + quote.PaidATG.String() + " ATG，赠送 " + quote.GiftATG.String() + " ATG，有效期 " + string(quote.Period) + "。请核对条款；不利变化需要本人重新确认，资金不足不会产生客户欠款。\n" + commerce.PackageTermsSummary(quote), Reference: "subscription:" + s.ID})
			if e != nil {
				return e
			}
			notice = renewalNotice{SubscriptionID: s.ID, QuoteID: quote.ID, CycleDueAt: s.NextDueAt, NotificationID: n.ID}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notice).Error; e != nil {
				return e
			}
		}
		if notice.AcceptedAt == nil {
			// The existing authorization permits unchanged/improved terms.
			// Publishing the in-app notice is sufficient; reading it or an
			// external email delivery must not become another consent gate.
			if e = a.Commerce.RecordRenewalNotice(ctx, tx, s.ID, quote.ID, s.NextDueAt, "in-app-published:"+notice.NotificationID); e != nil {
				return e
			}
			now := time.Now().UTC()
			if e = tx.Model(&renewalNotice{}).Where("subscription_id=? AND quote_id=? AND cycle_due_at=?", s.ID, quote.ID, s.NextDueAt).Update("accepted_at", now).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	if subscription.Status == "cancelled" || subscription.Status == "needs_confirmation" || time.Now().Before(subscription.NextDueAt) {
		return nil
	}
	_, e = a.Commerce.RenewSubscription(ctx, nil, subscription.ID)
	return e
}

func (a *Application) enqueuePackageChange(ctx context.Context, tx *gorm.DB, quote commerce.PackageQuote, cursor string) error {
	payload, _ := json.Marshal(workerPayload{OperatorID: quote.OperatorID, ID: quote.ID, Cursor: cursor})
	_, err := a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-maintenance", Kind: "package_change", BusinessKey: "package-change:" + quote.ID + ":" + cursor, Payload: payload, MaxAttempts: 12})
	return err
}

// Bounded, durable fanout keeps publication quick even for a popular package.
// Notice delivery never changes customer authorizations or existing entitlements.
func (a *Application) runPackageChange(ctx context.Context, p workerPayload) error {
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		quote, err := a.Commerce.GetQuote(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		pkg, err := a.Commerce.GetPackage(ctx, tx, p.OperatorID, quote.PackageID)
		if err != nil {
			return err
		}
		if pkg.CurrentQuoteID == nil || *pkg.CurrentQuoteID != quote.ID {
			return nil
		}
		var subscribers []commerce.Subscription
		query := tx.Table("commerce_subscriptions").Where("operator_id=? AND package_id=? AND status<>'cancelled' AND created_at<=?", p.OperatorID, pkg.ID, quote.CreatedAt)
		if p.Cursor != "" {
			query = query.Where("id>?", p.Cursor)
		}
		if err = query.Order("id").Limit(200).Find(&subscribers).Error; err != nil {
			return err
		}
		for _, subscription := range subscribers {
			title, text := "套餐续费条件更新", "套餐「"+pkg.Name+"」的后续续费条件已更新。下次报价为 "+quote.PriceUSD.String()+" USD，购买 "+quote.PaidATG.String()+" ATG，赠送 "+quote.GiftATG.String()+" ATG。价格上涨或权益减少须在套餐页面重新确认，否则不会扣款；当前已购权益不受影响。"
			if !pkg.RenewalEnabled {
				title, text = "套餐已停止续费", "经营方已停止套餐「"+pkg.Name+"」的后续续费，当前已购额度和有效期保持不变。可在套餐页面选择其他在售套餐。"
			}
			if _, err = a.Operations.Emit(ctx, tx, operations.Notification{OperatorID: p.OperatorID, RecipientID: subscription.CustomerID, BusinessKey: "package-change:" + quote.ID + ":" + subscription.ID, Category: "mandatory", Severity: "info", Title: title, Body: text, Reference: "subscription:" + subscription.ID}); err != nil {
				return err
			}
		}
		if len(subscribers) == 200 {
			return a.enqueuePackageChange(ctx, tx, quote, subscribers[len(subscribers)-1].ID)
		}
		return nil
	})
}
