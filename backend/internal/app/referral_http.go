package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

type referralReward struct {
	RequestID   string     `json:"request_id,omitempty"`
	ReversalOf  string     `json:"reversal_of,omitempty"`
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
	page, _ := strconv.Atoi(c.Query("page"))
	facts, err := a.Commission.PersonalPage(ctx, own.RoleIDs, page)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取收益记录失败，请重试", true)
		return
	}
	progress, err := a.Billing.ReferralProgress(ctx, p.UserID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取资格进度失败，请重试", true)
		return
	}
	rewards := []referralReward{}
	for _, e := range facts.Entries {
		rewards = append(rewards, referralReward{ID: e.ID, Kind: e.Kind, Status: e.Status, AmountMinor: e.AmountMinor, AvailableAt: e.AvailableAt, RequestID: e.RequestID, ReversalOf: e.ReversalOf})
	}
	professional := false
	if member, err := a.Identity.MemberRole(ctx, p.UserID); err == nil {
		professional = member.Status == "active" && (member.Type == identity.AcqAgent || member.Type == identity.AcqKOL1 || member.Type == identity.AcqKOL2)
	}
	codes := []string{}
	for _, code := range own.Codes {
		codes = append(codes, code.Code)
	}
	httpx.OK(c, gin.H{"item": gin.H{
		"codes": codes, "can_create": len(own.RoleIDs) == 0,
		"invited_count": own.InvitedCount, "can_commission": me.CanCommission,
		"rules":   gin.H{"spend_minor": rule.SpendMinor, "topup_minor": rule.TopupMinor, "gift_minor": rule.GiftMinor},
		"rewards": rewards, "settlements": facts.Settlements, "summary": facts.Summary,
		"progress": progress, "professional_customers": professional,
		"pagination": gin.H{"page": facts.Page, "page_size": facts.PageSize, "rewards_total": facts.EntriesTotal, "settlements_total": facts.SettlementsTotal},
	}})
}
