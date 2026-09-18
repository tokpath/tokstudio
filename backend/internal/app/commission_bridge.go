package app

import (
	"context"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
)

type commissionBridge struct {
	identity *identity.Service
	comm     *commission.Service
}

func (b *commissionBridge) AccrueUsage(ctx context.Context, usageEventID, requestID, userID, channelOrgID string, wholesaleMinor int64) error {
	in := commission.AccrueInput{
		UsageEventID: usageEventID, RequestID: requestID, UserID: userID,
		ChannelOrgID: channelOrgID, WholesaleMinor: wholesaleMinor,
	}
	if userID != "" {
		if attr, err := b.identity.GetAttribution(ctx, userID); err == nil {
			in.RoleID = attr.AcquisitionRoleID
			in.RoleType = attr.RoleType
			in.ParentRoleID = attr.ParentRoleID
			if in.ChannelOrgID == "" {
				in.ChannelOrgID = attr.ChannelOrgID
			}
		}
	}
	in.CanCommission = b.identity.RoleAllowsCommission(ctx, in.RoleID)
	if in.ChannelOrgID != "" {
		if market, err := b.identity.ResolveMarketChannelID(ctx, in.ChannelOrgID); err == nil {
			in.PolicyChannelID = market
		}
	}
	_, err := b.comm.Accrue(ctx, in)
	return err
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
