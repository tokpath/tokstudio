package app

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/media"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerBrandWriteRoutes(r *gin.Engine) {
	r.GET("/v1/public/brand-assets/:id", a.publicBrandAsset)
	r.POST("/admin/brands", a.requireRoles("platform_admin"), a.createBrand)
	r.GET("/admin/brands/:id", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.getBrand)
	r.PATCH("/admin/brands/:id", a.requireRoles("platform_admin"), a.patchBrandAdmin)
	r.POST("/admin/brands/:id/assets", a.requireRoles("platform_admin"), a.uploadBrandAssetAdmin)
	r.GET("/channel/brand", a.requireRoles("channel_admin"), a.channelBrand)
	r.PATCH("/channel/brand", a.requireRoles("channel_admin"), a.patchChannelBrand)
	r.POST("/channel/brand/assets", a.requireRoles("channel_admin"), a.uploadChannelBrandAsset)
}

func (a *App) abortBrandErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, identity.ErrThemeKey), errors.Is(err, identity.ErrThemeHex):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", err.Error(), false)
	case errors.Is(err, identity.ErrThemeContrast):
		httpx.Abort(c, http.StatusBadRequest, "theme_contrast", err.Error(), false)
	case errors.Is(err, identity.ErrAssetKind):
		httpx.Abort(c, http.StatusBadRequest, "asset_kind", "资源种类不在白名单", false)
	case errors.Is(err, identity.ErrAssetType):
		httpx.Abort(c, http.StatusBadRequest, "asset_type", "文件类型不允许", false)
	case errors.Is(err, identity.ErrAssetTooLarge):
		httpx.Abort(c, http.StatusBadRequest, "asset_too_large", "文件超过该种类体积上限", false)
	case errors.Is(err, identity.ErrAssetDimension):
		httpx.Abort(c, http.StatusBadRequest, "asset_dimension", "像素或宽高比不在约定范围内", false)
	case errors.Is(err, identity.ErrAssetSVG):
		httpx.Abort(c, http.StatusBadRequest, "asset_svg", "SVG 不安全或缺少 viewBox", false)
	case errors.Is(err, identity.ErrAssetRateLimited):
		httpx.Abort(c, http.StatusTooManyRequests, "rate_limited", "该品牌一小时内上传次数过多", true)
	case errors.Is(err, identity.ErrBrandNotCustomizable):
		httpx.Abort(c, http.StatusForbidden, "brand_not_customizable", "该渠道不能自定义品牌", false)
	case errors.Is(err, identity.ErrBrandDomainTaken):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "域名已被其它品牌使用", false)
	case errors.Is(err, identity.ErrStoreUnavailable), errors.Is(err, media.ErrStoreUnavailable):
		httpx.Abort(c, http.StatusServiceUnavailable, "store_unavailable", "存储不可用", true)
	case errors.Is(err, identity.ErrChannelImmutable):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "没有权限", false)
	case errors.Is(err, identity.ErrPromotionInvalid):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "品牌名称或域名不完整", false)
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, identity.ErrNotFound):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
	default:
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "品牌操作失败", true)
	}
	return true
}

