package app

import (
	"context"

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
		if pool, err := b.identity.ResolvePoolChannelID(ctx, in.ChannelOrgID); err == nil && pool != "" {
			in.ChannelOrgID = pool
		}
	}
	_, err := b.comm.Accrue(ctx, in)
	return err
}

func (b *commissionBridge) ReverseUsage(ctx context.Context, usageEventID string) error {
	return b.comm.Reverse(ctx, usageEventID)
}
