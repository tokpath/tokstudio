package identity

import "testing"

func TestPrincipalHasRole(t *testing.T) {
	admin := Principal{Roles: []string{"platform_admin"}}
	if !admin.HasRole("platform_admin", "audit_readonly") {
		t.Fatal("admin should pass platform_admin")
	}
	user := Principal{Roles: []string{"end_user"}}
	if user.HasRole("platform_admin") {
		t.Fatal("end user must not pass admin role")
	}
}

func TestVisibleChannelID(t *testing.T) {
	admin := Principal{Roles: []string{"platform_admin"}, ChannelOrgID: "chn_a"}
	if admin.VisibleChannelID() != "" {
		t.Fatal("platform admin must see all channels")
	}
	channelAdmin := Principal{Roles: []string{"channel_admin"}, ChannelOrgID: "chn_b"}
	if channelAdmin.VisibleChannelID() != "chn_b" {
		t.Fatal("channel admin must be scoped")
	}
}