func (a *App) getBrand(c *gin.Context) {
	item, err := a.Identity.BrandByID(c.Request.Context(), c.Param("id"))
	if a.abortBrandErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "brand": item, "customizable": true, "storage": a.storageView(), "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createBrand(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var in identity.BrandInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	item, err := a.Identity.CreateBrand(c.Request.Context(), *a.currentPrincipal(c), in)
	if a.abortBrandErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "brand.create", ResourceType: "brand", ResourceID: item.ID,
		After: map[string]string{"name": item.Name, "primary_domain": item.PrimaryDomain},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchBrandAdmin(c *gin.Context) {
	in, ok := a.bindBrandPatch(c)
	if !ok {
		return
	}
	if domainChange(in) && !a.requireConfirm(c) {
		return
	}
	before, _ := a.Identity.BrandByID(c.Request.Context(), c.Param("id"))
	item, err := a.Identity.PatchBrand(c.Request.Context(), c.Param("id"), in)
	if a.abortBrandErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "brand.update", ResourceType: "brand", ResourceID: item.ID,
		Before: before, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelBrand(c *gin.Context) {
	brand, channel, err := a.Identity.ChannelBrand(c.Request.Context(), *a.currentPrincipal(c))
	if a.abortBrandErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{
		"brand": brand, "channel": channel, "customizable": channel != nil && channel.Type == "C",
		"storage": a.storageView(), "request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) patchChannelBrand(c *gin.Context) {
	brand, channel, err := a.Identity.ChannelBrand(c.Request.Context(), *a.currentPrincipal(c))
	if a.abortBrandErr(c, err) {
		return
	}
	if err := a.Identity.AssertBrandWritable(channel, *a.currentPrincipal(c), brand.ID); err != nil {
		_ = a.abortBrandErr(c, err)
		return
	}
	in, ok := a.bindBrandPatch(c)
	if !ok {
		return
	}
	in.PrimaryDomain, in.APIDomain, in.AdminDomain = "", "", ""
	item, err := a.Identity.PatchBrand(c.Request.Context(), brand.ID, in)
	if a.abortBrandErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "brand.update", ResourceType: "brand", ResourceID: item.ID,
		Before: brand, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) uploadBrandAssetAdmin(c *gin.Context) {
	a.uploadBrandAsset(c, c.Param("id"), true)
}

func (a *App) uploadChannelBrandAsset(c *gin.Context) {
	brand, channel, err := a.Identity.ChannelBrand(c.Request.Context(), *a.currentPrincipal(c))
	if a.abortBrandErr(c, err) {
		return
	}
	if err := a.Identity.AssertBrandWritable(channel, *a.currentPrincipal(c), brand.ID); err != nil {
		_ = a.abortBrandErr(c, err)
		return
	}
	a.uploadBrandAsset(c, brand.ID, false)
}

func (a *App) uploadBrandAsset(c *gin.Context, brandID string, _ bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, identity.AssetFormMax)
	if err := c.Request.ParseMultipartForm(identity.AssetFormMax); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "asset_too_large", "上传表单超过 600KiB", false)
		return
	}
	kind := strings.TrimSpace(c.PostForm("kind"))
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "缺少 file", false)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(identity.AssetFormMax)+1))
	if err != nil || len(data) > identity.AssetFormMax {
		httpx.Abort(c, http.StatusBadRequest, "asset_too_large", "上传表单超过 600KiB", false)
		return
	}
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	item, err := a.Identity.UploadBrandAsset(c.Request.Context(), *a.currentPrincipal(c), brandID, kind, filename, data)
	if a.abortBrandErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "brand.asset.upload", ResourceType: "brand_asset", ResourceID: item.ID,
		After: map[string]any{"kind": item.Kind, "size_bytes": item.SizeBytes, "width_px": item.WidthPx, "height_px": item.HeightPx, "brand_id": brandID},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "storage": a.storageView(), "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) publicBrandAsset(c *gin.Context) {
	row, body, err := a.Identity.PublicBrandAsset(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, identity.ErrStoreUnavailable) {
			httpx.Abort(c, http.StatusServiceUnavailable, "store_unavailable", "存储不可用", true)
			return
		}
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=86400, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	if row.ContentType == "image/svg+xml" {
		c.Header("Content-Security-Policy", "default-src 'none'; img-src 'self'")
	}
	c.Data(http.StatusOK, row.ContentType, body)
}

func (a *App) bindBrandPatch(c *gin.Context) (identity.BrandInput, bool) {
	var in identity.BrandInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return in, false
	}
	return in, true
}

func domainChange(in identity.BrandInput) bool {
	return strings.TrimSpace(in.PrimaryDomain) != "" || strings.TrimSpace(in.APIDomain) != "" || strings.TrimSpace(in.AdminDomain) != ""
}
