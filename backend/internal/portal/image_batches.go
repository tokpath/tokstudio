package portal

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

func (a *Application) imageBatchRoutes(r *gin.RouterGroup) {
	r.POST("", a.submitImageBatch)
	r.GET("", a.listImageBatches)
	r.GET("/models", func(c *gin.Context) { a.writeAPIModels(c, true) })
	r.GET("/:id", a.getImageBatch)
	r.GET("/:id/items", a.listImageBatchItems)
	r.GET("/:id/items/:custom_id/content", a.imageBatchItemContent)
	r.GET("/:id/download", a.downloadImageBatch)
	r.POST("/:id/cancel", a.cancelImageBatch)
}

type imageBatchReference struct {
	Data     []byte `json:"data,omitempty"`
	FileURI  string `json:"file_uri,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type imageBatchSubmission struct {
	Model    string `json:"model"`
	TaskName string `json:"task_name,omitempty"`
	Size     string `json:"size,omitempty"`
	Quality  string `json:"quality,omitempty"`
	Items    []struct {
		CustomID        string                `json:"custom_id"`
		Prompt          string                `json:"prompt"`
		OutputCount     int                   `json:"output_count,omitempty"`
		ReferenceImages []imageBatchReference `json:"reference_images,omitempty"`
	} `json:"items"`
}

func (a *Application) submitImageBatch(c *gin.Context) {
	key, ok := a.apiKey(c)
	if !ok {
		return
	}
	idempotency, ok := modelBusinessKey(c)
	if !ok {
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20))
	decoder.DisallowUnknownFields()
	var in imageBatchSubmission
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF ||
		len(in.Items) == 0 || len(in.Items) > usage.MaxImageBatchItems {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	input := usage.ImageBatchInput{ModelID: in.Model, TaskName: in.TaskName}
	for _, item := range in.Items {
		if strings.TrimSpace(item.Prompt) == "" {
			handleError(c, modelruntime.ErrInvalidRequest)
			return
		}
		body := map[string]any{"model": in.Model, "prompt": item.Prompt}
		if in.Size != "" {
			body["size"] = in.Size
		}
		if in.Quality != "" {
			body["quality"] = in.Quality
		}
		if item.OutputCount != 0 {
			body["n"] = item.OutputCount
		}
		op := modelruntime.Image
		if len(item.ReferenceImages) != 0 {
			op = modelruntime.ImageEdit
			images := make([]map[string]any, 0, len(item.ReferenceImages))
			for _, image := range item.ReferenceImages {
				if (image.FileURI != "") == (len(image.Data) != 0) {
					handleError(c, modelruntime.ErrInvalidRequest)
					return
				}
				if image.FileURI != "" {
					images = append(images, map[string]any{"url": image.FileURI})
				} else {
					mime := http.DetectContentType(image.Data)
					if !strings.HasPrefix(mime, "image/") || image.MimeType != "" && image.MimeType != mime {
						handleError(c, modelruntime.ErrInvalidRequest)
						return
					}
					images = append(images, map[string]any{"image": image.Data})
				}
			}
			body["images"] = images
		}
		raw, _ := json.Marshal(body)
		request, err := modelruntime.DecodeProtocol("", op, raw)
		if err != nil {
			handleError(c, err)
			return
		}
		input.Items = append(input.Items, usage.ImageBatchEntry{CustomID: item.CustomID, Request: request})
	}
	batch, err := a.Usage.SubmitImageBatch(c.Request.Context(), key, idempotency, input)
	if err != nil {
		handleError(c, err)
		return
	}
	c.Header("Location", "/v1/images/batches/"+batch.ID)
	c.Header("Retry-After", "3")
	c.JSON(http.StatusAccepted, imageBatchView(batch))
}

func imageBatchView(batch usage.ImageBatch) gin.H {
	total := len(batch.Items)
	succeeded, failed, cancelled, running := 0, 0, 0, 0
	estimate, charged := ledger.Zero(), ledger.Zero()
	for _, item := range batch.Items {
		estimate, charged = estimate.Add(item.EstimateATG), charged.Add(item.ChargedATG)
		switch item.Status {
		case "completed":
			succeeded++
		case "failed", "free":
			failed++
		case "cancelled":
			cancelled++
		case "accepted":
			if item.CancelRequested {
				running++
			}
		default:
			running++
		}
	}
	ended := succeeded + failed + cancelled
	status := "queued"
	switch {
	case ended == total && succeeded > 0 && succeeded < total:
		status = "partial_failed"
	case ended == total && succeeded > 0:
		status = "completed"
	case ended == total && cancelled > 0 && failed == 0:
		status = "cancelled"
	case ended == total:
		status = "failed"
	case ended+running > 0:
		status = "in_progress"
	}
	return gin.H{
		"id": batch.ID, "object": "image.batch", "status": status, "task_name": batch.TaskName, "model": batch.ModelID,
		"created_at": batch.CreatedAt.Unix(), "delivered_at": batch.CreatedAt, "item_count": total, "success_count": succeeded,
		"fail_count": failed, "cancelled_count": cancelled, "estimate_atg": estimate.String(), "charged_atg": charged.String(),
	}
}

func (a *Application) imageBatchForKey(c *gin.Context) (usage.ImageBatch, bool) {
	key, ok := a.apiKey(c)
	if !ok {
		return usage.ImageBatch{}, false
	}
	batch, err := a.Usage.ReadImageBatch(c.Request.Context(), key, c.Param("id"))
	if err != nil {
		handleError(c, err)
		return usage.ImageBatch{}, false
	}
	return batch, true
}

func (a *Application) getImageBatch(c *gin.Context) {
	if batch, ok := a.imageBatchForKey(c); ok {
		c.JSON(http.StatusOK, imageBatchView(batch))
	}
}

func (a *Application) listImageBatches(c *gin.Context) {
	key, ok := a.apiKey(c)
	if !ok {
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	batches, next, err := a.Usage.ListImageBatches(c.Request.Context(), key, c.Query("cursor"), limit)
	if err != nil {
		handleError(c, err)
		return
	}
	data := make([]gin.H, 0, len(batches))
	for _, batch := range batches {
		data = append(data, imageBatchView(batch))
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data, "has_more": next != "", "next_cursor": next})
}

func (a *Application) listImageBatchItems(c *gin.Context) {
	batch, ok := a.imageBatchForKey(c)
	if !ok {
		return
	}
	data := make([]gin.H, 0, len(batch.Items))
	for _, item := range batch.Items {
		data = append(data, gin.H{"custom_id": item.CustomID, "request_id": item.RequestID,
			"status": videoProtocolStatus(item.Status), "cancel_requested": item.CancelRequested,
			"reason": item.Reason, "charged_atg": item.ChargedATG.String()})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data, "has_more": false})
}

func (a *Application) cancelImageBatch(c *gin.Context) {
	key, ok := a.apiKey(c)
	if !ok {
		return
	}
	batch, err := a.Usage.CancelImageBatch(c.Request.Context(), key, c.Param("id"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, imageBatchView(batch))
}

type batchImageFile struct {
	Data      []byte
	MediaType string
	Extension string
}

func (a *Application) batchImageFiles(ctx context.Context, customer string, item usage.ImageBatchItem) ([]batchImageFile, error) {
	switch item.Status {
	case "failed", "free", "cancelled":
		return nil, gorm.ErrRecordNotFound
	case "completed":
	default:
		return nil, usage.ErrInProgress
	}
	result, err := a.Usage.ReadResult(ctx, customer, item.RequestID)
	if errors.Is(err, usage.ErrResultUnavailable) && item.CompletedAt != nil &&
		item.CompletedAt.Add(a.Usage.ResultRetention).After(time.Now()) {
		return nil, objectstore.ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	var data struct {
		Images []struct {
			Bytes []byte `json:"b64_json"`
		} `json:"data"`
	}
	if json.Unmarshal(result.Output, &data) != nil || len(data.Images) == 0 {
		return nil, objectstore.ErrUnavailable
	}
	files := make([]batchImageFile, 0, len(data.Images))
	for _, img := range data.Images {
		mime := http.DetectContentType(img.Bytes)
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}[mime]
		if ext == "" {
			return nil, objectstore.ErrUnavailable
		}
		files = append(files, batchImageFile{Data: img.Bytes, MediaType: mime, Extension: ext})
	}
	return files, nil
}

func (a *Application) imageBatchItemContent(c *gin.Context) {
	batch, ok := a.imageBatchForKey(c)
	if !ok {
		return
	}
	index, err := strconv.Atoi(c.DefaultQuery("image_index", "0"))
	if err != nil || index < 0 {
		handleError(c, modelruntime.ErrInvalidRequest)
		return
	}
	for _, item := range batch.Items {
		if item.CustomID != c.Param("custom_id") {
			continue
		}
		files, err := a.batchImageFiles(c.Request.Context(), batch.CustomerID, item)
		if err != nil {
			handleError(c, err)
			return
		}
		if index >= len(files) {
			handleError(c, modelruntime.ErrInvalidRequest)
			return
		}
		file := files[index]
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%d.%s"`, item.CustomID, index+1, file.Extension))
		c.Header("X-TokenHub-Request-ID", item.RequestID)
		c.Data(http.StatusOK, file.MediaType, file.Data)
		return
	}
	handleError(c, gorm.ErrRecordNotFound)
}

