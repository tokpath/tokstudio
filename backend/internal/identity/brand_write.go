package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type ObjectStore interface {
	Put(key, contentType string, data []byte) error
	Read(key string) ([]byte, error)
	Delete(key string) error
}

type BrandInput struct {
	Name          string         `json:"name"`
	PrimaryDomain string         `json:"primary_domain"`
	APIDomain     string         `json:"api_domain"`
	AdminDomain   string         `json:"admin_domain"`
	Theme         map[string]any `json:"theme"`
}

type AssetView struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ContentType string `json:"content_type"`
	SizeBytes   int    `json:"size_bytes"`
	WidthPx     int    `json:"width_px"`
	HeightPx    int    `json:"height_px"`
	URL         string `json:"url"`
}

type brandAssetRow struct {
	ID          string    `gorm:"column:id;primaryKey"`
	BrandID     string    `gorm:"column:brand_id"`
	Kind        string    `gorm:"column:kind"`
	ObjectKey   string    `gorm:"column:object_key"`
	ContentType string    `gorm:"column:content_type"`
	SizeBytes   int64     `gorm:"column:size_bytes"`
	WidthPx     int       `gorm:"column:width_px"`
	HeightPx    int       `gorm:"column:height_px"`
	SHA256      string    `gorm:"column:sha256"`
	Status      string    `gorm:"column:status"`
	CreatedBy   string    `gorm:"column:created_by"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (brandAssetRow) TableName() string { return "identity_brand_assets" }

func (s *Service) SetStore(store ObjectStore) {
	s.store = store
}

func (s *Service) ChannelIDByBrand(ctx context.Context, brandID string) (string, error) {
	var row channelRow
	err := s.db.WithContext(ctx).Where("brand_id = ?", brandID).Order("id").First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return OfficialChannelID, nil
		}
		return "", err
	}
	return row.ID, nil
}

func (s *Service) CreateBrand(ctx context.Context, viewer Principal, in BrandInput) (*BrandView, error) {
	if !viewer.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PrimaryDomain = strings.ToLower(strings.TrimSpace(in.PrimaryDomain))
	in.APIDomain = strings.ToLower(strings.TrimSpace(in.APIDomain))
	in.AdminDomain = strings.ToLower(strings.TrimSpace(in.AdminDomain))
	if in.Name == "" || in.PrimaryDomain == "" || in.APIDomain == "" || in.AdminDomain == "" {
		return nil, ErrPromotionInvalid
	}
	theme, err := NormalizeTheme(in.Theme)
	if err != nil {
		return nil, err
	}
	if display := strings.TrimSpace(theme["display_name"]); display != "" {
		in.Name = display
	}
	if s.KnownBrandHost(ctx, in.PrimaryDomain) || s.KnownBrandHost(ctx, in.APIDomain) || s.KnownBrandHost(ctx, in.AdminDomain) {
		return nil, ErrBrandDomainTaken
	}
	row := brandRow{
		ID: id.New("brd"), Name: in.Name, PrimaryDomain: in.PrimaryDomain,
		APIDomain: in.APIDomain, AdminDomain: in.AdminDomain,
		ThemeJSON: themeBytes(theme), CreatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return brandView(row), nil
}

func (s *Service) PatchBrand(ctx context.Context, brandID string, in BrandInput) (*BrandView, error) {
	var row brandRow
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&row).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if name := strings.TrimSpace(in.Name); name != "" {
		updates["name"] = name
	}
	if in.PrimaryDomain != "" {
		updates["primary_domain"] = strings.ToLower(strings.TrimSpace(in.PrimaryDomain))
	}
	if in.APIDomain != "" {
		updates["api_domain"] = strings.ToLower(strings.TrimSpace(in.APIDomain))
	}
	if in.AdminDomain != "" {
		updates["admin_domain"] = strings.ToLower(strings.TrimSpace(in.AdminDomain))
	}
	if in.Theme != nil {
		stored := map[string]any{}
		_ = json.Unmarshal(row.ThemeJSON, &stored)
		current := map[string]any{}
		for k, v := range stored {
			if _, ok := themeAllowed[k]; ok {
				current[k] = v
			}
		}
		for k, v := range in.Theme {
			current[k] = v
		}
		theme, err := NormalizeTheme(current)
		if err != nil {
			return nil, err
		}
		if display := strings.TrimSpace(theme["display_name"]); display != "" {
			updates["name"] = display
		}
		updates["theme_json"] = themeBytes(theme)
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&brandRow{}).Where("id = ?", brandID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&row).Error; err != nil {
		return nil, err
	}
	return brandView(row), nil
}

func (s *Service) ChannelBrand(ctx context.Context, viewer Principal) (*BrandView, *ChannelView, error) {
	if viewer.ChannelOrgID == "" {
		return nil, nil, ErrNotFound
	}
	var channel channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", viewer.ChannelOrgID).First(&channel).Error; err != nil {
		return nil, nil, err
	}
	var row brandRow
	if err := s.db.WithContext(ctx).Where("id = ?", channel.BrandID).First(&row).Error; err != nil {
		return nil, nil, err
	}
	return brandView(row), &ChannelView{ID: channel.ID, Code: channel.Code, Type: channel.Type, Status: channel.Status, BrandID: channel.BrandID}, nil
}

func (s *Service) AssertBrandWritable(channel *ChannelView, viewer Principal, brandID string) error {
	if viewer.IsPlatformAdmin() {
		return nil
	}
	if channel == nil || channel.Type != "C" || channel.BrandID != brandID {
		return ErrBrandNotCustomizable
	}
	return nil
}

func (s *Service) UploadBrandAsset(ctx context.Context, viewer Principal, brandID, kind, filename string, data []byte) (*AssetView, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	decoded, err := ValidateAsset(kind, filename, data)
	if err != nil {
		return nil, err
	}
	var n int64
	_ = s.db.WithContext(ctx).Model(&brandAssetRow{}).
		Where("brand_id = ? AND created_at > ?", brandID, time.Now().UTC().Add(-time.Hour)).
		Count(&n).Error
	if n >= AssetHourLimit {
		return nil, ErrAssetRateLimited
	}
	var brand brandRow
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&brand).Error; err != nil {
		return nil, err
	}
	assetID := id.New("bas")
	key := BrandAssetObjectKey(brandID, decoded.Kind, decoded.SHA256, decoded.Ext)
	if err := s.store.Put(key, decoded.ContentType, data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&brandAssetRow{}).Where("brand_id = ? AND kind = ? AND status = ?", brandID, decoded.Kind, "active").
			Update("status", "superseded").Error; err != nil {
			return err
		}
		if err := tx.Create(&brandAssetRow{
			ID: assetID, BrandID: brandID, Kind: decoded.Kind, ObjectKey: key,
			ContentType: decoded.ContentType, SizeBytes: int64(decoded.Size),
			WidthPx: decoded.Width, HeightPx: decoded.Height, SHA256: decoded.SHA256,
			Status: "active", CreatedBy: viewer.UserID, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		url := BrandAssetURL(assetID)
		switch decoded.Kind {
		case AssetLogo:
			return tx.Model(&brandRow{}).Where("id = ?", brandID).Update("logo_url", url).Error
		case AssetLogoDark:
			return tx.Model(&brandRow{}).Where("id = ?", brandID).Update("logo_dark_url", url).Error
		case AssetFavicon:
			return tx.Model(&brandRow{}).Where("id = ?", brandID).Update("favicon_url", url).Error
		default:
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return &AssetView{
		ID: assetID, Kind: decoded.Kind, ContentType: decoded.ContentType,
		SizeBytes: decoded.Size, WidthPx: decoded.Width, HeightPx: decoded.Height,
		URL: BrandAssetURL(assetID),
	}, nil
}

func (s *Service) PublicBrandAsset(ctx context.Context, assetID string) (*brandAssetRow, []byte, error) {
	if s.store == nil {
		return nil, nil, ErrStoreUnavailable
	}
	var row brandAssetRow
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", assetID, "active").First(&row).Error; err != nil {
		return nil, nil, err
	}
	body, err := s.store.Read(row.ObjectKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return &row, body, nil
}

func (s *Service) BrandByID(ctx context.Context, brandID string) (*BrandView, error) {
	var row brandRow
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&row).Error; err != nil {
		return nil, err
	}
	return brandView(row), nil
}
