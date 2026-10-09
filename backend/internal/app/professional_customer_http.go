package app

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) createProfessionalCustomer(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		UserID   string `json:"user_id"`
		Type     string `json:"type"`
		ParentID string `json:"parent_id"`
	}
	if c.ShouldBindJSON(&body) != nil || body.UserID == "" {
		httpx.Abort(c, 400, "invalid_request", "请选择已注册的同渠道客户", false)
		return
	}
	var item *identity.AcquisitionRoleView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		item, err = a.Identity.CreateProfessionalCustomerTx(tx, *a.currentPrincipal(c), body.UserID, body.Type, body.ParentID)
		if err != nil {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "professional_customer.create", ResourceType: "acquisition_role", ResourceID: item.ID, After: map[string]any{"user_id": body.UserID, "role": item}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	httpx.Created(c, gin.H{"item": item})
}
func (a *App) professionalCustomerDetail(c *gin.Context) {
	item, err := a.Identity.ProfessionalCustomer(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if a.abortCustomerError(c, err) {
		return
	}
	p := a.currentPrincipal(c)
	owner, e := a.Identity.ResolvePaymentOwnerID(c.Request.Context(), item.Role.ChannelOrgID)
	manage := e == nil && ((p.IsPlatformAdmin() && owner == identity.OfficialChannelID) || (p.HasRole("channel_admin") && item.Role.ChannelOrgID == p.ChannelOrgID && owner == p.ChannelOrgID))
	httpx.OK(c, gin.H{"item": item, "manage": manage})
}
func (a *App) professionalCustomerChoices(c *gin.Context) {
	items, err := a.Identity.ProfessionalChoices(c.Request.Context(), *a.currentPrincipal(c), c.Query("channel_id"), c.Query("type"), c.Query("q"))
	if a.abortCustomerError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (a *App) searchProfessionalCustomers(c *gin.Context) {
	page, err := a.Identity.SearchProfessionalCustomers(c.Request.Context(), *a.currentPrincipal(c), a.customerQueryInput(c))
	if a.abortCustomerError(c, err) {
		return
	}
	httpx.OK(c, page)
}
