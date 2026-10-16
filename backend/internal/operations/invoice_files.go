package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxInvoiceBytes = 10 << 20

type InvoiceUpload struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}
type InvoiceFile struct {
	ID          string    `json:"id"`
	InvoiceID   string    `json:"invoice_id"`
	OperatorID  string    `json:"operator_id"`
	BusinessKey string    `json:"-"`
	Fingerprint string    `json:"-"`
	ObjectKey   string    `json:"-"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"size_bytes"`
	State       string    `json:"state"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (InvoiceFile) TableName() string { return "operations_invoice_files" }

type InvoiceDownload struct {
	Filename, ContentType string
	Data                  []byte
}

// UploadInvoiceFile claims one immutable payload before S3 I/O. A repeated
// business key resumes that same encrypted object after a crash; changed bytes
// cannot replace a previously uploaded legal document.
func (s *Service) UploadInvoiceFile(ctx context.Context, actor, operator, invoiceID, key string, in InvoiceUpload) (InvoiceFile, error) {
	out := InvoiceFile{}
	if s.InvoiceStore == nil || s.EncryptionKey == "" {
		return out, ErrNotReady
	}
	if key == "" || len(in.Data) == 0 || len(in.Data) > MaxInvoiceBytes || in.Filename == "" || len(in.Filename) > 180 || filepath.Base(in.Filename) != in.Filename || strings.ContainsAny(in.Filename, "\\\r\n\x00") {
		return out, ErrInvalid
	}
	mime := http.DetectContentType(in.Data)
	if mime != "application/pdf" && mime != "image/png" && mime != "image/jpeg" {
		return out, ErrInvalid
	}
	if in.ContentType != "" && in.ContentType != mime {
		return out, ErrInvalid
	}
	in.ContentType = mime
	sum := sha256.Sum256(in.Data)
	digest := hex.EncodeToString(sum[:])
	fp := hash([]string{in.Filename, mime, digest})
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		var invoice Invoice
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invoice, "id=? AND operator_id=?", invoiceID, operator).Error; e != nil {
			return e
		}
		if invoice.State == "rejected" {
			return ErrConflict
		}
		q := tx.Where("invoice_id=? AND business_key=?", invoiceID, key).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		var now time.Time
		if e := tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; e != nil {
			return e
		}
		id := uuid.NewString()
		out = InvoiceFile{ID: id, InvoiceID: invoiceID, OperatorID: operator, BusinessKey: key, Fingerprint: fp, ObjectKey: "invoices/" + operator + "/" + invoiceID + "/" + id, Filename: in.Filename, ContentType: mime, SHA256: digest, SizeBytes: int64(len(in.Data)), State: "uploading", CreatedBy: actor, CreatedAt: now}
		return tx.Create(&out).Error
	})
	if e != nil || out.State == "ready" {
		return out, e
	}
	sealed, e := s.seal(in.Data)
	if e != nil {
		return out, e
	}
	if e = s.InvoiceStore.Put(ctx, out.ObjectKey, []byte(sealed), "application/octet-stream", time.Time{}); e != nil {
		return out, e
	}
	e = s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return e
		}
		return tx.Model(&InvoiceFile{}).Where("id=? AND fingerprint=?", out.ID, fp).Update("state", "ready").Error
	})
	if e == nil {
		out.State = "ready"
	}
	return out, e
}

func (s *Service) invoiceReader(ctx context.Context, tx *gorm.DB, actor, operator, invoiceID string, customer bool, buyer string) (Invoice, error) {
	var out Invoice
	if e := tx.First(&out, "id=?", invoiceID).Error; e != nil {
		return out, e
	}
	if buyer != "" {
		if e := s.require(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
			return out, e
		}
		if out.PartyKind != "procurement" || out.BuyerOperatorID == nil || *out.BuyerOperatorID != buyer {
			return out, ErrForbidden
		}
	} else if customer {
		// Identity is obtained by the authenticated caller. Historical legal
		// documents remain readable after service ownership changes issuer.
		if !validID(actor) || out.PartyKind != "customer" || out.CustomerID != actor || out.OperatorID != operator {
			return out, ErrForbidden
		}
	} else {
		if out.OperatorID != operator {
			return out, ErrForbidden
		}
		if e := s.require(ctx, tx, actor, operator, "invoices.manage"); e != nil {
			return out, e
		}
	}
	if (customer || buyer != "") && (out.State != "issued" && out.State != "review_required" && out.State != "voided") {
		return out, ErrConflict
	}
	return out, nil
}
func (s *Service) listInvoiceFiles(ctx context.Context, actor, operator, id string, customer bool, buyer string) ([]InvoiceFile, error) {
	out := []InvoiceFile{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if _, e := s.invoiceReader(ctx, tx, actor, operator, id, customer, buyer); e != nil {
			return e
		}
		return tx.Where("invoice_id=? AND state='ready'", id).Order("created_at,id").Find(&out).Error
	})
	return out, e
}
func (s *Service) ListInvoiceFiles(ctx context.Context, actor, operator, id string, customer bool) ([]InvoiceFile, error) {
	return s.listInvoiceFiles(ctx, actor, operator, id, customer, "")
}
func (s *Service) ListProcurementInvoiceFiles(ctx context.Context, actor, buyer, id string) ([]InvoiceFile, error) {
	return s.listInvoiceFiles(ctx, actor, "", id, false, buyer)
}

func (s *Service) downloadInvoiceFile(ctx context.Context, actor, operator, id, fileID string, customer bool, buyer string) (InvoiceDownload, error) {
	out := InvoiceDownload{}
	if s.InvoiceStore == nil {
		return out, ErrNotReady
	}
	var f InvoiceFile
	authorize := func(tx *gorm.DB) error {
		if _, e := s.invoiceReader(ctx, tx, actor, operator, id, customer, buyer); e != nil {
			return e
		}
		return tx.First(&f, "id=? AND invoice_id=? AND state='ready'", fileID, id).Error
	}
	if e := s.within(ctx, nil, authorize); e != nil {
		return out, e
	}
	encrypted, e := s.InvoiceStore.Get(ctx, f.ObjectKey)
	if e != nil {
		return out, e
	}
	var data []byte
	if e = s.open(string(encrypted), &data); e != nil {
		return out, e
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != f.SizeBytes || hex.EncodeToString(sum[:]) != f.SHA256 {
		return out, ErrConflict
	}
	e = s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := authorize(tx); e != nil {
			return e
		}
		return tx.Exec(`INSERT INTO operations_invoice_file_access(id,file_id,invoice_id,actor_id,at) VALUES(?,?,?,?,clock_timestamp())`, uuid.NewString(), fileID, id, actor).Error
	})
	if e != nil {
		return out, e
	}
	return InvoiceDownload{Filename: f.Filename, ContentType: f.ContentType, Data: data}, nil
}
func (s *Service) DownloadInvoiceFile(ctx context.Context, actor, operator, id, fileID string, customer bool) (InvoiceDownload, error) {
	return s.downloadInvoiceFile(ctx, actor, operator, id, fileID, customer, "")
}
func (s *Service) DownloadProcurementInvoiceFile(ctx context.Context, actor, buyer, id, fileID string) (InvoiceDownload, error) {
	return s.downloadInvoiceFile(ctx, actor, "", id, fileID, false, buyer)
}
