package branding

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func DefaultTheme() Theme {
	return Theme{PrimaryColor: "#2563eb", BackgroundColor: "#ffffff", TextColor: "#111827"}
}
func InheritTheme(base Theme, overrides ThemeOverrides) Theme {
	if overrides.PrimaryColor != nil {
		base.PrimaryColor = *overrides.PrimaryColor
	}
	if overrides.BackgroundColor != nil {
		base.BackgroundColor = *overrides.BackgroundColor
	}
	if overrides.TextColor != nil {
		base.TextColor = *overrides.TextColor
	}
	if overrides.LogoURL != nil {
		base.LogoURL = *overrides.LogoURL
	}
	if overrides.SupportURL != nil {
		base.SupportURL = *overrides.SupportURL
	}
	return base
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ValidateTheme(v ThemeOverrides) error {
	for _, c := range []*string{v.PrimaryColor, v.BackgroundColor, v.TextColor} {
		if c != nil && !colorPattern.MatchString(*c) {
			return ErrInvalidInput
		}
	}
	for i, p := range []*string{v.LogoURL, v.SupportURL} {
		if p == nil || *p == "" {
			continue
		}
		if i == 0 {
			if _, ok := logoURLID(*p); ok {
				continue
			}
		}
		u, err := url.Parse(*p)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

type themeRow struct {
	ID, BrandID, ScopeType, ScopeID, ValuesJSON string
	Version                                     int64
	UpdatedBy                                   string
	UpdatedAt                                   time.Time
}

func (themeRow) TableName() string { return "branding_theme_configs" }
func (r themeRow) DTO() (ThemeConfig, error) {
	var v ThemeOverrides
	err := json.Unmarshal([]byte(r.ValuesJSON), &v)
	return ThemeConfig{BrandID: r.BrandID, ScopeType: r.ScopeType, ScopeID: r.ScopeID, Values: v, Version: r.Version, UpdatedAt: r.UpdatedAt}, err
}
func (s *Service) SetBrandTheme(ctx context.Context, tx *gorm.DB, in SetThemeInput) (ThemeConfig, error) {
	return s.setTheme(ctx, tx, "brand", in)
}
func (s *Service) SetChannelTheme(ctx context.Context, tx *gorm.DB, in SetThemeInput) (ThemeConfig, error) {
	return s.setTheme(ctx, tx, "channel", in)
}
func (s *Service) setTheme(ctx context.Context, tx *gorm.DB, scope string, in SetThemeInput) (out ThemeConfig, err error) {
	if !validID(in.ActorStaffID) || !validID(in.TargetID) || in.ExpectedVersion < 0 {
		return out, ErrInvalidInput
	}
	if err = ValidateTheme(in.Values); err != nil {
		return out, err
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		brandID := in.TargetID
		if scope == "brand" {
			if err := s.directory.RequireBrandBranding(ctx, tx, in.ActorStaffID, in.TargetID); err != nil {
				return err
			}
		} else {
			if err := s.directory.RequireChannelBranding(ctx, tx, in.ActorStaffID, in.TargetID); err != nil {
				return err
			}
			o, err := s.directory.GetOperator(ctx, tx, in.TargetID)
			if err != nil {
				return err
			}
			brandID = o.BrandID
		}
		if err := s.validateLogoReference(ctx, tx, brandID, scope, in.TargetID, in.Values.LogoURL); err != nil {
			return err
		}
		var old themeRow
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("scope_type=? AND scope_id=?", scope, in.TargetID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		var before any
		if q.RowsAffected > 0 {
			if old.Version != in.ExpectedVersion {
				return ErrConflict
			}
			c, err := old.DTO()
			if err != nil {
				return err
			}
			before = c
		} else if in.ExpectedVersion != 0 {
			return ErrConflict
		}
		b, err := json.Marshal(in.Values)
		if err != nil {
			return err
		}
		r := themeRow{ID: uuid.NewString(), BrandID: brandID, ScopeType: scope, ScopeID: in.TargetID, ValuesJSON: string(b), Version: in.ExpectedVersion + 1, UpdatedBy: in.ActorStaffID, UpdatedAt: s.now()}
		if q.RowsAffected > 0 {
			r.ID = old.ID
			err = tx.Exec(`UPDATE branding_theme_configs SET values_json=?::jsonb,version=?,updated_by=?,updated_at=? WHERE id=?`, r.ValuesJSON, r.Version, r.UpdatedBy, r.UpdatedAt, r.ID).Error
		} else {
			err = tx.Exec(`INSERT INTO branding_theme_configs(id,brand_id,scope_type,scope_id,values_json,version,updated_by,updated_at) VALUES(?,?,?,?,?::jsonb,?,?,?)`, r.ID, r.BrandID, r.ScopeType, r.ScopeID, r.ValuesJSON, r.Version, r.UpdatedBy, r.UpdatedAt).Error
		}
		if err != nil {
			return err
		}
		out, err = r.DTO()
		if err != nil {
			return err
		}
		return s.audit(tx, in.ActorStaffID, brandID, scope, in.TargetID, "theme.changed", before, out)
	})
	return
}
func (s *Service) themeConfig(ctx context.Context, tx *gorm.DB, scope, id string) (ThemeConfig, error) {
	var r themeRow
	q := s.dbFor(ctx, tx).Where("scope_type=? AND scope_id=?", scope, id).Limit(1).Find(&r)
	if q.Error != nil {
		return ThemeConfig{}, q.Error
	}
	if q.RowsAffected == 0 {
		return ThemeConfig{ScopeType: scope, ScopeID: id}, nil
	}
	return r.DTO()
}
func (s *Service) ResolveTheme(ctx context.Context, tx *gorm.DB, brandID, channelID string) (out ThemeContext, err error) {
	b, err := s.directory.GetBrand(ctx, tx, brandID)
	if err != nil {
		return out, err
	}
	opID := b.OwnerOperatorID
	if channelID != "" {
		o, err := s.directory.GetOperator(ctx, tx, channelID)
		if err != nil {
			return out, err
		}
		if o.Kind != organization.Channel || o.BrandID != brandID {
			return out, organization.ErrForbidden
		}
		opID = o.ID
	}
	o, err := s.directory.GetOperator(ctx, tx, opID)
	if err != nil {
		return out, err
	}
	bc, err := s.themeConfig(ctx, tx, "brand", brandID)
	if err != nil {
		return out, err
	}
	theme := InheritTheme(DefaultTheme(), bc.Values)
	out = ThemeContext{Brand: b, Operator: o, Theme: theme, BrandVersion: bc.Version}
	if channelID != "" {
		cc, err := s.themeConfig(ctx, tx, "channel", channelID)
		if err != nil {
			return out, err
		}
		out.Theme = InheritTheme(theme, cc.Values)
		out.ChannelVersion = cc.Version
	}
	return out, nil
}