func (a *Application) downloadImageBatch(c *gin.Context) {
	batch, ok := a.imageBatchForKey(c)
	if !ok {
		return
	}
	var archive *zip.Writer
	var unavailable error
	pending := false
	manifest := make([]gin.H, 0, len(batch.Items))
	for _, item := range batch.Items {
		entry := gin.H{"custom_id": item.CustomID, "request_id": item.RequestID, "status": videoProtocolStatus(item.Status)}
		manifest = append(manifest, entry)
		if item.Status != "completed" {
			switch item.Status {
			case "accepted", "executing", "submitted", "awaiting_usage":
				pending = true
			}
			continue
		}
		files, err := a.batchImageFiles(c.Request.Context(), batch.CustomerID, item)
		if err != nil {
			entry["result"] = "unavailable"
			if unavailable == nil || errors.Is(err, objectstore.ErrUnavailable) {
				unavailable = err
			}
			continue
		}
		if archive == nil {
			c.Header("Content-Type", "application/zip")
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, batch.ID))
			archive = zip.NewWriter(c.Writer)
			defer archive.Close()
		}
		names := make([]string, 0, len(files))
		for i, file := range files {
			name := fmt.Sprintf("%02d-%s/%d.%s", item.Position+1, item.CustomID, i+1, file.Extension)
			writer, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
			if err != nil {
				return
			}
			if _, err = writer.Write(file.Data); err != nil {
				return
			}
			names = append(names, name)
		}
		entry["files"] = names
	}
	if archive == nil {
		if unavailable == nil {
			unavailable = gorm.ErrRecordNotFound
			if pending {
				unavailable = usage.ErrInProgress
			}
		}
		handleError(c, unavailable)
		return
	}
	if entry, err := archive.Create("manifest.json"); err == nil {
		_ = json.NewEncoder(entry).Encode(manifest)
	}
}
