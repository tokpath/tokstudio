package portal

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type priceCheck struct {
	PriceID, OperatorID, Status string
	Snapshot                    json.RawMessage `gorm:"type:jsonb"`
	ErrorCode                   string
	CheckedAt                   time.Time
}

func (priceCheck) TableName() string { return "portal_price_checks" }

// Price publication must survive a newly discovered coverage gap. Every exact
// affected responsibility gets a persistent result and failed checks have a
// durable retry; no existing grant is rewritten or deleted.
func (a *Application) pricePublished(ctx context.Context, tx *gorm.DB, v catalog.PriceVersion) error {
	if _, e := a.recheckPrice(ctx, tx, v); e != nil {
		return e
	}
	payload, _ := json.Marshal(v)
	_, e := a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-price-check", Kind: "coverage", BusinessKey: v.ID, Payload: payload, MaxAttempts: 20})
	return e
}
func (a *Application) recheckPrice(ctx context.Context, tx *gorm.DB, v catalog.PriceVersion) (bool, error) {
	// Traversal runs as a trusted system task. Results are presented only in the
	// precise operator's own workspace; parent financial views do not expand.
	pending := []string{a.Installation.OperatorID}
	operators := []organization.Operator{}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		op, e := a.Organization.GetOperator(ctx, tx, id)
		if e != nil {
			return false, e
		}
		operators = append(operators, op)
		children, e := a.Organization.Children(ctx, tx, id)
		if e != nil {
			return false, e
		}
		for _, child := range children {
			pending = append(pending, child.ID)
		}
	}
	allReady := true
	for _, op := range operators {
		point := "coverage_" + strings.ReplaceAll(op.ID, "-", "")
		if e := tx.SavePoint(point).Error; e != nil {
			return false, e
		}
		snapshot, e := a.Cooperation.Coverage(ctx, tx, op.ID, v.EffectiveAt)
		status, code := "ready", ""
		if e != nil {
			if rollback := tx.RollbackTo(point).Error; rollback != nil {
				return false, rollback
			}
			status = "recheck_required"
			code = "coverage_dependency_unavailable"
			allReady = false
		} else if snapshot.HasUpstream && snapshot.Space.Sign() < 0 {
			status = "gap"
			code = "insufficient_sales_space"
		}
		raw, _ := json.Marshal(snapshot)
		row := priceCheck{PriceID: v.ID, OperatorID: op.ID, Status: status, Snapshot: raw, ErrorCode: code, CheckedAt: time.Now().UTC()}
		var prior priceCheck
		if e := tx.Where("price_id=? AND operator_id=?", v.ID, op.ID).Limit(1).Find(&prior).Error; e != nil {
			return false, e
		}
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "price_id"}, {Name: "operator_id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "snapshot", "error_code", "checked_at"})}).Create(&row).Error; e != nil {
			return false, e
		}
		if status != "ready" || prior.Status != "" && prior.Status != "ready" {
			recipients, e := a.Organization.OperatorNotificationStaff(ctx, tx, op.ID)
			if e != nil {
				return false, e
			}
			for _, recipient := range recipients {
				title, description, severity := "调价后的责任覆盖需要处理", "请检查本经营方采购、价格与已售责任。已售权益保留；新承诺将按实时覆盖检查受限。", "warning"
				if status == "ready" {
					title = "责任覆盖已恢复"
					description = "本经营方调价后的覆盖重算已恢复，请查看覆盖明细。"
					severity = "info"
				}
				_, e = a.Operations.Emit(ctx, tx, operations.Notification{OperatorID: op.ID, RecipientID: recipient, BusinessKey: "price-check:" + v.ID + ":" + status, Category: "mandatory", Severity: severity, Title: title, Body: description, Reference: "price:" + v.ID})
				if e != nil {
					return false, e
				}
			}
		}
	}
	return allReady, nil
}
func (a *Application) RunPriceChecks(ctx context.Context, owner string, limit int) error {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		claimed, e := a.Jobs.Claim(ctx, "portal-price-check", owner, 30*time.Second, 1)
		if e != nil {
			return e
		}
		if len(claimed) == 0 {
			return nil
		}
		job := claimed[0]
		var v catalog.PriceVersion
		e = json.Unmarshal(job.Payload, &v)
		ready := false
		if e == nil {
			e = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; ready, err = a.recheckPrice(ctx, tx, v); return err })
		}
		if e != nil || !ready {
			_ = a.Jobs.Fail(ctx, job.ID, owner, job.Fence, "coverage recheck requires dependency recovery")
			continue
		}
		if e = a.Jobs.Complete(ctx, job.ID, owner, job.Fence); e != nil {
			return e
		}
	}
	return nil
}

var _ cooperation.CoverageSnapshot

// Model removal is applied provisionally in the controller transaction. Any
// outstanding obligation that can no longer be served rolls the change back.
func (a *Application) validateCatalogChange(ctx context.Context, tx *gorm.DB, change catalog.CatalogChange) error {
	start := change.OperatorID
	if change.Kind == "authorization" {
		var flag struct {
			BuyerID string `json:"buyer_id"`
		}
		if json.Unmarshal(change.Payload, &flag) != nil || flag.BuyerID == "" {
			return catalog.ErrInvalidInput
		}
		start = flag.BuyerID
	}
	operators, e := a.Organization.CatalogNotificationOperators(ctx, tx, start)
	if e != nil {
		return e
	}
	for _, op := range operators {
		snapshot, e := a.Cooperation.Coverage(ctx, tx, op, change.EffectiveAt)
		if e != nil {
			return e
		}
		if snapshot.HasUpstream && snapshot.Space.Sign() < 0 {
			return catalog.ErrChangeNotice
		}
	}
	return nil
}
