package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestPostgresTakeoverHTTPPreparationAndSnapshotReview(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	f.openRegistration(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + f.a.Installation.OperatorID
	child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: f.a.Installation.OperatorID, Kind: organization.Channel, Name: "HTTP Channel", LegalName: "Channel Legal", QuoteReference: "supply terms", ExitTermsReference: "exit terms"})
	if e != nil {
		t.Fatal(e)
	}
	status, out, _ := f.call(t, "GET", scope+"/takeovers", nil, "")
	mustOK(t, status, out)
	if len(out["channels"].([]any)) != 1 {
		t.Fatal("channel selector unavailable", out)
	}
	action := uuid.NewString()
	input := map[string]any{"channel_id": child.Operator.ID, "method": "supply_coverage", "service_usd_per_atg": "1", "procurement_usd_per_atg": "1", "extra_usd": "0", "contract_reference": "signed no-asset takeover", "evidence": "explicit brand decision"}
	status, out, _ = f.call(t, "POST", scope+"/takeovers", input, action)
	mustOK(t, status, out)
	id := out["id"].(string)
	if out["state"] != "preparing" {
		t.Fatal(out)
	}
	status, retry, _ := f.call(t, "POST", scope+"/takeovers", input, action)
	mustOK(t, status, retry)
	if retry["id"] != id {
		t.Fatal("takeover duplicated on response retry", retry)
	}
	status, out, _ = f.call(t, "POST", scope+"/takeovers/"+id+"/execute", map[string]any{"expected_snapshot": "unreviewed"}, "")
	if status == 200 {
		t.Fatal("executed without reviewed complete snapshot")
	}
	status, out, _ = f.call(t, "POST", scope+"/takeovers/"+id+"/cancel", map[string]any{"evidence": "cancel preparation while reviewing contract"}, "")
	mustOK(t, status, out)
	gates, e := f.a.Organization.GetBusinessStatus(ctx, nil, child.Operator.ID)
	if e != nil {
		t.Fatal(e)
	}
	if gates.Registration || gates.NewPurchase || gates.ModelUsage {
		t.Fatal("cancel silently reopened channel", gates)
	}
	status, out, _ = f.call(t, "GET", scope+"/takeover-settlements", nil, "")
	mustOK(t, status, out)
}
func TestPostgresCaddyTLSAskOnlyConfiguredAndVerifiedDomains(t *testing.T) {
	f := setupPortal(t)
	for host, wanted := range map[string]int{"127.0.0.1": 403, "unknown.example.test": 403, "bad/user@example.test": 403} {
		response, e := f.client.Get(f.server.URL + "/v1/public/tls-check?domain=" + host)
		if e != nil {
			t.Fatal(e)
		}
		status := response.StatusCode
		response.Body.Close()
		if status != wanted {
			t.Fatal(host, status)
		}
	}
	f.a.Config.OfficialHost = "official.example.test"
	response, e := f.client.Get(f.server.URL + "/v1/public/tls-check?domain=official.example.test")
	if e != nil {
		t.Fatal(e)
	}
	status := response.StatusCode
	response.Body.Close()
	if status != 200 {
		t.Fatal("configured official hostname refused", status)
	}
}

func TestExternalPrivateFinanceEvidenceUsesEncryptedS3AndExactScope(t *testing.T) {
	if os.Getenv("TOKENHUB_TEST_S3_BUCKET") == "" {
		t.Skip("requires private isolated S3")
	}
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + f.a.Installation.OperatorID
	key := uuid.NewString()
	data := []byte("%PDF-1.4\nprivate bank transfer account 123456\n%%EOF")
	upload := func(contents []byte) (int, map[string]any) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("file", "bank-proof.pdf")
		if e != nil {
			t.Fatal(e)
		}
		part.Write(contents)
		writer.Close()
		req, e := http.NewRequest("POST", f.server.URL+scope+"/evidence", &body)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Origin", f.server.URL)
		req.Header.Set("X-CSRF-Token", f.csrf)
		req.Header.Set("Idempotency-Key", key)
		response, e := f.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var out map[string]any
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, out
	}
	status, out := upload(data)
	mustOK(t, status, out)
	id := out["id"].(string)
	status, retry := upload(data)
	mustOK(t, status, retry)
	if retry["id"] != id {
		t.Fatal("upload response retry duplicated evidence")
	}
	status, out = upload([]byte("%PDF-1.4\nchanged payment\n%%EOF"))
	if status != 409 {
		t.Fatal("changed evidence accepted with same command", status, out)
	}
	var stored financeEvidence
	if e := f.a.DB.First(&stored, "id=?", id).Error; e != nil {
		t.Fatal(e)
	}
	cipher, e := f.a.ObjectStore.Get(context.Background(), stored.ObjectKey)
	if e != nil || bytes.Contains(cipher, []byte("123456")) {
		t.Fatal("bank evidence not encrypted in S3", e)
	}
	response, e := f.client.Get(f.server.URL + scope + "/evidence/" + id + "/content")
	if e != nil {
		t.Fatal(e)
	}
	actual, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !bytes.Equal(actual, data) || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("authorized private download differs", response.StatusCode)
	}
	response, e = f.client.Get(f.server.URL + "/v1/operators/" + uuid.NewString() + "/evidence/" + id + "/content")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("evidence escaped exact operator scope")
	}
}
