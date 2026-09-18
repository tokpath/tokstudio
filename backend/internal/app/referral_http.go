package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

type referralReward struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	AmountMinor int64      `json:"amount_minor"`
	AvailableAt *time.Time `json:"available_at,omitempty"`
}

func (a *App) createMeReferral(c *gin.Context) {
	if err := a.Identity.CreatePersonalReferral(c.Request.Context(), a.currentPrincipal(c).UserID); err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "生成推广链接失败，请重试", true)
		return
	}
	a.meReferral(c)
}

func (a *App) meReferral(c *gin.Context) {
	ctx := c.Request.Context()
	p := a.currentPrincipal(c)
	me, err := a.Identity.Me(ctx, *p)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	own, err := a.Identity.PersonalReferral(ctx, p.UserID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取推广信息失败，请重试", true)
		return
	}
	rule, err := a.Identity.EffectiveEligibility(ctx, me.ChannelOrgID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取奖励规则失败，请重试", true)
		return
	}
	rewards := []referralReward{}
	// ListEntries treats an empty role list as unrestricted: never call it without membership.
	if len(own.RoleIDs) > 0 {
		entries, err := a.Commission.ListEntries(ctx, "", own.RoleIDs, "")
		if err != nil {
			httpx.Abort(c, 500, "internal_error", "读取奖励记录失败，请重试", true)
			return
		}
		for _, e := range entries {
			rewards = append(rewards, referralReward{e.ID, e.Kind, e.Status, e.AmountMinor, e.AvailableAt})
		}
	}
	codes := []string{}
	for _, code := range own.Codes {
		codes = append(codes, code.Code)
	}
	httpx.OK(c, gin.H{"item": gin.H{
		"codes": codes, "can_create": len(own.RoleIDs) == 0,
		"invited_count": own.InvitedCount, "can_commission": me.CanCommission,
		"rules":   gin.H{"spend_minor": rule.SpendMinor, "topup_minor": rule.TopupMinor, "gift_minor": rule.GiftMinor},
		"rewards": rewards,
	}})
}
