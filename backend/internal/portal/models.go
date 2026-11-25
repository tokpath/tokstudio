package portal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/usage"
)

func (a *Application) modelRoutes(r *gin.RouterGroup) {
	r.GET("/models", a.apiModels)
	r.GET("/responses", a.responsesWebSocket)
	for path, op := range map[string]modelruntime.Operation{"/chat/completions": modelruntime.Chat, "/responses": modelruntime.Responses, "/messages": modelruntime.Messages, "/embeddings": modelruntime.Embedding, "/images/generations": modelruntime.Image, "/images/edits": modelruntime.ImageEdit, "/audio/speech": modelruntime.Speech, "/audio/transcriptions": modelruntime.Transcription, "/videos": modelruntime.Video} {
		r.POST(path, a.executeModel(op, false))
	}
	r.POST("/images/generations/async", a.executeModel(modelruntime.Image, true))
	r.POST("/images/edits/async", a.executeModel(modelruntime.ImageEdit, true))
	a.imageBatchRoutes(r.Group("/images/batches"))
	r.POST("/videos/generations", a.executeModel(modelruntime.Video, false))
	r.GET("/images/tasks/:id", a.imageTaskStatus)
	r.GET("/videos/:id", a.videoStatus)
	r.GET("/videos/:id/content", a.videoContent)
	r.DELETE("/videos/:id", a.videoCancel)
}
func (a *Application) apiKey(c *gin.Context) (usage.Key, bool) {
	header := c.GetHeader("Authorization")
	if header == "" && c.GetBool(messagesModelAPI) && c.GetHeader("X-Api-Key") != "" {
		header = "Bearer " + c.GetHeader("X-Api-Key")
	}
	if !strings.HasPrefix(header, "Bearer ") {
		fail(c, 401, "invalid_api_key", "请使用当前品牌 API Key", false)
		return usage.Key{}, false
	}
	k, e := a.Usage.AuthenticateKey(c.Request.Context(), brand(c).ID, strings.TrimPrefix(header, "Bearer "))
	if e != nil {
		fail(c, 401, "invalid_api_key", "API Key 不可用或不属于当前品牌", false)
		return k, false
	}
	return k, true
}
func modelBody(c *gin.Context, op modelruntime.Operation) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		if e := c.Request.ParseMultipartForm(16 << 20); e != nil {
			return nil, e
		}
		defer c.Request.MultipartForm.RemoveAll()
		if op == modelruntime.ImageEdit {
			return multipartImageEdit(c)
		}
		obj := map[string]any{}
		for k, v := range c.Request.MultipartForm.Value {
			if len(v) != 1 {
				return nil, modelruntime.ErrInvalidRequest
			}
			obj[k] = v[0]
		}
		file, header, e := c.Request.FormFile("file")
		if e != nil {
			return nil, e
		}
		defer file.Close()
		raw, e := io.ReadAll(io.LimitReader(file, (16<<20)+1))
		if e != nil || len(raw) > 16<<20 {
			return nil, modelruntime.ErrInvalidRequest
		}
		obj["file"] = base64.StdEncoding.EncodeToString(raw)
		obj["filename"] = header.Filename
		return json.Marshal(obj)
	}
	return io.ReadAll(c.Request.Body)
}
func multipartImageEdit(c *gin.Context) ([]byte, error) {
	form := c.Request.MultipartForm
	body := make(map[string]any, len(form.Value)+2)
	for key, vals := range form.Value {
		if len(vals) != 1 {
			return nil, modelruntime.ErrInvalidRequest
		}
		switch key {
		case "n", "output_compression", "partial_images":
			value, err := strconv.Atoi(vals[0])
			if err != nil {
				return nil, modelruntime.ErrInvalidRequest
			}
			body[key] = value
		case "stream":
			if vals[0] != "true" && vals[0] != "false" {
				return nil, modelruntime.ErrInvalidRequest
			}
			body[key] = vals[0] == "true"
		default:
			body[key] = vals[0]
		}
	}
	images := make([]map[string][]byte, 0)
	if len(form.File["image"]) > 0 && len(form.File["image[]"]) > 0 {
		return nil, modelruntime.ErrInvalidRequest
	}
	for key, files := range form.File {
		if key != "image" && key != "image[]" && key != "mask" || key == "mask" && len(files) != 1 {
			return nil, modelruntime.ErrInvalidRequest
		}
		for _, header := range files {
			f, err := header.Open()
			if err != nil {
				return nil, err
			}
			raw, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
			_ = f.Close()
			if err != nil || len(raw) == 0 || len(raw) > 16<<20 {
				return nil, modelruntime.ErrInvalidRequest
			}
			if key == "mask" {
				if body["mask"] != nil {
					return nil, modelruntime.ErrInvalidRequest
				}
				body["mask"] = raw
			} else {
				images = append(images, map[string][]byte{"image": raw})
			}
		}
	}
	if len(images) > 0 {
		if body["image"] != nil || body["images"] != nil {
			return nil, modelruntime.ErrInvalidRequest
		}
		body["images"] = images
	}
	return json.Marshal(body)
}

