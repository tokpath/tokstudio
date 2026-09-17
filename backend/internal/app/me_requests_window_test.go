package app_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestMeRequestsTimeWindowIncludesEndOfLocalDay(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "req_tz_admin"
	cfg.BootstrapUser = "req_tz_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "reqtz-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	userID := userIDOf(reg)
	endOfDay := time.Date(2026, 9, 16, 15, 59, 59, 0, time.UTC)
	nextMidnight := time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC)
	insertGatewayRequest(t, application.DB, userID, "req_end_of_shanghai_day", endOfDay)
	insertGatewayRequest(t, application.DB, userID, "req_next_shanghai_midnight", nextMidnight)

	listed := getAuthJSON(t, server.URL+"/v1/me/requests?from=2026-09-15T16:00:00Z&to=2026-09-16T16:00:00Z&limit=100", session)
	mustRequest(t, listed, "req_end_of_shanghai_day")
	if containsRequest(listed, "req_next_shanghai_midnight") {
		t.Fatalf("exclusive end must not include next local midnight: %+v", listed)
	}

	utcDay := getAuthJSON(t, server.URL+"/v1/me/requests?from=2026-09-16&to=2026-09-16&limit=100", session)
	mustRequest(t, utcDay, "req_end_of_shanghai_day")
	mustRequest(t, utcDay, "req_next_shanghai_midnight")

	if code, body := doJSON(t, http.MethodGet, server.URL+"/v1/me/requests?from=2026-09-17&to=2026-09-16", session, false, nil); code != http.StatusBadRequest {
		t.Fatalf("reversed dates must 400: %d %+v", code, body)
	}
	if code, body := doJSON(t, http.MethodGet, server.URL+"/v1/me/requests?from=not-a-day", session, false, nil); code != http.StatusBadRequest {
		t.Fatalf("invalid from must 400: %d %+v", code, body)
	}
}

func insertGatewayRequest(t *testing.T, db *gorm.DB, userID, requestID string, started time.Time) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO gateway_requests (id, request_id, user_id, public_model_id, protocol, status, started_at)
		VALUES (?, ?, ?, ?, 'openai.chat', 'succeeded', ?)
	`, "grq_"+requestID, requestID, userID, catalog.EchoModelID, started).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM gateway_requests WHERE request_id = ?`, requestID).Error
	})
}
