package cooperation

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

type CombinedRefundApproval struct {
	Procurement *ProcurementRefunds
	Other       payments.RefundApproval
}

func (c CombinedRefundApproval) RequireApprovedRefund(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund) error {
	if c.Procurement != nil {
		e := c.Procurement.RequireApprovedRefund(ctx, tx, actor, r)
		if !errors.Is(e, ErrNotProcurementRefund) {
			return e
		}
	}
	if c.Other == nil {
		return payments.ErrNotConfigured
	}
	return c.Other.RequireApprovedRefund(ctx, tx, actor, r)
}

type CombinedRefundSink struct {
	Procurement *ProcurementRefunds
	Other       payments.RefundFactSink
}

func (c CombinedRefundApproval) RequireExternalRefundFact(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund, d payments.ExternalRefundDestination) error {
	if c.Procurement != nil {
		e := c.Procurement.RequireExternalRefundFact(ctx, tx, actor, r, d)
		if !errors.Is(e, ErrNotProcurementRefund) {
			return e
		}
	}
	other, ok := c.Other.(payments.ExternalRefundApproval)
	if !ok {
		return payments.ErrNotConfigured
	}
	return other.RequireExternalRefundFact(ctx, tx, actor, r, d)
}

func (c CombinedRefundApproval) RequireResponsibleRefundExecution(ctx context.Context, tx *gorm.DB, actor, operator, legID string, d payments.ExternalRefundDestination) (payments.ResponsibleRefundBasis, error) {
	if c.Procurement != nil {
		basis, e := c.Procurement.RequireResponsibleRefundExecution(ctx, tx, actor, operator, legID, d)
		if !errors.Is(e, ErrNotProcurementRefund) {
			return basis, e
		}
	}
	other, ok := c.Other.(payments.ResponsibleRefundAuthority)
	if !ok {
		return payments.ResponsibleRefundBasis{}, payments.ErrNotConfigured
	}
	return other.RequireResponsibleRefundExecution(ctx, tx, actor, operator, legID, d)
}

func (c CombinedRefundSink) RecordVerifiedRefund(ctx context.Context, tx *gorm.DB, m payments.Merchant, o payments.Order, r payments.Refund, f payments.Fact, v payments.USDValue) error {
	if c.Procurement != nil {
		e := c.Procurement.RecordVerifiedRefund(ctx, tx, m, o, r, f, v)
		if !errors.Is(e, ErrNotProcurementRefund) {
			return e
		}
	}
	if c.Other == nil {
		return payments.ErrNotConfigured
	}
	return c.Other.RecordVerifiedRefund(ctx, tx, m, o, r, f, v)
}