func (a *Application) executeModel(op modelruntime.Operation, async bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if op == modelruntime.Messages {
			c.Set(messagesModelAPI, true)
		}
		k, ok := a.apiKey(c)
		if !ok {
			return
		}
		businessKey, ok := modelBusinessKey(c)
		if !ok {
			return
		}
		raw, e := modelBody(c, op)
		if e != nil {
			handleError(c, modelruntime.ErrInvalidRequest)
			return
		}
		input, e := modelruntime.DecodeProtocol(uuid.NewString(), op, raw)
		if e != nil {
			handleError(c, e)
			return
		}
		if async {
			request, err := a.Usage.SubmitImage(c.Request.Context(), k, businessKey, input)
			if err != nil {
				handleError(c, err)
				return
			}
			path := "/v1/images/tasks/" + request.ID
			c.Header("Location", path)
			c.Header("Retry-After", "3")
			c.Header("X-TokenHub-Request-ID", request.ID)
			c.JSON(http.StatusAccepted, imageTaskView(request))
			return
		}
		streamed := false
		var emit modelruntime.Emit
		if input.Stream {
			emit = func(ctx context.Context, event modelruntime.Event) error {
				if !streamed {
					c.Header("Content-Type", "text/event-stream")
					c.Header("Cache-Control", "no-cache, no-transform")
					c.Header("X-Accel-Buffering", "no")
					c.Header("X-TokenHub-Request-ID", event.RequestID)
					c.Status(200)
					streamed = true
				}
				if c.Request.Context().Err() != nil {
					return c.Request.Context().Err()
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if e := writeModelEvent(c, event.Type, event.Data); e != nil {
					return e
				}
				c.Writer.Flush()
				return nil
			}
		}
		out, e := a.Usage.Execute(c.Request.Context(), k, businessKey, input, emit)
		if streamed {
			if e != nil {
				message := "请求中断或用量待核实，请查询原请求"
				if op == modelruntime.Messages {
					b, _ := json.Marshal(messagesError(http.StatusBadGateway, message, out.Request.ID))
					_ = writeModelEvent(c, "error", b)
				} else {
					b, _ := json.Marshal(gin.H{"error": gin.H{"code": "request_interrupted", "message": message, "request_id": out.Request.ID}})
					_ = writeModelEvent(c, "", b)
				}
			}
			if op != modelruntime.Messages {
				_ = writeModelEvent(c, "", []byte("[DONE]"))
			}
			c.Writer.Flush()
			return
		}
		if e != nil {
			handleError(c, e)
			return
		}
		c.Header("X-TokenHub-Request-ID", out.Request.ID)
		if len(out.Result.Binary) > 0 && (op != modelruntime.Video || len(out.Result.Output) == 0) {
			c.Data(200, out.Result.ContentType, out.Result.Binary)
			return
		}
		if len(out.Result.Output) > 0 {
			if op == modelruntime.Video {
				var video map[string]any
				if json.Unmarshal(out.Result.Output, &video) == nil {
					video["id"] = out.Request.ID
					video["status"] = videoProtocolStatus(out.Request.Status)
					c.JSON(200, video)
					return
				}
			}
			c.Data(200, "application/json", out.Result.Output)
			return
		}
		c.JSON(202, gin.H{"id": out.Request.ID, "object": "video", "status": videoProtocolStatus(out.Request.Status), "model": out.Request.ModelID, "created_at": out.Request.AcceptedAt.Unix(), "delivered_at": out.Request.DeliveredAt})
	}
}

