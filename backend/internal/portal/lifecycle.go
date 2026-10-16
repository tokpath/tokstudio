package portal

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) Stop(ctx context.Context, tx *gorm.DB, in organization.CustomerLifecycleInput) error {
	if e := a.Usage.DisableCustomerKeys(ctx, tx, in.CustomerID, in.Evidence); e != nil {
		return e
	}
	return a.Commerce.StopCustomerRenewals(ctx, tx, in.OperatorID, in.CustomerID, in.Evidence)
}
func (a *Application) BeginClose(ctx context.Context, tx *gorm.DB, in organization.CustomerLifecycleInput) error {
	if e := a.Stop(ctx, tx, in); e != nil {
		return e
	}
	if e := a.Incentives.CloseCustomerPromotion(ctx, tx, in.OperatorID, in.CustomerID, in.Evidence, in.EffectiveAt); e != nil {
		return e
	}
	if in.WaiverEvidence != "" {
		if in.ActorID != in.CustomerID {
			return organization.ErrForbidden
		}
		return a.Ledger.WaiveCustomerCredits(ctx, tx, in.OperatorID, in.CustomerID, "closure:"+in.CustomerID, in.ActorID, in.WaiverEvidence, in.EffectiveAt)
	}
	return nil
}
func (a *Application) CheckClose(ctx context.Context, tx *gorm.DB, in organization.CustomerLifecycleInput) ([]organization.Requirement, error) {
	checks, e := a.Usage.ClosureRequirements(ctx, tx, in.CustomerID)
	if e != nil {
		return nil, e
	}
	inc, e := a.Incentives.ClosureRequirements(ctx, tx, in.OperatorID, in.CustomerID)
	if e != nil {
		return nil, e
	}
	checks = append(checks, inc...)
	customer, e := a.Organization.GetCustomer(ctx, tx, in.CustomerID)
	if e != nil {
		return nil, e
	}
	extra, e := a.Remediation.CustomerClosureRequirements(ctx, tx, in.CustomerID, customer.BrandID)
	if e != nil {
		return nil, e
	}
	checks = append(checks, extra...)
	// Pending original-currency facts survive commercial order closure and
	// operator takeover. Only this stable customer's retail order IDs qualify.
	pending := false
	cursor := ""
	for {
		rows, e := a.Commerce.ListCustomerOrdersPage(ctx, tx, in.CustomerID, cursor, "", 100)
		if e != nil {
			return nil, e
		}
		byOperator := map[string][]string{}
		for _, r := range rows {
			source := r.OriginalOperatorID
			if source == "" {
				source = r.OperatorID
			}
			byOperator[source] = append(byOperator[source], r.ID)
		}
		for source, ids := range byOperator {
			facts, e := a.Payments.PendingReceiptObligations(ctx, tx, source, ids)
			if e != nil {
				return nil, e
			}
			pending = pending || len(facts) > 0
			links, e := a.Payments.RecordedReceiptLinks(ctx, tx, source, ids)
			if e != nil {
				return nil, e
			}
			receiptIDs := []string{}
			for _, link := range links {
				receiptIDs = append(receiptIDs, link.ReceiptID)
			}
			if len(receiptIDs) > 0 {
				funds, e := a.Commerce.UnsettledReceiptFunds(ctx, tx, source, receiptIDs)
				if e != nil {
					return nil, e
				}
				pending = pending || len(funds) > 0
			}
		}
		if len(rows) < 100 {
			break
		}
		cursor = commerce.NextOrderCursor(rows[len(rows)-1])
	}
	checks = append(checks, organization.Requirement{Code: "pending_receipts", Description: "原币真实已收款仍待核实付款时点、估值或入账；须按原订单处理，不能重复付款", Ready: !pending})
	account, e := a.Ledger.FindAccount(ctx, tx, in.OperatorID, in.CustomerID, "customer:usd")
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	checks = append(checks, organization.Requirement{Code: "usd_balance", Description: "USD余额及占用须通过原路退款等真实结算归零", Ready: account.Balance.IsZero() && account.Held.IsZero()})
	grants, e := a.Ledger.CustomerGrants(ctx, tx, in.OperatorID, in.CustomerID)
	if e != nil {
		return nil, e
	}
	paid, gift := ledger.Zero(), ledger.Zero()
	for start := 0; start < len(grants); start += 100 {
		end := start + 100
		if end > len(grants) {
			end = len(grants)
		}
		ids := []string{}
		for _, g := range grants[start:end] {
			ids = append(ids, g.ID)
		}
		remaining, e := a.Ledger.GrantRemainders(ctx, tx, in.OperatorID, in.CustomerID, ids)
		if e != nil {
			return nil, e
		}
		for _, r := range remaining {
			paid = paid.Add(r.PurchasedATG)
			gift = gift.Add(r.GiftATG)
		}
	}
	checks = append(checks, organization.Requirement{Code: "remaining_paid_grants", Description: "购买ATG仍有余额；可继续使用、等待到期或本人明确放弃，不能按现价兑现", Ready: paid.IsZero()}, organization.Requirement{Code: "remaining_gifts", Description: "剩余赠送ATG将在正式注销的同一事务内失效，不要求放弃购买资产来清除纯赠送", Ready: true})
	// Several independent domains contribute payouts/disputes. Every answer must
	// be ready; overwriting one result with another would hide an obligation.
	byCode := map[string]organization.Requirement{}
	order := []string{}
	for _, r := range checks {
		prior, exists := byCode[r.Code]
		if !exists {
			byCode[r.Code] = r
			order = append(order, r.Code)
		} else {
			prior.Ready = prior.Ready && r.Ready
			prior.Description += "；" + r.Description
			byCode[r.Code] = prior
		}
	}
	out := []organization.Requirement{}
	for _, code := range order {
		out = append(out, byCode[code])
	}
	return out, nil
}
func (a *Application) CloseCustomerLifecycle(ctx context.Context, tx *gorm.DB, in organization.CustomerLifecycleInput) error {
	if e := a.Ledger.CancelClosureGifts(ctx, tx, in.OperatorID, in.CustomerID, "formal-close:"+in.CustomerID, in.ActorID, in.Evidence, in.EffectiveAt); e != nil {
		return e
	}
	return tx.WithContext(ctx).Where("subject_id=? AND kind='customer'", in.CustomerID).Delete(&browserSession{}).Error
}
func (a *Application) customerLifecycle(c *gin.Context) {
	var in struct {
		Version  int64  `json:"version"`
		Evidence string `json:"evidence"`
		Waiver   string `json:"waiver_evidence"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	input := organization.CustomerLifecycleInput{CustomerID: id.Customer.Customer.ID, OperatorID: id.Customer.Ownership.OperatorID, ActorID: id.Customer.Customer.ID, ExpectedVersion: in.Version, Evidence: in.Evidence, WaiverEvidence: in.Waiver}
	raw, e := a.command(c, "customer-lifecycle:"+input.CustomerID+":"+c.Param("action"), in, func(tx *gorm.DB) (any, error) {
		var out organization.Customer
		var err error
		switch c.Param("action") {
		case "stop":
			out, err = a.Organization.StopUsing(c.Request.Context(), tx, input, lifecycleHandler{a})
		case "resume":
			out, err = a.Organization.ResumeUsing(c.Request.Context(), tx, input, lifecycleHandler{a})
		case "begin-close":
			out, err = a.Organization.BeginCustomerClosure(c.Request.Context(), tx, input, lifecycleHandler{a})
		case "complete-close":
			out, err = a.Organization.CompleteCustomerClosure(c.Request.Context(), tx, input, lifecycleHandler{a})
		default:
			err = organization.ErrInvalidInput
		}
		if err != nil {
			return nil, err
		}
		return gin.H{"id": out.ID, "status": out.Status, "version": out.Version}, nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) closureStatus(c *gin.Context) {
	id := subject(c)
	var checks []organization.Requirement
	e := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		checks, err = a.CheckClose(c.Request.Context(), tx, organization.CustomerLifecycleInput{CustomerID: id.Customer.Customer.ID, OperatorID: id.Customer.Ownership.OperatorID, ActorID: id.Customer.Customer.ID, EffectiveAt: time.Now().UTC()})
		return err
	})
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"customer": id.Customer.Customer, "requirements": checks})
}
func (a *Application) waiveClosingCredits(c *gin.Context) {
	var in struct {
		Evidence string `json:"waiver_evidence"`
		Version  int64
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	if in.Evidence == "" || id.Customer.Customer.Status != "closing" || id.Customer.Customer.Version != in.Version {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	raw, e := a.command(c, "closure-waiver:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		if _, e := a.Organization.LockOwnership(c.Request.Context(), tx, id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, id.Customer.Ownership.Version); e != nil {
			return nil, e
		}
		var at time.Time
		if e := tx.Raw("SELECT clock_timestamp()").Row().Scan(&at); e != nil {
			return nil, e
		}
		e := a.Ledger.WaiveCustomerCredits(c.Request.Context(), tx, id.Customer.Ownership.OperatorID, id.Customer.Customer.ID, "closing:"+id.Customer.Customer.ID, id.Customer.Customer.ID, in.Evidence, at)
		return commandResult(id.Customer.Customer.ID, "remaining_atg_waived"), e
	})
	jsonResponse(c, raw, e)
}

type lifecycleHandler struct{ *Application }

func (h lifecycleHandler) Close(ctx context.Context, tx *gorm.DB, in organization.CustomerLifecycleInput) error {
	return h.CloseCustomerLifecycle(ctx, tx, in)
}
