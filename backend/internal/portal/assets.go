package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"io"
	"net/http"
)

func (a *Application) uploadLogo(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "branding")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 3<<20)
	if e := c.Request.ParseMultipartForm(3 << 20); e != nil {
		handleError(c, branding.ErrInvalidInput)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, header, e := c.Request.FormFile("file")
	if e != nil {
		handleError(c, branding.ErrInvalidInput)
		return
	}
	defer file.Close()
	data, e := io.ReadAll(io.LimitReader(file, 2<<20+1))
	if e != nil || len(data) > 2<<20 {
		handleError(c, branding.ErrInvalidInput)
		return
	}
	scope, target := "brand", op.BrandID
	if op.Kind == organization.Channel {
		scope, target = "channel", op.ID
	}
	asset, e := a.Branding.UploadLogo(c.Request.Context(), branding.UploadLogoInput{ActorStaffID: actor, ScopeType: scope, ScopeID: target, Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data})
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": asset.ID, "status": asset.Status, "message": "Logo已存入私有对象存储。请在主题Logo字段使用此地址并保存版本：" + asset.URL, "asset": asset})
}
func (a *Application) publicLogo(c *gin.Context) {
	b := brand(c)
	operator := c.Query("operator_id")
	if operator == "" {
		operator = b.OwnerOperatorID
	}
	op, e := a.Organization.GetOperator(c.Request.Context(), nil, operator)
	if e != nil || op.BrandID != b.ID {
		handleError(c, branding.ErrNotFound)
		return
	}
	asset, data, e := a.Branding.PublicLogo(c.Request.Context(), b.ID, channelThemeID(op), c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(200, asset.ContentType, data)
}

func channelThemeID(op organization.Operator) string {
	if op.Kind == organization.Channel {
		return op.ID
	}
	return ""
}
