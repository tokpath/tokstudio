package portal

import (
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestActionAuthorityMatchesThePermissionThatActuallyPerformsIt(t *testing.T) {
	contains := func(values []string, want string) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}
	finance := navigationPermissions([]string{"payments.manage", "refunds.manage"}, organization.OEM)
	if !contains(finance, "exits.read") || !contains(finance, "refunds.read") {
		t.Fatal("finance cannot reach the exit or refund workspaces where payment actions live", finance)
	}
	cooperation := navigationPermissions([]string{"cooperation.manage"}, organization.OEM)
	if !contains(cooperation, "exits.read") || contains(cooperation, "refunds.read") {
		t.Fatal("procurement authority opened the wrong finance workspace", cooperation)
	}
	marketing := businessFields(operation{Path: "/api/v1/operators/operator/marketing", Fields: []field{jsonField("")}})
	for _, field := range marketing {
		if field.Name == "concession_rate" {
			t.Fatal("operator campaign form still asks for a KOL concession")
		}
	}
	activity := businessFields(operation{Path: "/api/v1/me/activities", Fields: []field{jsonField("")}})
	found := false
	for _, field := range activity {
		if field.Name == "concession_rate" {
			found = true
		}
	}
	if !found {
		t.Fatal("KOL activity form lost the concession field")
	}
}
