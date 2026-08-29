package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/media"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerMediaRoutes(r *gin.Engine) {
	r.GET("/v1/me/media", a.requireAnyUser(), a.listMyMedia)
	r.GET("/admin/media", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.listAdminMedia)
	r.POST("/v1/videos", a.requireAPIKey(), a.createVideo)
	r.GET("/v1/videos/:id", a.requireAPIKey(), a.getVideo)
	r.GET("/v1/videos/:id/content", a.requireAPIKey(), a.videoContent)
	r.POST("/v1/videos/:id/cancel", a.requireAPIKey(), a.cancelVideo)
	r.POST("/v1/images/generations", a.requireAPIKey(), a.createImage)
	r.POST("/v1/images/edits", a.requireAPIKey(), a.editImage)
	r.GET("/v1/images/:id", a.requireAPIKey(), a.getVideo)
	r.GET("/v1/images/:id/content", a.requireAPIKey(), a.videoContent)
	r.POST("/v1/media/callbacks", a.mediaCallback)
	r.GET("/v1/media/objects", a.mediaObject)
}

func (a *App) listMyMedia(c *gin.Context) {
	items, err := a.Media.List(c.Request.Context(), a.currentPrincipal(c).UserID, c.Query("kind"), c.Query("status"), 200)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取媒体任务失败", true)
		return
	}
	httpx.OKPage(c, items, 20, func(item media.JobView) string { return item.ID })
}

func (a *App) listAdminMedia(c *gin.Context) {
	items, err := a.Media.List(c.Request.Context(), c.Query("user_id"), c.Query("kind"), c.Query("status"), 200)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取媒体任务失败", true)
		return
	}
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "media.csv", []string{"id", "kind", "status", "model"}, items, func(item media.JobView) []string {
			return []string{item.ID, item.Kind, item.Status, item.Model}
		})
		return
	}
	httpx.OKPage(c, items, 50, func(item media.JobView) string { return item.ID })
}

func (a *App) createVideo(c *gin.Context) {
	a.createMedia(c, media.KindVideo)
}

func (a *App) createImage(c *gin.Context) {
	a.createMedia(c, media.KindImage)
}

func (a *App) editImage(c *gin.Context) {
	a.createMedia(c, media.KindImage)
}

func (a *App) createMedia(c *gin.Context, kind string) {
	var body struct {
		Model       string   `json:"model"`
		Prompt      string   `json:"prompt"`
		Duration    int      `json:"duration"`
		Resolution  string   `json:"resolution"`
		AspectRatio string   `json:"aspect_ratio"`
		FPS         int      `json:"fps"`
		Audio       bool     `json:"generate_audio"`
		CallbackURL string   `json:"callback_url"`
		Images      []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	if body.Prompt == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 prompt", false)
		return
	}
	if body.Model == "" {
		if kind == media.KindImage {
			body.Model = catalog.ImageModelID
		} else {
			body.Model = catalog.SeedanceModelID
		}
	}
	job, err := a.Media.Create(c.Request.Context(), media.CreateInput{
		Caller:         *a.currentAPIKey(c),
		RequestID:      c.GetString(httpx.ContextRequestID),
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		Kind:           kind,
		Model:          body.Model,
		Prompt:         body.Prompt,
		Duration:       body.Duration,
		Resolution:     body.Resolution,
		AspectRatio:    body.AspectRatio,
		FPS:            body.FPS,
		Audio:          body.Audio,
		CallbackURL:    body.CallbackURL,
		Images:         body.Images,
		ForceFail:      c.GetHeader("X-Tokenhub-Force-Fail"),
	})
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrInsufficientBalance):
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_balance", "余额不足", false)
		default:
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建媒体任务失败", false)
		}
		return
	}
	httpx.Accepted(c, job)
}

func (a *App) getVideo(c *gin.Context) {
	job, err := a.Media.Get(c.Request.Context(), c.Param("id"), a.currentAPIKey(c).UserID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "任务不存在", false)
		return
	}
	httpx.OK(c, job)
}

func (a *App) videoContent(c *gin.Context) {
	item, err := a.Media.Content(c.Request.Context(), c.Param("id"), a.currentAPIKey(c).UserID)
	if err != nil {
		if errors.Is(err, media.ErrNotReady) {
			httpx.Abort(c, http.StatusConflict, "media_job_not_ready", "结果未就绪或已过期", true)
			return
		}
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "任务不存在", false)
		return
	}
	httpx.OK(c, item)
}

func (a *App) cancelVideo(c *gin.Context) {
	job, err := a.Media.Cancel(c.Request.Context(), c.Param("id"), a.currentAPIKey(c).UserID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "任务不存在", false)
		return
	}
	httpx.OK(c, job)
}

func (a *App) mediaCallback(c *gin.Context) {
	body, _ := io.ReadAll(c.Request.Body)
	var payload struct {
		EventID string         `json:"event_id"`
		JobID   string         `json:"job_id"`
		Usage   map[string]int `json:"usage"`
	}
	_ = json.Unmarshal(body, &payload)
	if payload.EventID == "" || payload.JobID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 event_id 和 job_id", false)
		return
	}
	if err := a.Media.HandleCallback(c.Request.Context(), payload.EventID, payload.JobID, c.GetHeader("X-Tokenhub-Signature"), body, payload.Usage); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "回调无效", false)
		return
	}
	httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) mediaObject(c *gin.Context) {
	key := c.Query("key")
	sig := c.Query("sig")
	exp, _ := strconv.ParseInt(c.Query("exp"), 10, 64)
	data, err := a.Media.Object(key, sig, exp)
	if err != nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "签名无效或已过期", false)
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", data)
}
