package portal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
)

// cooperationContract is a separate private document type. It cannot be used
// as verified financial evidence, and its storage location is never client input.
type cooperationContract struct {
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

func (cooperationContract) TableName() string { return "organization_cooperation_contracts" }

func (a *Application) listOwnedCooperationContracts(ctx context.Context, operator string) ([]cooperationContract, error) {
	rows := []cooperationContract{}
	err := a.DB.WithContext(ctx).Where("operator_id=? AND status='ready'", operator).Order("created_at DESC,id DESC").Find(&rows).Error
	return rows, err
}

// A buyer receives access only after the supplier deliberately attaches the
// document to that buyer's agreement or quote. Historical orders retain access.
func cooperationContractScope(db *gorm.DB, operator string) *gorm.DB {
	return db.Where(`operator_id=? OR (status='ready' AND (
 EXISTS(SELECT 1 FROM organization_supply_agreements a WHERE a.contract_attachment_id=organization_cooperation_contracts.id::text AND a.supplier_id=organization_cooperation_contracts.operator_id AND a.buyer_id=?) OR
 EXISTS(SELECT 1 FROM cooperation_purchase_quotes q WHERE q.contract_attachment_id=organization_cooperation_contracts.id::text AND q.supplier_id=organization_cooperation_contracts.operator_id AND q.buyer_id=?) OR
 EXISTS(SELECT 1 FROM cooperation_purchase_orders o WHERE o.contract_attachment_id=organization_cooperation_contracts.id::text AND o.supplier_id=organization_cooperation_contracts.operator_id AND o.buyer_id=?)))`, operator, operator, operator, operator)
}

func (a *Application) contractResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows := []cooperationContract{}
	if e := cooperationContractScope(a.DB.WithContext(c.Request.Context()), op.ID).Order("created_at DESC,id DESC").Limit(100).Find(&rows).Error; e != nil {
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
	p["title"] = "合作合同附件"
	p["description"] = "可选上传合作合同，文件加密保存。先上传，再在建立合作或发布报价时选择；仅供货方和关联采购方可查看。实际供货金额、额度和退出规则以订单接受的报价为准。"
	p["items"] = items
	p["columns"] = []gin.H{col("filename", "合同文件", "text"), col("content_type", "格式", "text"), col("created_at", "上传时间", "datetime"), col("download_url", "查看合同", "link")}
	p["operations"] = []operation{{Name: "upload", Label: "上传可选合作合同", Method: "POST", Path: base, Fields: []field{f("file", "PDF/PNG/JPEG合同（不超过10MiB）", "file")}}}
	a.resourceResponse(c, p)
}

func (a *Application) uploadContract(c *gin.Context, op organization.Operator, actor string) {
	if e := a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, "cooperation.manage"); e != nil {
		handleError(c, e)
		return
	}
	businessKey, ok := key(c)
	if !ok {
		return
	}
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
	data, e := io.ReadAll(io.LimitReader(file, (10<<20)+1))
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
	if filename == "" || filename == "." || filename == "/" || len(filename) > 255 || strings.ContainsAny(filename, "\x00\r\n") {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	digest := cooperationContractDigest(contentType, filename, data)
	ctx := c.Request.Context()
	row := cooperationContract{}
	e = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "cooperation-contract:"+op.ID+":"+businessKey).Error; e != nil {
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
		row = cooperationContract{ID: uuid.NewString(), OperatorID: op.ID, BusinessKey: businessKey, Fingerprint: digest, Filename: filename, ContentType: contentType, SizeBytes: len(data), Status: "pending", CreatedBy: actor, CreatedAt: time.Now().UTC()}
		row.ObjectKey = "cooperation-contracts/" + op.ID + "/" + row.ID
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
	c.JSON(200, gin.H{"id": row.ID, "status": "ready", "message": "合同已加密保存，可在合作或供货报价中选择此附件。"})
}
func cooperationContractDigest(contentType, filename string, data []byte) string {
	hash := sha256.Sum256(append([]byte(contentType+":"+filename+":"), data...))
	return hex.EncodeToString(hash[:])
}
func (a *Application) downloadContract(c *gin.Context) {
	op, _, ok := a.staffScope(c, "contracts")
	if !ok {
		return
	}
	if _, e := uuid.Parse(c.Param("id")); e != nil {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	if a.ObjectStore == nil {
		handleError(c, objectstore.ErrUnavailable)
		return
	}
	row := cooperationContract{}
	if e := cooperationContractScope(a.DB.WithContext(c.Request.Context()), op.ID).Where("id=? AND status='ready'", c.Param("id")).First(&row).Error; e != nil {
		handleError(c, e)
		return
	}
	cipher, e := a.ObjectStore.Get(c.Request.Context(), row.ObjectKey)
	if e != nil {
		handleError(c, e)
		return
	}
	data, e := pc.Open(a.Config.AppKey, string(cipher))
	if e != nil || len(data) != row.SizeBytes || cooperationContractDigest(row.ContentType, row.Filename, []byte(data)) != row.Fingerprint {
		handleError(c, objectstore.ErrUnavailable)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": row.Filename}))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, row.ContentType, []byte(data))
}
