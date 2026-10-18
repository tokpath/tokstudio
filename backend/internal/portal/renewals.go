package portal

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
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
			delivery, e := a.Operations.GetDelivery(ctx, tx, notice.NotificationID)
			if e != nil {
				return e
			}
			read, e := a.Operations.GetNotificationRead(ctx, tx, notice.NotificationID)
			if e != nil {
				return e
			}
			accepted := read
			evidence := "customer-read:" + notice.NotificationID
			if delivery.State == "accepted" && delivery.AcceptedAt != nil {
				accepted = delivery.AcceptedAt
				evidence = "smtp-accepted:" + notice.NotificationID
			}
			if accepted != nil {
				if e = a.Commerce.RecordRenewalNotice(ctx, tx, s.ID, quote.ID, s.NextDueAt, evidence); e != nil {
					return e
				}
				if e = tx.Model(&renewalNotice{}).Where("subscription_id=? AND quote_id=? AND cycle_due_at=?", s.ID, quote.ID, s.NextDueAt).Update("accepted_at", accepted).Error; e != nil {
					return e
				}
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
