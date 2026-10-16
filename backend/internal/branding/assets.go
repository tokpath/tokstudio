package branding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

const MaxLogoBytes = 2 << 20

type UploadLogoInput struct {
	ActorStaffID, ScopeType, ScopeID, Filename, ContentType string
	Data                                                    []byte
}
type LogoAsset struct {
	ID          string    `json:"id"`
	BrandID     string    `json:"brand_id"`
	ScopeType   string    `json:"scope_type"`
	ScopeID     string    `json:"scope_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Status      string    `json:"status"`
	URL         string    `gorm:"-" json:"url"`
	CreatedAt   time.Time `json:"created_at"`
}
type logoAssetRow struct {
	LogoAsset  `gorm:"embedded"`
	ObjectKey  string
	UploadedBy string
}

func (logoAssetRow) TableName() string { return "branding_logo_assets" }
func logoURL(id string) string         { return "/v1/branding/assets/" + id + "/content" }
func logoURLID(value string) (string, bool) {
	const prefix = "/v1/branding/assets/"
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "/content") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "/content")
	return id, validID(id) && value == logoURL(id)
}
func (s *Service) requireLogoScope(ctx context.Context, tx *gorm.DB, actor, scope, id string) (string, error) {
	if scope == "brand" {
		if e := s.directory.RequireBrandBranding(ctx, tx, actor, id); e != nil {
			return "", e
		}
		return id, nil
	}
	if scope != "channel" {
		return "", ErrInvalidInput
	}
	if e := s.directory.RequireChannelBranding(ctx, tx, actor, id); e != nil {
		return "", e
	}
	op, e := s.directory.GetOperator(ctx, tx, id)
	if e != nil {
		return "", e
	}
	return op.BrandID, nil
}
func validateLogo(data []byte, declared string) (string, image.Config, error) {
	if len(data) == 0 || len(data) > MaxLogoBytes {
		return "", image.Config{}, ErrInvalidInput
	}
	c, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || c.Width <= 0 || c.Height <= 0 || int64(c.Width)*int64(c.Height) > 10000000 {
		return "", c, ErrInvalidInput
	}
	content := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if content == "" || (declared != "" && declared != "application/octet-stream" && declared != content) {
		return "", c, ErrInvalidInput
	}
	// Decode the full image: a valid header followed by truncated or malformed
	// image data must not become a theme asset.
	if _, _, e = image.Decode(bytes.NewReader(data)); e != nil {
		return "", c, ErrInvalidInput
	}
	return content, c, nil
}

func (s *Service) UploadLogo(ctx context.Context, in UploadLogoInput) (LogoAsset, error) {
	if s.assets == nil {
		return LogoAsset{}, objectstore.ErrUnavailable
	}
	if !validID(in.ActorStaffID) || !validID(in.ScopeID) {
		return LogoAsset{}, ErrInvalidInput
	}
	content, dimensions, e := validateLogo(in.Data, strings.TrimSpace(in.ContentType))
	if e != nil {
		return LogoAsset{}, e
	}
	name := filepath.Base(strings.ReplaceAll(in.Filename, "\\", "/"))
	if name == "" || name == "." || len(name) > 255 || strings.ContainsAny(name, "\r\n\x00") {
		return LogoAsset{}, ErrInvalidInput
	}
	hash := sha256.Sum256(in.Data)
	id := uuid.NewString()
	r := logoAssetRow{LogoAsset: LogoAsset{ID: id, ScopeType: in.ScopeType, ScopeID: in.ScopeID, Filename: name, ContentType: content, Size: int64(len(in.Data)), SHA256: hex.EncodeToString(hash[:]), Width: dimensions.Width, Height: dimensions.Height, Status: "uploading", CreatedAt: s.now()}, ObjectKey: "branding/logos/" + id, UploadedBy: in.ActorStaffID}
	e = s.withTx(ctx, nil, func(tx *gorm.DB) error {
		var e error
		r.BrandID, e = s.requireLogoScope(ctx, tx, in.ActorStaffID, in.ScopeType, in.ScopeID)
		if e != nil {
			return e
		}
		return tx.Create(&r).Error
	})
	if e != nil {
		return LogoAsset{}, e
	}
	if e = s.assets.Put(ctx, r.ObjectKey, in.Data, content, time.Time{}); e != nil {
		_ = s.db.WithContext(ctx).Model(&logoAssetRow{}).Where("id=?", id).Update("status", "failed").Error
		return LogoAsset{}, e
	}
	e = s.withTx(ctx, nil, func(tx *gorm.DB) error {
		if _, e := s.requireLogoScope(ctx, tx, in.ActorStaffID, in.ScopeType, in.ScopeID); e != nil {
			return e
		}
		if e := tx.Model(&logoAssetRow{}).Where("id=? AND status='uploading'", id).Update("status", "ready").Error; e != nil {
			return e
		}
		r.Status = "ready"
		return s.audit(tx, in.ActorStaffID, r.BrandID, "logo", id, "logo.uploaded", nil, r.LogoAsset)
	})
	if e != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_ = s.assets.Delete(cleanupCtx, r.ObjectKey)
		_ = s.db.WithContext(cleanupCtx).Model(&logoAssetRow{}).Where("id=?", id).Update("status", "failed").Error
		return LogoAsset{}, e
	}
	r.URL = logoURL(id)
	return r.LogoAsset, nil
}
func (s *Service) logo(ctx context.Context, id string) (logoAssetRow, error) {
	var r logoAssetRow
	if !validID(id) {
		return r, ErrInvalidInput
	}
	e := s.dbFor(ctx, nil).First(&r, "id=? AND status='ready'", id).Error
	r.URL = logoURL(id)
	return r, classify(e)
}
func (s *Service) logoBytes(ctx context.Context, r logoAssetRow) (LogoAsset, []byte, error) {
	if s.assets == nil {
		return LogoAsset{}, nil, objectstore.ErrUnavailable
	}
	data, e := s.assets.Get(ctx, r.ObjectKey)
	if e != nil {
		return LogoAsset{}, nil, e
	}
	hash := sha256.Sum256(data)
	if int64(len(data)) != r.Size || hex.EncodeToString(hash[:]) != r.SHA256 {
		return LogoAsset{}, nil, fmt.Errorf("%w: logo integrity mismatch", objectstore.ErrUnavailable)
	}
	return r.LogoAsset, data, nil
}
func (s *Service) ReadLogo(ctx context.Context, actor, id string) (LogoAsset, []byte, error) {
	r, e := s.logo(ctx, id)
	if e != nil {
		return LogoAsset{}, nil, e
	}
	if _, e = s.requireLogoScope(ctx, nil, actor, r.ScopeType, r.ScopeID); e != nil {
		return LogoAsset{}, nil, e
	}
	return s.logoBytes(ctx, r)
}

// PublicLogo serves only the exact asset currently rendered by this active
// tenant theme. Knowing an unused private asset ID never grants access.
func (s *Service) PublicLogo(ctx context.Context, brand, channel, id string) (LogoAsset, []byte, error) {
	r, e := s.logo(ctx, id)
	if e != nil {
		return LogoAsset{}, nil, e
	}
	if r.BrandID != brand {
		return LogoAsset{}, nil, ErrNotFound
	}
	// A public image URL need not carry account state or a channel query string.
	// The immutable asset scope supplies its presentation context; publication
	// still requires the exact current theme to reference this asset.
	if channel == "" && r.ScopeType == "channel" {
		channel = r.ScopeID
	}
	theme, e := s.ResolveTheme(ctx, nil, brand, channel)
	if e != nil {
		return LogoAsset{}, nil, e
	}
	if theme.Brand.Status != "active" || theme.Theme.LogoURL != logoURL(id) {
		return LogoAsset{}, nil, ErrNotFound
	}
	if r.BrandID != brand || !(r.ScopeType == "brand" || (r.ScopeType == "channel" && r.ScopeID == channel)) {
		return LogoAsset{}, nil, organization.ErrForbidden
	}
	return s.logoBytes(ctx, r)
}
func (s *Service) validateLogoReference(ctx context.Context, tx *gorm.DB, brand, scope, id string, value *string) error {
	if value == nil || *value == "" || !strings.HasPrefix(*value, "/") {
		return nil
	}
	assetID, ok := logoURLID(*value)
	if !ok {
		return ErrInvalidInput
	}
	var asset logoAssetRow
	if e := tx.First(&asset, "id=? AND status='ready'", assetID).Error; e != nil {
		return classify(e)
	}
	if asset.BrandID != brand || !(asset.ScopeType == "brand" && asset.ScopeID == brand || scope == "channel" && asset.ScopeType == "channel" && asset.ScopeID == id) {
		return organization.ErrForbidden
	}
	return nil
}
