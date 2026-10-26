package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresPrivateCooperationContractUploadAndPartyBoundaries(t *testing.T) {
	f := setupPortal(t)
	if f.a.ObjectStore == nil {
		t.Skip("private S3 integration requires configured test object store")
	}
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	payload := []byte("%PDF-1.4\nprivate cooperation agreement\n%%EOF\n")
	upload := func(operator, idempotencyKey string, data []byte) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, e := writer.CreateFormFile("file", "signed-contract.pdf")
		if e != nil {
			t.Fatal(e)
		}
		_, _ = part.Write(data)
		_ = writer.Close()
		req, e := http.NewRequest("POST", f.server.URL+"/v1/operators/"+operator+"/contracts", &buf)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Idempotency-Key", idempotencyKey)
		req.Header.Set("X-CSRF-Token", f.csrf)
		req.Header.Set("Origin", f.server.URL)
		res, e := f.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var out map[string]any
		if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, out
	}
	key := uuid.NewString()
	status, out := upload(parent, key, payload)
	mustOK(t, status, out)
	id := out["id"].(string)
	status, out = upload(parent, key, payload)
	mustOK(t, status, out)
	if out["id"] != id {
		t.Fatal("same upload retry duplicated the contract", out)
	}
	status, out = upload(parent, key, []byte("%PDF-1.4\nchanged contract\n%%EOF"))
	if status != 409 {
		t.Fatal("changed upload reused same key", status, out)
	}
	status, out = upload(parent, uuid.NewString(), []byte("<html>executable upload</html>"))
	if status != 400 {
		t.Fatal("active content accepted as a contract", status, out)
	}
	row := cooperationContract{}
	if e := f.a.DB.Where("id=?", id).First(&row).Error; e != nil {
		t.Fatal(e)
	}
	cipher, e := f.a.ObjectStore.Get(ctx, row.ObjectKey)
	if e != nil || bytes.Contains(cipher, payload) {
		t.Fatal("contract was not privately encrypted", e)
	}
	child := func(name, attachment string) organization.OperatorCreated {
		t.Helper()
		v, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.Channel, Name: name, ContractAttachmentID: attachment})
		if e != nil {
			t.Fatal(e)
		}
		inv, e := f.a.Organization.InviteStaff(ctx, nil, owner, v.Operator.ID, name+"@example.test", "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: name + "@example.test", EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		return v
	}
	buyer := child("contract-buyer", id)
	sibling := child("contract-unrelated", "")
	download := func(operator string) (int, []byte, http.Header) {
		t.Helper()
		res, e := f.client.Get(f.server.URL + "/v1/operators/" + operator + "/contracts/" + id + "/content")
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, data, res.Header
	}
	for _, operator := range []string{parent, buyer.Operator.ID} {
		status, data, headers := download(operator)
		if status != 200 || !bytes.Equal(data, payload) || !strings.Contains(headers.Get("Cache-Control"), "no-store") {
			t.Fatal("contract party cannot access original document safely", status, string(data))
		}
	}
	status, data, _ := download(sibling.Operator.ID)
	if status != 404 {
		t.Fatal("unrelated buyer read private contract", status, string(data))
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+parent+"/procurement", map[string]any{"action": "quote", "buyer_id": sibling.Operator.ID, "gift_tiers": []map[string]string{{"from_usd": "0", "gift_percent": "900"}}, "contract_attachment_id": id, "exit_policy": "remaining_paid_proportional", "enabled": true}, "")
	mustOK(t, status, out)
	status, data, _ = download(sibling.Operator.ID)
	if status != 200 || !bytes.Equal(data, payload) {
		t.Fatal("buyer of attached quote cannot read agreed contract", status)
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+buyer.Operator.ID+"/references/contracts", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 0 {
		t.Fatal("buyer can choose an upstream document as its own supply attachment", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+buyer.Operator.ID+"/contracts", nil, "")
	mustOK(t, status, out)
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), row.ObjectKey) || strings.Contains(string(encoded), row.Fingerprint) || !strings.Contains(string(encoded), "signed-contract.pdf") {
		t.Fatal("private storage metadata leaked or authorized contract missing", string(encoded))
	}
	// A buyer may read a supplier's document, but cannot attach it as its own
	// supply asset, even when the same staff identity works in both scopes.
	if e = f.a.Organization.RequireSupplyContract(ctx, nil, buyer.Operator.ID, id); e != organization.ErrForbidden {
		t.Fatal("view permission became attachment ownership", e)
	}
	if e = f.a.ObjectStore.Put(ctx, row.ObjectKey, []byte("tampered ciphertext"), "application/octet-stream", row.CreatedAt); e != nil {
		t.Fatal(e)
	}
	status, data, _ = download(parent)
	if status == 200 || bytes.Contains(data, payload) {
		t.Fatal("tampered contract was served", status, string(data))
	}
}

func TestPostgresReadableCooperationFormsDoNotRequireInternalReferences(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	parent := f.a.Installation.OperatorID
	for _, resource := range []string{"operators", "procurement", "plans"} {
		status, out, _ := f.call(t, "GET", "/v1/operators/"+parent+"/"+resource, nil, "")
		mustOK(t, status, out)
		for _, group := range []string{"operations", "row_operations"} {
			for _, value := range out[group].([]any) {
				op := value.(map[string]any)
				for _, value := range op["fields"].([]any) {
					field := value.(map[string]any)
					name := field["name"].(string)
					if (resource == "operators" && name == "exit_policy") || name == "api_base_url" || name == "quote_reference" || name == "exit_terms_reference" || name == "terms_reference" || strings.HasPrefix(name, "exit_terms.") {
						t.Fatal("business form exposes a technical term reference", resource, field)
					}
					if (name == "action" || name == "version" || name == "expected_version") && field["kind"] != "hidden" {
						t.Fatal("business form exposes internal command fields", resource, field)
					}
				}
			}
		}
	}
	status, out, _ := f.call(t, "POST", "/v1/operators/"+parent+"/operators", map[string]any{"kind": "channel", "name": "Simple partner", "legal_name": "Partner company", "supply_terms": "按已确认报价采购"}, "")
	mustOK(t, status, out)
	agreement, e := f.a.Organization.GetSupplyAgreement(context.Background(), nil, out["id"].(string))
	if e != nil || agreement.SupplyTerms != "按已确认报价采购" {
		t.Fatal("form business rules were lost", agreement, e)
	}
}
