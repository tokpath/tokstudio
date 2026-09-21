package app

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
)

type commissionBridge struct {
	identity *identity.Service
	comm     *commission.Service
}

func (b *commissionBridge) AccrueUsageTx(tx *gorm.DB, usageEventID, requestID, userID, channelOrgID string, wholesaleMinor int64) error {
	attr, eligible, market, err := b.identity.CommissionContextTx(tx, userID, channelOrgID)
	if err != nil {
		return err
	}
	if channelOrgID == "" {
		channelOrgID = attr.ChannelOrgID
	}
	_, err = b.comm.AccrueTx(tx, commission.AccrueInput{UsageEventID: usageEventID, RequestID: requestID, UserID: userID, ChannelOrgID: channelOrgID, WholesaleMinor: wholesaleMinor, RoleID: attr.AcquisitionRoleID, RoleType: attr.RoleType, ParentRoleID: attr.ParentRoleID, CanCommission: eligible, PolicyChannelID: market})
	return err
}
func (b *commissionBridge) RecalcUsageTx(tx *gorm.DB, usageID string, base int64) (*billing.CommissionView, error) {
	return b.comm.RecalculateTx(tx, usageID, base)
}

func (b *commissionBridge) ReverseUsage(ctx context.Context, usageEventID string) error {
	return b.comm.Reverse(ctx, usageEventID)
}

func (b *commissionBridge) ReverseUsageTx(tx *gorm.DB, usageEventID string) error {
	return b.comm.ReverseTx(tx, usageEventID)
}

func (b *commissionBridge) Totals(ctx context.Context) (int64, int64, error) {
	return b.comm.Totals(ctx)
}

func (b *commissionBridge) LockLifecycleTx(tx *gorm.DB) error { return b.comm.LockLifecycleTx(tx) }
