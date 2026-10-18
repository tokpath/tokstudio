package portal

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

func customerKeyFixture(t *testing.T) (*testApp, map[string]any) {
	t.Helper()
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "key-replay@example.test")
	f.login(t, "customer", "key-replay@example.test", "customer-password-test")
	status, out, _ := f.call(t, "POST", "/v1/me/keys", map[string]any{
		"name": "Original application", "model_ids": nil,
		"budget": map[string]any{"limit_atg": "5.000000000000000001", "period": "month", "timezone": "UTC"},
	}, "")
	mustOK(t, status, out)
	return f, out["key"].(map[string]any)
}

func TestPostgresCustomerKeyConfigurationReplaysOriginalResponse(t *testing.T) {
	f, created := customerKeyFixture(t)
	id := created["id"].(string)
	path := "/v1/me/keys/" + id
	input := map[string]any{"name": "Updated application", "model_ids": []string{}, "budget": map[string]any{"limit_atg": "0.000000000000000001", "period": "quarter", "timezone": "UTC"}, "version": 1}
	businessKey := uuid.NewString()
	status, original, _ := f.call(t, "PATCH", path, input, businessKey)
	mustOK(t, status, original)
	if original["version"] != float64(2) || original["budget"].(map[string]any)["limit_atg"] != "0.000000000000000001" {
		t.Fatal("configuration response lost version or precision", original)
	}
	status, replay, _ := f.call(t, "PATCH", path, input, businessKey)
	mustOK(t, status, replay)
	if !reflect.DeepEqual(replay, original) {
		t.Fatal("lost response retry did not return the original configuration", original, replay)
	}
	status, advanced, _ := f.call(t, "POST", path+"/disable", map[string]any{"version": 2}, "")
	mustOK(t, status, advanced)
	if advanced["version"] != float64(3) {
		t.Fatal(advanced)
	}
	status, replay, _ = f.call(t, "PATCH", path, input, businessKey)
	mustOK(t, status, replay)
	if !reflect.DeepEqual(replay, original) {
		t.Fatal("replay was replaced with later key state", replay)
	}
	changed := map[string]any{"name": "Changed request content", "model_ids": []string{}, "budget": input["budget"], "version": 1}
	status, _, _ = f.call(t, "PATCH", path, changed, businessKey)
	if status != http.StatusConflict {
		t.Fatal("same key accepted changed configuration", status)
	}
	var saved usage.Key
	if err := f.a.DB.First(&saved, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Version != 3 || saved.Status != "disabled" || saved.Name != "Updated application" {
		t.Fatal("replay or conflicting retry mutated current key", saved)
	}
	status, _, _ = f.call(t, "PATCH", path, input, uuid.NewString())
	if status != http.StatusConflict {
		t.Fatal("a new operation bypassed version checking", status)
	}
}

func TestPostgresCustomerKeyStateReplaysWithoutUndoingLaterActions(t *testing.T) {
	f, created := customerKeyFixture(t)
	id := created["id"].(string)
	path := "/v1/me/keys/" + id
	disableKey := uuid.NewString()
	status, disabled, _ := f.call(t, "POST", path+"/disable", map[string]any{"version": 1}, disableKey)
	mustOK(t, status, disabled)
	status, enabled, _ := f.call(t, "POST", path+"/enable", map[string]any{"version": 2}, "")
	mustOK(t, status, enabled)
	if enabled["status"] != "active" || enabled["version"] != float64(3) {
		t.Fatal(enabled)
	}
	status, replay, _ := f.call(t, "POST", path+"/disable", map[string]any{"version": 1}, disableKey)
	mustOK(t, status, replay)
	if !reflect.DeepEqual(disabled, replay) {
		t.Fatal("disable retry did not replay the original response", disabled, replay)
	}
	status, _, _ = f.call(t, "POST", path+"/disable", map[string]any{"version": 3}, disableKey)
	if status != http.StatusConflict {
		t.Fatal("state retry accepted a changed version", status)
	}
	var saved usage.Key
	if err := f.a.DB.First(&saved, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != "active" || saved.Version != 3 {
		t.Fatal("an old disable operation disabled the newly enabled key", saved)
	}
	deleteKey := uuid.NewString()
	status, deleted, _ := f.call(t, "DELETE", path, map[string]any{"version": 3}, deleteKey)
	mustOK(t, status, deleted)
	status, replay, _ = f.call(t, "DELETE", path, map[string]any{"version": 3}, deleteKey)
	mustOK(t, status, replay)
	if !reflect.DeepEqual(deleted, replay) || replay["status"] != "deleted" || replay["version"] != float64(4) {
		t.Fatal("delete retry changed the original operation", deleted, replay)
	}
}

func TestPostgresCustomerKeyMutationRollsBackWhenReplayCannotBeSaved(t *testing.T) {
	for _, action := range []string{"configuration", "state"} {
		t.Run(action, func(t *testing.T) {
			f, created := customerKeyFixture(t)
			id := created["id"].(string)
			path, method := "/v1/me/keys/"+id, "PATCH"
			input := map[string]any{"name": "Atomic configuration", "budget": map[string]any{"limit_atg": "5.000000000000000001", "period": "month", "timezone": "UTC"}, "version": 1}
			if action == "state" {
				path, method = path+"/disable", "POST"
				input = map[string]any{"version": 1}
			}
			callback := "test:key-replay-storage-failure"
			if err := f.a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "portal_commands" {
					tx.AddError(errors.New("synthetic replay storage failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.a.DB.Callback().Create().Remove(callback) })
			businessKey := uuid.NewString()
			status, _, _ := f.call(t, method, path, input, businessKey)
			if status != http.StatusInternalServerError {
				t.Fatal("synthetic replay failure was not returned", status)
			}
			var saved usage.Key
			if err := f.a.DB.First(&saved, "id=?", id).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Version != 1 || saved.Status != "active" || saved.Name != "Original application" {
				t.Fatal("key mutation committed without its replay response", saved)
			}
			if err := f.a.DB.Callback().Create().Remove(callback); err != nil {
				t.Fatal(err)
			}
			status, retried, _ := f.call(t, method, path, input, businessKey)
			mustOK(t, status, retried)
			if retried["version"] != float64(2) {
				t.Fatal("recovered command did not apply exactly once", retried)
			}
		})
	}
}
