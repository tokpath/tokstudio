package app_test

import (
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestDeprecateModelWithoutProviderOrPrices(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	cfg.BootstrapAdmin = "deprecate_admin"
	cfg.BootstrapUser = "deprecate_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	s := httptest.NewServer(a.Router())
	defer s.Close()
	publicID := "test/deprecate-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	postJSONRaw(t, s.URL+"/admin/models", cfg.BootstrapAdmin, map[string]any{"public_id": publicID, "vendor": "tokenhub", "display_name": "Unconfigured local model"})
	for i := 0; i < 2; i++ {
		body := postJSONRaw(t, s.URL+"/admin/models/deprecate", cfg.BootstrapAdmin, map[string]any{"public_id": publicID})
		if body["item"].(map[string]any)["status"] != "deprecated" {
			t.Fatalf("unexpected result %+v", body)
		}
	}
}
