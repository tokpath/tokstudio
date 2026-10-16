package portal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type financeEvidence struct {
	ID          string    `json:"id"`
	OperatorID  string    `json:"-"`
	BusinessKey string    `json:"-"`
	Fingerprint string    `json:"-"`
	ObjectKey   string    `json:"-"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int       `json:"size_bytes"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (financeEvidence) TableName() string { return "portal_finance_evidence" }
func (a *Application) evidenceResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows := []financeEvidence{}
	if e := a.DB.WithContext(c.Request.Context()).Where("operator_id=?", op.ID).Order("created_at DESC,id DESC").Limit(100).Find(&rows).Error; e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, row := range rows {
		v := values(row)
		if row.Status == "ready" {
			v["download_url"] = base + "/" + row.ID + "/content"
		}
		items = append(items, entity(row.ID, row.Status, 0, v))
	}
	p["title"] = "私有金融凭据"
	p["description"] = "银行与原付款人凭据加密写入私有对象存储。上传文件本身不是付款或退款成功证明，办理事项仍需核实真实资金事实。"
	p["items"] = items
	p["columns"] = []gin.H{col("id", "证据引用ID", "text"), col("filename", "原文件名", "text"), col("content_type", "格式", "text"), col("created_by", "登记人", "text"), col("created_at", "登记时间", "datetime"), col("download_url", "授权下载", "link")}
	p["operations"] = []operation{{Name: "upload", Label: "上传私有银行/收款凭据", Method: "POST", Path: base, Fields: []field{f("file", "PDF/PNG/JPEG凭据（不超过10MiB）", "file")}}}
	a.resourceResponse(c, p)
}
func (a *Application) uploadEvidence(c *gin.Context, op organization.Operator, actor string) {
	if a.ObjectStore == nil {
		handleError(c, objectstore.ErrUnavailable)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)
	if e := c.Request.ParseMultipartForm(11 << 20); e != nil {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, header, e := c.Request.FormFile("file")
	if e != nil {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	defer file.Close()
	data, e := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if e != nil || len(data) == 0 || len(data) > 10<<20 {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	contentType := http.DetectContentType(data)
	if contentType != "application/pdf" && contentType != "image/png" && contentType != "image/jpeg" {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	filename := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if filename == "" || len(filename) > 255 || strings.ContainsAny(filename, "\x00\r\n") {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	hash := sha256.Sum256(append([]byte(contentType+":"+filename+":"), data...))
	digest := hex.EncodeToString(hash[:])
	businessKey, _ := key(c)
	ctx := c.Request.Context()
	row := financeEvidence{}
	e = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "evidence:"+op.ID+businessKey).Error; e != nil {
			return e
		}
		e := tx.First(&row, "operator_id=? AND business_key=?", op.ID, businessKey).Error
		if e == nil {
			if row.Fingerprint != digest {
				return organization.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		row = financeEvidence{ID: uuid.NewString(), OperatorID: op.ID, BusinessKey: businessKey, Fingerprint: digest, Filename: filename, ContentType: contentType, SizeBytes: len(data), Status: "pending", CreatedBy: actor, CreatedAt: time.Now().UTC()}
		row.ObjectKey = "finance-evidence/" + op.ID + "/" + row.ID
		return tx.Create(&row).Error
	})
	if e != nil {
		handleError(c, e)
		return
	}
	if row.Status != "ready" {
		cipher, e := pc.Seal(a.Config.AppKey, string(data))
		if e != nil {
			handleError(c, e)
			return
		}
		if e = a.ObjectStore.Put(ctx, row.ObjectKey, []byte(cipher), "application/octet-stream", time.Time{}); e != nil {
			handleError(c, e)
			return
		}
		if e = a.DB.WithContext(ctx).Model(&row).Update("status", "ready").Error; e != nil {
			handleError(c, e)
			return
		}
	}
	c.JSON(200, gin.H{"id": row.ID, "status": "ready", "message": "凭据已加密保存，请使用此证据ID关联原业务核实记录。"})
}
func (a *Application) downloadEvidence(c *gin.Context) {
	op, _, ok := a.staffScope(c, "evidence")
	if !ok {
		return
	}
	var row financeEvidence
	if e := a.DB.WithContext(c.Request.Context()).First(&row, "id=? AND operator_id=? AND status='ready'", c.Param("id"), op.ID).Error; e != nil {
		handleError(c, e)
		return
	}
	cipher, e := a.ObjectStore.Get(c.Request.Context(), row.ObjectKey)
	if e != nil {
		handleError(c, e)
		return
	}
	data, e := pc.Open(a.Config.AppKey, string(cipher))
	if e != nil {
		handleError(c, objectstore.ErrUnavailable)
		return
	}
	hash := sha256.Sum256(append([]byte(row.ContentType+":"+row.Filename+":"), []byte(data)...))
	if hex.EncodeToString(hash[:]) != row.Fingerprint {
		handleError(c, objectstore.ErrUnavailable)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": row.Filename}))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(200, row.ContentType, []byte(data))
}
