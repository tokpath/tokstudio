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
