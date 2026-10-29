// Package operations owns business notifications, operational review and automatic expiry
// of model results and exports. It cannot mutate financial balances or silently resolve discrepancies.
package operations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	appcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
	"io/fs"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { sub, _ := fs.Sub(migrations, "migrate"); return "operations", sub }

var ErrForbidden = errors.New("operations permission denied")
var ErrInvalid = errors.New("invalid operations command")
var ErrConflict = errors.New("operations command conflicts with recorded facts")
var ErrNotReady = errors.New("operations dependency is not configured")
var ErrExpired = errors.New("temporary content has expired")
var ErrTooLarge = errors.New("export range contains too many records; select a smaller range")

type Authorizer interface {
	RequirePermission(context.Context, *gorm.DB, string, string, string) error
}
type Recipient struct {
	ID, OperatorID, Email, BrandID, Brand string
	EmailVerified                         bool
}

// ResolveRecipient must verify the current account relationship to the exact
// operator. A supplier relationship alone does not grant customer visibility.
type RecipientDirectory interface {
	ResolveRecipient(context.Context, *gorm.DB, string, string) (Recipient, error)
}
type Service struct {
	DB                  *gorm.DB
	EncryptionKey       string
	Authorize           Authorizer
	Recipients          RecipientDirectory
	Mail                mail.Sender
	Source              Source
	Invoices            InvoiceSource
	ProcurementInvoices ProcurementInvoiceSource
	InvoiceStore        objectstore.BlobStore
	Results             ResultRetention
	Restrictions        RestrictionReviewDirectory
}

func (s *Service) within(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return s.DB.WithContext(ctx).Transaction(fn)
}
func (s *Service) require(ctx context.Context, tx *gorm.DB, actor, operator, permission string) error {
	if s.Authorize == nil {
		return ErrNotReady
	}
	return s.Authorize.RequirePermission(ctx, tx, actor, operator, permission)
}
func (s *Service) self(ctx context.Context, tx *gorm.DB, actor, operator string) (Recipient, error) {
	if s.Recipients == nil {
		return Recipient{}, ErrNotReady
	}
	r, e := s.Recipients.ResolveRecipient(ctx, tx, operator, actor)
	if e != nil {
		return r, e
	}
	if r.ID != actor || r.OperatorID != operator {
		return r, ErrForbidden
	}
	return r, nil
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validID(s string) bool { _, e := uuid.Parse(s); return e == nil }
func lock(tx *gorm.DB, key string) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "operations:"+key).Error
}
func (s *Service) seal(v any) (string, error) {
	if s.EncryptionKey == "" {
		return "", ErrNotReady
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	return appcrypto.Seal(s.EncryptionKey, string(b))
}
func (s *Service) open(ciphertext string, v any) error {
	if s.EncryptionKey == "" {
		return ErrNotReady
	}
	b, e := appcrypto.Open(s.EncryptionKey, ciphertext)
	if e != nil {
		return e
	}
	return json.Unmarshal([]byte(b), v)
}
