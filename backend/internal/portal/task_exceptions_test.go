package portal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
)

func (f *testApp) taskFixture(t *testing.T, queue, kind, key, status string, exhausted bool, at time.Time, payload string) jobs.Job {
	t.Helper()
	j, err := f.a.Jobs.Enqueue(t.Context(), jobs.Input{Queue: queue, Kind: kind, BusinessKey: key, Payload: json.RawMessage(payload), MaxAttempts: 12})
	if err != nil {
		t.Fatal(err)
	}
	updates := map[string]any{"status": status, "created_at": at, "updated_at": at}
	if exhausted {
		updates["exhausted_at"], updates["attempts"], updates["last_error"] = at, 12, kind+": dependency_or_state_requires_review"
	}
	if err = f.a.DB.Model(&jobs.Job{}).Where("id=?", j.ID).Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.First(&j, "id=?", j.ID).Error; err != nil {
		t.Fatal(err)
	}
	return j
}

func (f *testApp) taskScheduleFixture(t *testing.T, subject string, job jobs.Job, payload string) {
	t.Helper()
	if err := f.a.DB.Create(&scheduledTask{ID: job.Kind + ":" + subject, Kind: job.Kind, Payload: json.RawMessage(payload), LastJobID: &job.ID, NextDueAt: time.Now().Add(time.Hour), CreatedAt: job.CreatedAt}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTaskExceptionsGroupCurrentFailuresAndGuardRecovery(t *testing.T) {
	f := setupPortal(t)
	ctx, now := t.Context(), time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	// Successful later cycles hide old failures, independently for each subject.
	old := f.taskFixture(t, "portal-maintenance", "retention", "retention:recovered:1", "dead", true, now, `{}`)
	done := f.taskFixture(t, "portal-maintenance", "retention", "retention:recovered:2", "completed", false, now.Add(time.Minute), `{}`)
	f.taskScheduleFixture(t, "recovered", done, `{}`)
	f.taskFixture(t, "portal-maintenance", "retention", "retention:broken:1", "dead", true, now, `{}`)
	latest := f.taskFixture(t, "portal-maintenance", "retention", "retention:broken:2", "dead", true, now.Add(2*time.Minute), `{}`)
	f.taskScheduleFixture(t, "broken", latest, `{}`)
	f.taskFixture(t, "portal-maintenance", "retention", "retention:retrying:1", "dead", true, now.Add(3*time.Minute), `{}`)
	retrying := f.taskFixture(t, "portal-maintenance", "retention", "retention:retrying:2", "pending", false, now.Add(4*time.Minute), `{}`)
	f.taskScheduleFixture(t, "retrying", retrying, `{}`)
	// Separate one-off work must never be collapsed just because the kind matches.
	one := f.taskFixture(t, "portal-price-check", "coverage", "price-one", "dead", true, now.Add(5*time.Minute), `{"model_id":"public-model-one"}`)
	f.taskFixture(t, "portal-price-check", "coverage", "price-two", "dead", true, now.Add(5*time.Minute), `{"model_id":"public-model-two"}`)
	f.taskFixture(t, "portal-price-check", "coverage", "completed", "completed", true, now.Add(6*time.Minute), `{}`)
	f.taskFixture(t, "portal-mail", "verification", "normal-retry", "pending", false, now.Add(7*time.Minute), `{}`)
	if err := f.a.DB.Model(&jobs.Job{}).Where("id=?", one.ID).Update("last_error", "secret-provider-response-must-not-leak").Error; err != nil {
		t.Fatal(err)
	}
	rows, err := f.a.taskExceptions(ctx, "", 100)
	if err != nil || len(rows) != 4 {
		t.Fatal(len(rows), err)
	}
	if rows[2].CurrentJobID != retrying.ID || rows[3].ID != latest.ID {
		t.Fatal("wrong grouping or failed-time sort", rows)
	}
	raw, _ := json.Marshal(taskCursor{At: *rows[1].ExhaustedAt, ID: rows[1].ID})
	next, err := f.a.taskExceptions(ctx, base64.RawURLEncoding.EncodeToString(raw), 2)
	if err != nil || len(next) != 2 || next[0].ID != rows[2].ID || next[1].ID != rows[3].ID {
		t.Fatal("unstable exception cursor", next, err)
	}
	if _, err = f.a.taskExceptions(ctx, "bad-cursor", 10); err == nil {
		t.Fatal("accepted invalid cursor")
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + f.a.Installation.OperatorID + "/jobs"
	status, out, _ := f.call(t, "GET", path+"?status=completed", nil, "")
	mustOK(t, status, out)
	encoded, _ := json.Marshal(out)
	if out["title"] != "后台任务异常" || out["status_supported"] != false || len(out["items"].([]any)) != 4 || strings.Contains(string(encoded), "secret-provider") || strings.Contains(string(encoded), "business_key") {
		t.Fatal(out)
	}
	for _, item := range out["items"].([]any) {
		r := item.(map[string]any)
		if r["status"] == "pending" && len(r["allowed_actions"].([]any)) != 0 {
			t.Fatal("offered obsolete recurring recovery", r)
		}
	}
	status, out, _ = f.call(t, "POST", path+"/"+old.ID+"/redrive", map[string]any{}, uuid.NewString())
	if status != 409 {
		t.Fatal("restarted a superseded cycle", status, out)
	}
	key := uuid.NewString()
	for range 2 {
		status, out, _ = f.call(t, "POST", path+"/"+latest.ID+"/redrive", map[string]any{}, key)
		mustOK(t, status, out)
	}
	var restored jobs.Job
	if err = f.a.DB.First(&restored, "id=?", latest.ID).Error; err != nil || restored.MaxAttempts != 15 || restored.Attempts != 12 || restored.ExhaustedAt == nil || restored.Status != "pending" {
		t.Fatal(restored, err)
	}
	rows, err = f.a.taskExceptions(ctx, "", 100)
	if err != nil || len(rows) != 4 {
		t.Fatal("retry disappeared before a successful result", rows, err)
	}
	if err = f.a.DB.Model(&jobs.Job{}).Where("id IN ?", []string{retrying.ID, latest.ID}).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	rows, err = f.a.taskExceptions(ctx, "", 100)
	if err != nil || len(rows) != 2 {
		t.Fatal("success left old failures as pending work", rows, err)
	}
}

func TestPostgresTaskExceptionsRespectScopeAndRetiredSchedules(t *testing.T) {
	f := setupPortal(t)
	roles := f.seedPaymentOperatorRoles(t)
	op := f.a.Installation.OperatorID
	for i, target := range []string{op, roles[0]["operator"]} {
		payload := fmt.Sprintf(`{"operator_id":%q}`, target)
		job := f.taskFixture(t, "portal-maintenance", "risks", "risks:"+target+":1", "dead", true, time.Now().Add(time.Duration(i)*time.Second), payload)
		f.taskScheduleFixture(t, target, job, payload)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/jobs", nil, "")
	mustOK(t, status, out)
	rows := out["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["values"].(map[string]any)["impact_name"] != "Payment oem" {
		t.Fatal("wrong responsibility label or retired platform risk exposed", out)
	}
	id := rows[0].(map[string]any)["id"].(string)
	for _, role := range roles {
		f.login(t, "staff", role["email"], role["password"])
		for _, operator := range []string{op, role["operator"]} {
			status, out, _ = f.call(t, "GET", "/v1/operators/"+operator+"/jobs", nil, "")
			if status != 403 {
				t.Fatal("nonplatform read", status, out)
			}
			status, out, _ = f.call(t, "POST", "/v1/operators/"+operator+"/jobs/"+id+"/redrive", map[string]any{}, uuid.NewString())
			if status != 403 {
				t.Fatal("nonplatform recovery", status, out)
			}
		}
	}
}
