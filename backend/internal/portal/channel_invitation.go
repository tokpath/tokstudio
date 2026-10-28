package portal

import (
	"errors"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func (a *Application) channelCustomerInvitation(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "customers")
	if !ok {
		return
	}
	if op.Kind != organization.Channel {
		fail(c, 404, "not_found", "此入口仅用于渠道客户邀请", false)
		return
	}
	ctx := c.Request.Context()
	code, enabled, err := a.Organization.ChannelInvite(ctx, nil, actor, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	brand, err := a.Organization.GetBrand(ctx, nil, op.BrandID)
	if err != nil {
		handleError(c, err)
		return
	}
	link := ""
	if endpoint := a.brandAPIEndpoint(ctx, op.BrandID); endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil {
			handleError(c, err)
			return
		}
		u.Path, u.RawPath, u.RawQuery, u.Fragment = "/register", "", url.Values{"invite": {code}}.Encode(), ""
		link = u.String()
	}
	registrationOpen := false
	if enabled {
		err := a.Organization.RequireBusiness(ctx, nil, op.ID, organization.Registration)
		if err != nil && !errors.Is(err, organization.ErrBusinessClosed) && !errors.Is(err, organization.ErrFrozen) {
			handleError(c, err)
			return
		}
		registrationOpen = err == nil
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, gin.H{"invite_code": code, "invite_url": link, "brand_name": brand.Name, "enabled": enabled, "registration_open": registrationOpen})
}