func modelBusinessKey(c *gin.Context) (string, bool) {
	value := c.GetHeader("Idempotency-Key")
	if value == "" {
		return uuid.NewString(), true
	}
	if len(value) > 150 {
		fail(c, 400, "invalid_idempotency_key", "请求编号过长", false)
		return "", false
	}
	return value, true
}

func writeModelEvent(c *gin.Context, kind string, data []byte) error {
	if kind != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", kind); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	return err
}
func (a *Application) modelRequestForKey(c *gin.Context) (usage.Request, bool) {
	key, ok := a.apiKey(c)
	if !ok {
		return usage.Request{}, false
	}
	r, e := a.Usage.RequestForKey(c.Request.Context(), key, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return r, false
	}
	return r, true
}

func imageTaskView(r usage.Request) gin.H {
	return gin.H{"id": r.ID, "object": "image.task", "status": videoProtocolStatus(r.Status),
		"model": r.ModelID, "created_at": r.AcceptedAt.Unix(), "delivered_at": r.DeliveredAt}
}

func (a *Application) imageTaskStatus(c *gin.Context) {
	r, ok := a.modelRequestForKey(c)
	if !ok {
		return
	}
	if !r.Operation.IsImage() || !r.AsyncImage && !r.Streaming {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	view := imageTaskView(r)
	if r.Status == "completed" {
		ctx := c.Request.Context()
		meta, err := a.Usage.ResultMetadata(ctx, r.CustomerID, r.ID)
		if err != nil {
			handleError(c, err)
			return
		}
		if meta == nil {
			if r.CompletedAt != nil && r.CompletedAt.Add(a.Usage.ResultRetention).After(time.Now()) {
				fail(c, http.StatusServiceUnavailable, "result_unavailable", "结果暂不可用，请使用原请求编号重试", true)
			} else {
				handleError(c, usage.ErrResultUnavailable)
			}
			return
		}
		result, err := a.Usage.ReadResult(ctx, r.CustomerID, r.ID)
		if err != nil || len(result.Output) == 0 {
			if errors.Is(err, usage.ErrResultUnavailable) && !meta.ExpiresAt.After(time.Now()) {
				handleError(c, err)
			} else {
				fail(c, http.StatusServiceUnavailable, "result_unavailable", "结果暂不可用，请使用原请求编号重试", true)
			}
			return
		}
		view["result"], view["expires_at"] = result.Output, meta.ExpiresAt.Unix()
	}
	if r.Reason != "" && r.Status != "completed" {
		view["reason"] = r.Reason
	}
	c.JSON(http.StatusOK, view)
}

func (a *Application) videoStatus(c *gin.Context) {
	r, ok := a.modelRequestForKey(c)
	if !ok {
		return
	}
	if r.Operation != modelruntime.Video {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	updated, e := a.Usage.RefreshAsync(c.Request.Context(), r.ID)
	if e == nil {
		r = updated
	}
	c.JSON(200, gin.H{"id": r.ID, "object": "video", "status": videoProtocolStatus(r.Status), "model": r.ModelID, "created_at": r.AcceptedAt.Unix(), "delivered_at": r.DeliveredAt, "reason": r.Reason})
}
func (a *Application) videoContent(c *gin.Context) {
	r, ok := a.modelRequestForKey(c)
	if !ok {
		return
	}
	result, e := a.Usage.DownloadAsync(c.Request.Context(), r.CustomerID, r.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	c.Data(200, result.ContentType, result.Binary)
}
func (a *Application) videoCancel(c *gin.Context) {
	r, ok := a.modelRequestForKey(c)
	if !ok {
		return
	}
	r, e := a.Usage.RequestCancellation(c.Request.Context(), r.CustomerID, r.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": r.ID, "status": r.Status, "cancel_requested": r.CancelRequested})
}
func (a *Application) requestDTO(ctx context.Context, r usage.Request) (gin.H, error) {
	held := ledger.Zero()
	switch r.Status {
	case "accepted", "executing", "submitted", "awaiting_usage":
		held = r.EstimateATG
	}
	name := "网页体验"
	if key, e := a.Usage.GetKey(ctx, r.CustomerID, r.KeyID); e == nil {
		name = key.Name
	}
	out := gin.H{"id": r.ID, "model_id": r.ModelID, "key_name": name, "status": r.Status, "actual_atg": r.ActualATG, "charged_atg": r.ChargedATG, "corrected_atg": r.CorrectedATG, "corrected_charged_atg": r.CorrectedChargedATG, "net_actual_atg": r.ActualATG.Sub(r.CorrectedATG), "net_charged_atg": r.ChargedATG.Sub(r.CorrectedChargedATG), "held_atg": held, "created_at": r.AcceptedAt, "reason": r.Reason}
	if r.DeliveredAt != nil {
		out["delivered_at"] = r.DeliveredAt
	}
	meta, e := a.Usage.ResultMetadata(ctx, r.CustomerID, r.ID)
	if e != nil {
		return nil, e
	}
	if meta != nil {
		out["result_url"] = "/api/v1/me/requests/" + r.ID + "/result"
		out["result_expires_at"] = meta.ExpiresAt
	}
	if e = a.billingDetails(ctx, r, out); e != nil {
		return nil, e
	}
	return out, nil
}
func (a *Application) requests(c *gin.Context) {
	rows, e := a.Usage.ListRequestsPage(c.Request.Context(), subject(c).Customer.Customer.ID, c.Query("cursor"), c.Query("q"), 51)
	if e != nil {
		handleError(c, e)
		return
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = usage.RequestCursor(rows[49])
	}
	items := []any{}
	for _, r := range rows {
		dto, e := a.requestDTO(c.Request.Context(), r)
		if e != nil {
			handleError(c, e)
			return
		}
		items = append(items, dto)
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}
func (a *Application) requestDetail(c *gin.Context) {
	r, e := a.Usage.GetRequest(c.Request.Context(), subject(c).Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.requestDTO(c.Request.Context(), r)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, dto)
}
func (a *Application) requestResult(c *gin.Context) {
	result, e := a.Usage.ReadResult(c.Request.Context(), subject(c).Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	if len(result.Binary) > 0 {
		c.Header("Content-Disposition", "attachment")
		c.Data(200, result.ContentType, result.Binary)
		return
	}
	c.Data(200, "application/json", result.Output)
}
func (a *Application) cancelRequest(c *gin.Context) {
	r, e := a.Usage.RequestCancellation(c.Request.Context(), subject(c).Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(r.ID, "cancellation_requested"))
}
func (a *Application) playground(c *gin.Context) {
	raw, e := modelBody(c, modelruntime.Transcription)
	if e != nil {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	var in struct{ Model, Capability, Input, Voice, ResponseFormat, Seconds, File, Filename string }
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	decode := func(k string, v *string) { _ = json.Unmarshal(fields[k], v) }
	decode("model", &in.Model)
	decode("capability", &in.Capability)
	decode("input", &in.Input)
	decode("voice", &in.Voice)
	decode("response_format", &in.ResponseFormat)
	decode("seconds", &in.Seconds)
	decode("file", &in.File)
	decode("filename", &in.Filename)
	op := modelruntime.Operation(in.Capability)
	if op == "image" {
		op = modelruntime.Image
	}
	if op == "video" {
		op = modelruntime.Video
	}
	ctx := c.Request.Context()
	operator, err := a.Organization.GetOperator(ctx, nil, subject(c).Customer.Ownership.OperatorID)
	if err != nil {
		handleError(c, err)
		return
	}
	models, err := a.callableCatalog(ctx, operator)
	if err != nil {
		handleError(c, err)
		return
	}
	found := false
	for _, model := range models {
		if model.ID != in.Model || model.Status != "available" {
			continue
		}
		for _, cap := range model.Capabilities {
			if cap.Operation == op {
				found = true
			}
		}
	}
	if !found {
		handleError(c, modelruntime.ErrUnsupported)
		return
	}
	obj := map[string]any{"model": in.Model}
	switch op {
	case modelruntime.Chat:
		obj["messages"] = []any{gin.H{"role": "user", "content": in.Input}}
	case modelruntime.Responses:
		obj["input"] = in.Input
	case modelruntime.Embedding:
		obj["input"] = in.Input
	case modelruntime.Image:
		obj["prompt"] = in.Input
	case modelruntime.ImageEdit:
		obj["prompt"] = in.Input
		obj["image"] = gin.H{"image": in.File}
	case modelruntime.Speech:
		obj["input"] = in.Input
		if in.Voice != "" {
			obj["voice"] = in.Voice
		}
		if in.ResponseFormat != "" {
			obj["response_format"] = in.ResponseFormat
		}
	case modelruntime.Transcription:
		obj["file"] = in.File
		obj["filename"] = in.Filename
	case modelruntime.Video:
		obj["prompt"] = in.Input
		if in.Seconds != "" {
			obj["seconds"] = in.Seconds
		}
	default:
		handleError(c, modelruntime.ErrUnsupported)
		return
	}
	if value, ok := fields["parameters"]; ok {
		var parameters map[string]json.RawMessage
		var encoded string
		if json.Unmarshal(value, &encoded) == nil {
			value = json.RawMessage(encoded)
		}
		if json.Unmarshal(value, &parameters) != nil {
			handleError(c, modelruntime.ErrInvalidRequest)
			return
		}
		for name, value := range parameters {
			if contains([]string{"model", "messages", "input", "prompt", "file", "filename", "image", "images", "stream"}, name) {
				handleError(c, modelruntime.ErrUnsupported)
				return
			}
			var decoded any
			decoder := json.NewDecoder(strings.NewReader(string(value)))
			decoder.UseNumber()
			if decoder.Decode(&decoded) != nil {
				handleError(c, modelruntime.ErrInvalidRequest)
				return
			}
			obj[name] = decoded
		}
	}
	encoded, _ := json.Marshal(obj)
	input, e := modelruntime.DecodeProtocol(uuid.NewString(), op, encoded)
	if e != nil {
		handleError(c, e)
		return
	}
	k, e := a.Usage.PlaygroundKey(c.Request.Context(), subject(c).Customer.Customer.ID, "UTC")
	if e != nil {
		handleError(c, e)
		return
	}
	businessKey, _ := key(c)
	out, e := a.Usage.Execute(c.Request.Context(), k, businessKey, input, nil)
	if e != nil {
		handleError(c, e)
		return
	}
	result := gin.H{"request_id": out.Request.ID, "status": out.Request.Status, "charged_atg": out.Request.ChargedATG, "reason": out.Request.Reason}
	playgroundOutput(result, op, out.Result.Output)
	if strings.HasPrefix(out.Result.ContentType, "audio/") {
		result["media_kind"] = "audio"
	} else if op == modelruntime.Video {
		result["media_kind"] = "video"
	}
	meta, e := a.Usage.ResultMetadata(c.Request.Context(), k.CustomerID, out.Request.ID)
	if e != nil && !errors.Is(e, context.Canceled) {
		handleError(c, e)
		return
	}
	if meta != nil {
		result["result_url"] = "/api/v1/me/requests/" + out.Request.ID + "/result"
		result["result_expires_at"] = meta.ExpiresAt
	}
	c.JSON(200, result)
}

func videoProtocolStatus(status string) string {
	switch status {
	case "accepted":
		return "queued"
	case "executing", "submitted", "awaiting_usage":
		return "in_progress"
	case "free":
		return "failed"
	}
	return status
}

// apiModels exposes only this verified key's current authorized public models.
func (a *Application) apiModels(c *gin.Context) {
	a.writeAPIModels(c, false)
}

func (a *Application) writeAPIModels(c *gin.Context, imagesOnly bool) {
	k, ok := a.apiKey(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	ownership, e := a.Organization.CurrentOwnership(ctx, nil, k.CustomerID)
	if e != nil {
		handleError(c, e)
		return
	}
	op, e := a.Organization.GetOperator(ctx, nil, ownership.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	rows, e := a.callableCatalog(ctx, op)
	if e != nil {
		handleError(c, e)
		return
	}
	var allowed []string
	if json.Unmarshal(k.ModelIDs, &allowed) != nil {
		handleError(c, usage.ErrKey)
		return
	}
	data := []gin.H{}
	for _, m := range rows {
		if m.Status != "available" {
			continue
		}
		if imagesOnly {
			var images bool
			for _, capability := range m.Capabilities {
				images = images || capability.Operation.IsImage()
			}
			if !images {
				continue
			}
		}
		permitted := allowed == nil
		for _, id := range allowed {
			if id == m.ID {
				permitted = true
			}
		}
		if permitted {
			data = append(data, gin.H{"id": m.ID, "object": "model", "owned_by": brand(c).Name})
		}
	}
	c.JSON(200, gin.H{"object": "list", "data": data})
}
