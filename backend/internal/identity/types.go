package identity

import "time"

// Principal 是跨模块可安全传递的身份视图，不是 ORM Model。
type Principal struct {
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	Roles        []string `json:"roles"`
	ChannelOrgID string   `json:"channel_org_id,omitempty"`
	BrandID      string   `json:"brand_id,omitempty"`
	ScopeType    string   `json:"scope_type,omitempty"`
	ScopeID      string   `json:"scope_id,omitempty"`
}

func (p Principal) HasRole(codes ...string) bool {
	wanted := map[string]struct{}{}
	for _, code := range codes {
		wanted[code] = struct{}{}
	}
	for _, role := range p.Roles {
		if _, ok := wanted[role]; ok {
			return true
		}
	}
	return false
}

func (p Principal) IsPlatformAdmin() bool {
	return p.HasRole("platform_admin")
}

// VisibleChannelID 返回查询时应强制使用的渠道。平台管理员可看全部（空字符串）。
func (p Principal) VisibleChannelID() string {
	if p.IsPlatformAdmin() || p.HasRole("finance_admin", "ops_admin", "tech_admin", "audit_readonly") {
		return ""
	}
	return p.ChannelOrgID
}

type UserView struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	ChannelOrgID string    `json:"channel_org_id,omitempty"`
	BrandID      string    `json:"brand_id,omitempty"`
	Roles        []string  `json:"roles,omitempty"`
	SourceCode   string    `json:"source_code,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type BrandView struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	LogoURL       string         `json:"logo_url,omitempty"`
	PrimaryDomain string         `json:"primary_domain"`
	APIDomain     string         `json:"api_domain"`
	AdminDomain   string         `json:"admin_domain"`
	Theme         map[string]any `json:"theme"`
}

type ChannelView struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	BrandID string `json:"brand_id"`
}

type Session struct {
	Token     string    `json:"token"`
	User      UserView  `json:"user"`
	ExpiresAt time.Time `json:"expires_at"`
}

type RegisterInput struct {
	Email         string
	Password      string
	PromotionCode string
	OTP           string
}

type LoginInput struct {
	Email    string
	Password string
}
