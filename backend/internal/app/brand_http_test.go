package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

func TestOEMBrandSelfService(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL and TOKENHUB_REDIS_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "brand_admin_token"
	cfg.BootstrapUser = "brand_user_token"
	cfg.BootstrapChannel = "brand_channel_token"
	cfg.AllowDemoProbes = true
	cfg.Env = "test"
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	application := app.New(cfg, gdb, rdb, logx.New("error", os.Stdout))
	if err := application.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := application.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	requireObjectStore(t, application)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	created := postJSONRaw(t, server.URL+"/admin/brands", "brand_admin_token", map[string]any{
		"name": "Acme API", "primary_domain": "acme-" + suffix + ".localhost",
		"api_domain": "api-acme-" + suffix + ".localhost", "admin_domain": "admin-acme-" + suffix + ".localhost",
		"theme": map[string]string{"brand": "#92400E", "brand_emphasis": "#92400E", "brand_emphasis_dark": "#F0B27A"},
	})
	brand := created["item"].(map[string]any)
	brandID := brand["id"].(string)
	host := brand["primary_domain"].(string)

	channel := postJSONRaw(t, server.URL+"/admin/channels", "brand_admin_token", map[string]any{
		"code": "oem-" + suffix, "type": "C", "brand_id": brandID,
	})
	if channel["item"].(map[string]any)["brand_id"] != brandID {
		t.Fatalf("channel brand: %+v", channel)
	}

	public := getAuthJSON(t, server.URL+"/v1/public/brand?host="+host, "")
	if public["brand"].(map[string]any)["name"] != "Acme API" {
		t.Fatalf("public brand: %+v", public)
	}
	models := getAuthJSON(t, server.URL+"/v1/public/models?host="+host, "")
	if models["brand_id"] != brandID {
		t.Fatalf("models should bind new brand: %+v", models)
	}

	bCode, bBody := doJSON(t, http.MethodPatch, server.URL+"/channel/brand", "brand_channel_token", false, map[string]string{"name": "Nope"})
	if bCode != http.StatusForbidden {
		t.Fatalf("B channel patch: %d %+v", bCode, bBody)
	}

	cLogin := postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": "channel.c@tokenhub.local", "password": identity.BootstrapPassword,
	})
	cToken := cLogin["session"].(map[string]any)["token"].(string)
	mine := getAuthJSON(t, server.URL+"/channel/brand", cToken)
	if mine["customizable"] != true {
		t.Fatalf("C brand should be customizable: %+v", mine)
	}
	updated := patchJSONRaw(t, server.URL+"/channel/brand", cToken, map[string]any{
		"theme": map[string]string{"brand": "#92400E", "brand_emphasis": "#92400E", "brand_emphasis_dark": "#F0B27A"},
	})
	if updated["item"].(map[string]any)["id"] != identity.OEMBrandID {
		t.Fatalf("C patch: %+v", updated)
	}

	logo := pngBytes(t, 128, 128)
	uploaded := postBrandFile(t, server.URL+"/channel/brand/assets", cToken, "logo", "logo.png", logo)
	url, _ := uploaded["item"].(map[string]any)["url"].(string)
	if url == "" {
		t.Fatalf("upload: %+v", uploaded)
	}
	assetID := uploaded["item"].(map[string]any)["id"].(string)
	res, err := http.Get(server.URL + url)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("public asset %s: %v %v", url, err, res)
	}
	_ = res.Body.Close()

	tooBig := bytes.Repeat([]byte("x"), identity.LogoMaxBytes+1)
	code, body := postBrandFileStatus(t, server.URL+"/channel/brand/assets", cToken, "logo", "big.bin", tooBig)
	if code != http.StatusBadRequest {
		t.Fatalf("oversize: %d %+v", code, body)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "asset_too_large" {
		t.Fatalf("oversize code: %+v", body)
	}

	tiny := pngBytes(t, 32, 32)
	code, body = postBrandFileStatus(t, server.URL+"/channel/brand/assets", cToken, "logo", "tiny.png", tiny)
	if code != http.StatusBadRequest {
		t.Fatalf("tiny: %d %+v", code, body)
	}

	again := postBrandFile(t, server.URL+"/channel/brand/assets", cToken, "logo", "logo2.png", pngBytes(t, 256, 256))
	old, err := http.Get(server.URL + "/v1/public/brand-assets/" + assetID)
	if err != nil {
		t.Fatal(err)
	}
	if old.StatusCode != http.StatusNotFound {
		t.Fatalf("superseded asset should 404, got %d", old.StatusCode)
	}
	_ = old.Body.Close()
	if again["item"].(map[string]any)["id"] == assetID {
		t.Fatal("second upload must mint a new id")
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postBrandFile(t *testing.T, url, token, kind, name string, data []byte) map[string]any {
	t.Helper()
	code, body := postBrandFileStatus(t, url, token, kind, name, data)
	if code >= 300 {
		t.Fatalf("upload %s %d %+v", url, code, body)
	}
	return body
}

func postBrandFileStatus(t *testing.T, url, token, kind, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("kind", kind)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = jsonDecode(raw, &out)
	return resp.StatusCode, out
}

func jsonDecode(raw []byte, dest any) error {
	return json.NewDecoder(bytes.NewReader(raw)).Decode(dest)
}
