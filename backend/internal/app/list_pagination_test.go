package app_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

func TestMediaHistoryDatabasePagination(t *testing.T) {
	a, server := newOAuthEnv(t)
	ctx := context.Background()
	marker := fmt.Sprintf("media-page-%d", time.Now().UnixNano())
	register := func(name, brand, promotion string) (string, string) {
		s, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: brand, Email: marker + name + "@example.test", Password: "synthetic-password", PromotionCode: promotion})
		if err != nil {
			t.Fatal(err)
		}
		return s.User.ID, s.Token
	}
	userID, token := register("mine", identity.OfficialBrandID, "THA1")
	foreignID, _ := register("foreign", identity.OfficialBrandID, "THB1")
	oemUserID, _ := register("oem", identity.OEMBrandID, "THC1")
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seed := func(prefix, user, channel string, count int) {
		err := a.DB.Exec(`INSERT INTO media_jobs (id,request_id,user_id,channel_org_id,public_model_id,job_kind,task_type,status,prompt,provider_id,upstream_job_id,created_at)
		SELECT ? || lpad(n::text,4,'0'), ? || lpad(n::text,4,'0'), ?, ?, 'synthetic/model',
		CASE WHEN n%2=0 THEN 'video' ELSE 'image' END, CASE WHEN n%2=0 THEN 't2v' ELSE 'generate' END,
		CASE WHEN n%3=0 THEN 'completed' ELSE 'failed' END, 'synthetic private prompt', 'synthetic-provider', 'synthetic-upstream', ?::timestamptz+(n/3)*interval '1 second'
		FROM generate_series(0, ?-1) AS n`, prefix, prefix, user, channel, base, count).Error
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(marker+"-mine-", userID, identity.OfficialChannelID, 275)
	seed(marker+"-foreign-", foreignID, identity.ResellerChannelID, 17)
	seed(marker+"-oem-", oemUserID, identity.OEMChannelID, 205)
	t.Cleanup(func() { a.DB.Exec("DELETE FROM media_jobs WHERE id LIKE ?", marker+"%") })

	walk := func(path, auth string, expected int, prefix string, private bool) {
		seen := map[string]bool{}
		cursor := ""
		for n := 0; n < 40; n++ {
			endpoint := path
			if cursor != "" {
				endpoint += "&cursor=" + url.QueryEscape(cursor)
			}
			body := getAuthJSON(t, server.URL+endpoint, auth)
			items := body["items"].([]any)
			if len(items) == 0 {
				t.Fatalf("unexpected empty page after %d records: %+v", len(seen), body)
			}
			for _, raw := range items {
				row := raw.(map[string]any)
				key := row["id"].(string)
				if seen[key] || !strings.HasPrefix(key, prefix) {
					t.Fatalf("duplicate or foreign record: %+v", row)
				}
				if private && (row["provider"] != nil || row["upstream_job_id"] != nil) {
					t.Fatalf("private supplier data leaked: %+v", row)
				}
				seen[key] = true
			}
			next, _ := body["next_cursor"].(string)
			if next == "" {
				break
			}
			if next == cursor {
				t.Fatal("pagination did not advance")
			}
			cursor = next
		}
		if len(seen) != expected {
			t.Fatalf("read %d records, want %d", len(seen), expected)
		}
	}
	q := url.QueryEscape(marker + "-mine-")
	walk("/v1/me/media?limit=25&q="+q+"&user_id="+foreignID, token, 275, marker+"-mine-", true)
	walk("/admin/media?limit=100&q="+q+"&user_id="+userID, "oauth_admin", 275, marker+"-mine-", false)
	walk("/v1/me/media?limit=17&kind=video&status=completed&q="+q, token, 46, marker+"-mine-", true)
	oemToken := tokenOf(bootstrapLogin(t, server.URL+"/v1/auth/login", "channel.c"))
	walk("/channel/media?limit=37&q="+url.QueryEscape(marker), oemToken, 205, marker+"-oem-", true)
	if items, next, err := a.Media.ListChannelsPage(ctx, nil, marker, 50, ""); err != nil || len(items) != 0 || next != "" {
		t.Fatalf("empty OEM scope widened: %+v %q %v", items, next, err)
	}
	if code := getStatus(t, server.URL+"/channel/media?channel_id="+identity.ResellerChannelID, oemToken); code != 403 {
		t.Fatalf("foreign OEM scope: %d", code)
	}

	export := getBytesAuth(t, server.URL+"/admin/media?format=csv&limit=1&cursor=ignored&q="+q+"&user_id="+userID, "oauth_admin")
	rows, err := csv.NewReader(strings.NewReader(string(export))).ReadAll()
	if err != nil || len(rows) != 276 {
		t.Fatalf("truncated media export: %d rows, %v", len(rows), err)
	}
	if strings.Contains(string(export), "synthetic private prompt") || strings.Contains(string(export), "synthetic-provider") || strings.Contains(string(export), marker+"-foreign-") {
		t.Fatal("unsafe media export")
	}

	first := getAuthJSON(t, server.URL+"/v1/me/media?limit=20&q="+q, token)
	items := first["items"].([]any)
	anchor := items[len(items)-1].(map[string]any)["id"].(string)
	legacy := getAuthJSON(t, server.URL+"/v1/me/media?limit=20&q="+q+"&cursor="+anchor, token)
	if len(legacy["items"].([]any)) != 20 || legacy["items"].([]any)[0].(map[string]any)["id"] == anchor {
		t.Fatal("legacy ID cursor did not advance")
	}
	for _, raw := range []string{"missing", marker + "-foreign-0000", strings.Repeat("x", 2048)} {
		if code := getStatus(t, server.URL+"/v1/me/media?cursor="+url.QueryEscape(raw), token); code != http.StatusBadRequest {
			t.Fatalf("invalid/foreign cursor returned %d", code)
		}
	}
	// A new newest record and deletion of the old anchor must not move the
	// remaining history window or make the opaque cursor depend on that row.
	if err := a.DB.Exec("DELETE FROM media_jobs WHERE id=?", anchor).Error; err != nil {
		t.Fatal(err)
	}
	seed(marker+"-new-", userID, identity.OfficialChannelID, 1)
	if err := a.DB.Exec("UPDATE media_jobs SET created_at=? WHERE id=?", base.Add(24*time.Hour), marker+"-new-0000").Error; err != nil {
		t.Fatal(err)
	}
	remaining := map[string]bool{}
	cursor := first["next_cursor"].(string)
	for cursor != "" {
		body := getAuthJSON(t, server.URL+"/v1/me/media?limit=100&cursor="+url.QueryEscape(cursor), token)
		for _, raw := range body["items"].([]any) {
			key := raw.(map[string]any)["id"].(string)
			if remaining[key] || !strings.HasPrefix(key, marker+"-mine-") {
				t.Fatalf("moving history included new/duplicate record: %s", key)
			}
			remaining[key] = true
		}
		cursor, _ = body["next_cursor"].(string)
	}
	if len(remaining) != 255 {
		t.Fatalf("remaining history truncated after insert/delete: %d", len(remaining))
	}
}

