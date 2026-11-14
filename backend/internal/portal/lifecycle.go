package portal

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
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
	// Check outstanding facts against the original collector, including after
	// order closure or takeover. Paid, unfulfilled orders are obligations even
	// when the funds came from the wallet or a manual receipt.
	var pending bool
	if e := tx.WithContext(ctx).Raw(`SELECT EXISTS (
		SELECT 1 FROM commerce_orders o WHERE o.customer_id=? AND (
			o.status IN ('pending','partial','paid','exception')
			OR EXISTS (SELECT 1 FROM payments_intents i JOIN payments_merchants m ON m.id=i.merchant_id
				WHERE i.order_id=o.id::text AND i.operator_id=o.original_operator_id::text
				  AND m.driver<>'manual' AND i.state NOT IN ('failed','closed','verified'))
			OR EXISTS (SELECT 1 FROM payments_verified_facts f
				LEFT JOIN ledger_accounts cash ON cash.owner_id::text=f.receipt_id AND cash.operator_id=o.original_operator_id AND cash.purpose='receipt:unallocated'
				WHERE f.order_id=o.id::text AND f.operator_id=o.original_operator_id::text AND f.kind='receipt'
				  AND ((COALESCE(f.receipt_id,'')='' AND f.state IN ('awaiting_settlement','verified_pending_receipt','verified')) OR cash.balance>0))
		))`, in.CustomerID).Scan(&pending).Error; e != nil {
		return nil, e
	}
	checks = append(checks, organization.Requirement{Code: "pending_receipts", Description: "待付款、待交付或待核实原收款的订单，须先在原订单中处理。", Ready: !pending})
	account, e := a.Ledger.FindAccount(ctx, tx, in.OperatorID, in.CustomerID, "customer:money")
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	waiving := in.WaiveBalance != nil && in.ActorID == in.CustomerID && in.WaiveBalance.Sign() > 0 && in.WaiveBalance.Cmp(account.Balance) == 0
	checks = append(checks, organization.Requirement{Code: "money_balance", Description: "支付余额须结清，或由本人明确放弃。已有资金占用须先处理。", Ready: account.Held.IsZero() && (account.Balance.IsZero() || waiving)})
	var paid bool
	if e := tx.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM ledger_grants g JOIN ledger_grant_sources src ON src.grant_id=g.id
		WHERE g.operator_id=? AND g.customer_id=? AND src.kind='paid' AND src.quantity-src.consumed-src.expired-src.cancelled>0)`, in.OperatorID, in.CustomerID).Scan(&paid).Error; e != nil {
		return nil, e
	}
	checks = append(checks, organization.Requirement{Code: "remaining_paid_grants", Description: "剩余购买 ATG 可等待到期或由本人明确放弃。", Ready: !paid}, organization.Requirement{Code: "remaining_gifts", Description: "剩余赠送 ATG 将在正式注销时失效。", Ready: true})
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
	if in.WaiveBalance != nil {
		if in.ActorID != in.CustomerID {
			return organization.ErrForbidden
		}
		if e := a.Ledger.WaiveCustomerBalance(ctx, tx, in.OperatorID, in.CustomerID, in.Evidence, *in.WaiveBalance); e != nil {
			return e
		}
	}
	if e := a.Ledger.CancelClosureGifts(ctx, tx, in.OperatorID, in.CustomerID, "formal-close:"+in.CustomerID, in.ActorID, in.Evidence, in.EffectiveAt); e != nil {
		return e
	}
	return tx.WithContext(ctx).Where("subject_id=? AND kind='customer'", in.CustomerID).Delete(&browserSession{}).Error
}
func (a *Application) customerLifecycle(c *gin.Context) {
	var in struct {
		Version      int64           `json:"version"`
		Evidence     string          `json:"evidence"`
		Waiver       string          `json:"waiver_evidence"`
		WaiveBalance *ledger.Decimal `json:"waive_balance_money"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	if in.WaiveBalance != nil {
		in.Evidence = "customer confirmed permanent closure and waiver of " + in.WaiveBalance.String() + " settlement currency units"
	} else if in.Evidence == "" {
		in.Evidence = "customer-action:" + c.Param("action")
	}
	if in.WaiveBalance != nil && (c.Param("action") != "complete-close" || !in.WaiveBalance.Valid() || in.WaiveBalance.Sign() <= 0) {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	input := organization.CustomerLifecycleInput{CustomerID: id.Customer.Customer.ID, OperatorID: id.Customer.Ownership.OperatorID, ActorID: id.Customer.Customer.ID, ExpectedVersion: in.Version, Evidence: in.Evidence, WaiverEvidence: in.Waiver, WaiveBalance: in.WaiveBalance}
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