func TestAlertHistoryDatabasePaginationAndCSV(t *testing.T) {
	a, server := newOAuthEnv(t)
	marker := fmt.Sprintf("alert-page-%d", time.Now().UnixNano())
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if err := a.DB.Exec(`INSERT INTO ops_alerts (id,kind,scope_key,severity,status,message,created_at)
	SELECT ? || lpad(n::text,4,'0'), 'circuit_open', ? || n::text, 'medium', CASE WHEN n%2=0 THEN 'open' ELSE 'resolved' END,
	? || n::text || ', quoted "value"' || chr(10) || 'next line', ?::timestamptz+(n/4)*interval '1 second'
	FROM generate_series(0,119) AS n`, marker, marker, marker, base).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.DB.Exec("DELETE FROM ops_alerts WHERE id LIKE ?", marker+"%") })
	for _, filter := range []struct {
		status string
		want   int
	}{{"", 120}, {"open", 60}, {"resolved", 60}} {
		seen := map[string]bool{}
		cursor := ""
		for n := 0; n < 10; n++ {
			body := getAuthJSON(t, server.URL+"/admin/ops/alerts?limit=40&q="+marker+"&status="+filter.status+"&cursor="+url.QueryEscape(cursor), "oauth_admin")
			items := body["items"].([]any)
			if len(items) == 0 {
				t.Fatal("spurious empty terminal alert page")
			}
			for _, raw := range items {
				row := raw.(map[string]any)
				key := row["id"].(string)
				if seen[key] || !strings.HasPrefix(key, marker) || filter.status != "" && row["status"] != filter.status {
					t.Fatalf("duplicate/unfiltered alert: %+v", row)
				}
				seen[key] = true
			}
			next, _ := body["next_cursor"].(string)
			if next == "" {
				break
			}
			if next == cursor {
				t.Fatal("alert cursor did not advance")
			}
			cursor = next
		}
		if len(seen) != filter.want {
			t.Fatalf("alerts: got %d, want %d", len(seen), filter.want)
		}
	}
	first := getAuthJSON(t, server.URL+"/admin/ops/alerts?limit=20&q="+marker, "oauth_admin")
	anchor := first["items"].([]any)[19].(map[string]any)["id"].(string)
	legacy := getAuthJSON(t, server.URL+"/admin/ops/alerts?limit=20&q="+marker+"&cursor="+anchor, "oauth_admin")
	if len(legacy["items"].([]any)) != 20 {
		t.Fatal("legacy alert cursor broke")
	}
	if code := getStatus(t, server.URL+"/admin/ops/alerts?cursor=invalid", "oauth_admin"); code != 400 {
		t.Fatalf("bad alert cursor: %d", code)
	}
	if code := getStatus(t, server.URL+"/admin/ops/alerts", "oauth_user"); code != 403 {
		t.Fatalf("user can read platform alerts: %d", code)
	}
	for _, filter := range []struct {
		status string
		want   int
	}{{"", 121}, {"resolved", 61}} {
		export := getBytesAuth(t, server.URL+"/admin/ops/alerts?format=csv&limit=1&cursor=ignored&q="+marker+"&status="+filter.status, "oauth_admin")
		rows, err := csv.NewReader(strings.NewReader(string(export))).ReadAll()
		if err != nil || len(rows) != filter.want {
			t.Fatalf("alert CSV truncated or malformed: %d %v", len(rows), err)
		}
		if !strings.Contains(rows[1][4], ", quoted \"value\"\nnext line") {
			t.Fatalf("CSV escaping changed message: %q", rows[1][4])
		}
	}
}
