package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func invoiceProcurementOrder(t *testing.T, f *testApp, supplier, buyer string, amount int64) cooperation.PurchaseOrder {
	t.Helper()
	ctx, owner := context.Background(), f.a.Installation.OwnerStaffID
	quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: supplier, BuyerID: buyer, Name: "Invoice procurement", GiftTiers: []cooperation.PurchaseGiftTier{{FromMoney: ledger.Zero(), GiftPercent: ledger.Zero()}}, ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":false,"method":"non_refundable","terms_reference":"test terms"}`), TermsReference: "test terms", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.a.Cooperation.CreatePurchaseOrder(ctx, nil, owner, buyer, quote.ID, uuid.NewString(), ledger.FromInt64(amount))
	if err != nil {
		t.Fatal(err)
	}
	zero, now := ledger.Zero(), time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: supplier, ProviderReceiptID: uuid.NewString(), ValuationEvidence: "USD", Network: "cash", GrossMoney: order.DueMoney, NetMoney: &order.DueMoney, FeeMoney: &zero, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: order.DueMoney.String(), PaidAt: &now, ObservedAt: now, SettledAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Cooperation.ApplyVerifiedPurchaseReceipt(ctx, nil, f.a.Commerce, order.ID, receipt.ID, uuid.NewString(), order.DueMoney); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Cooperation.FulfillPurchase(ctx, nil, order.ID, owner); err != nil {
		t.Fatal(err)
	}
	return order
}

func TestInvoiceWorkspaceRolesAndProcurementApplications(t *testing.T) {
	f := setupPortal(t)
	f.a.Operations.InvoiceStore = &invoiceWorkspaceBlobs{data: map[string][]byte{}}
	operators := riskOperators(t, f)
	platform, oem, channel := operators[organization.Platform], operators[organization.OEM], operators[organization.Channel]
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for kind, operator := range operators {
		base := "/v1/operators/" + operator
		code, page, _ := f.call(t, "GET", base+"/invoices", nil, "")
		mustOK(t, code, page)
		count := len(page["operations"].([]any))
		if count != 1 || page["operations"].([]any)[0].(map[string]any)["name"] != "instructions" {
			t.Fatal(kind, page)
		}
		code, _, _ = f.call(t, "GET", base+"/procurement-invoices", nil, "")
		if kind == organization.Platform {
			if code != 404 {
				t.Fatal(code)
			}
			code, _, _ = f.call(t, "POST", base+"/procurement-invoices", map[string]any{}, uuid.NewString())
			if code != 404 {
				t.Fatal(code)
			}
		} else if code != 200 {
			t.Fatal(code)
		}
	}
	first := invoiceProcurementOrder(t, f, platform, oem, 1000)
	second := invoiceProcurementOrder(t, f, oem, channel, 40)
	for _, order := range []cooperation.PurchaseOrder{first, second} {
		buyerBase := "/v1/operators/" + order.BuyerID + "/procurement-invoices"
		issuerBase := "/v1/operators/" + order.SupplierID + "/invoices"
		code, page, _ := f.call(t, "GET", buyerBase, nil, "")
		mustOK(t, code, page)
		if len(page["operations"].([]any)) != 1 {
			t.Fatal("application requires setup", page)
		}
		for _, raw := range page["operations"].([]any)[0].(map[string]any)["fields"].([]any) {
			if raw.(map[string]any)["name"] == "issuer_id" {
				t.Fatal("procurement form submits an unsupported customer issuer field")
			}
		}
		code, choices, _ := f.call(t, "GET", "/v1/operators/"+order.BuyerID+"/references/invoice-orders", nil, "")
		mustOK(t, code, choices)
		if len(choices["items"].([]any)) != 1 {
			t.Fatal("eligible purchase missing without instructions", choices)
		}
		code, created, _ := f.call(t, "POST", buyerBase, map[string]any{"supplier_id": order.SupplierID, "lines": []map[string]string{{"order_id": order.ID}}, "details": map[string]string{"title": "Procurement buyer", "tax_id": "TEST-123", "email": "billing@example.test"}}, uuid.NewString())
		mustOK(t, code, created)
		id := created["id"].(string)
		for _, base := range []string{buyerBase, issuerBase} {
			code, application, _ := f.call(t, "GET", base+"/"+id+"/files", nil, "")
			mustOK(t, code, application)
			orders := application["orders"].([]any)
			if len(orders) != 1 || orders[0].(map[string]any)["order_id"] != order.ID || orders[0].(map[string]any)["amount_money"] != order.DueMoney.String() {
				t.Fatal(application)
			}
			if application["details"].(map[string]any)["title"] != "Procurement buyer" {
				t.Fatal(application)
			}
			if base == buyerBase && application["files_available"] != false {
				t.Fatal("unissued document exposed", application)
			}
		}
		code, page, _ = f.call(t, "GET", issuerBase+"?q=Risk", nil, "")
		mustOK(t, code, page)
		row := page["items"].([]any)[0].(map[string]any)
		v := row["values"].(map[string]any)
		if !strings.Contains(v["applicant"].(string), "Risk") || v["applicant_kind"] != "经营方采购" || strings.Contains(row["presentation"].(map[string]any)["title"].(string), "客户发票") {
			t.Fatal(row)
		}
		// Rejection requires no made-up invoice number, file or proof.
		code, result, _ := f.call(t, "POST", issuerBase+"/"+id+"/register", map[string]any{"state": "rejected", "reason": "Correct the title"}, uuid.NewString())
		mustOK(t, code, result)
		code, note, _ := f.call(t, "POST", issuerBase, map[string]any{"instructions": "Within three working days", "expected_version": 0}, uuid.NewString())
		mustOK(t, code, note)
		// Rejected orders can be requested again and issued using an actual file.
		code, created, _ = f.call(t, "POST", buyerBase, map[string]any{"supplier_id": order.SupplierID, "lines": []map[string]string{{"order_id": order.ID}}, "details": map[string]string{"title": "Corrected buyer", "tax_id": "TEST-123", "email": "billing@example.test"}}, uuid.NewString())
		mustOK(t, code, created)
		issuedID := created["id"].(string)
		fileID := uploadWorkspaceInvoice(t, f, issuerBase+"/"+issuedID+"/files")
		code, application, _ := f.call(t, "GET", buyerBase+"/"+issuedID+"/files", nil, "")
		mustOK(t, code, application)
		if application["instructions"] != "Within three working days" {
			t.Fatal("missing instructions snapshot", application)
		}
		if len(application["items"].([]any)) != 0 {
			t.Fatal("draft file visible", application)
		}
		downloadPath := buyerBase + "/" + issuedID + "/files/" + fileID
		code, _, _ = f.call(t, "GET", downloadPath, nil, "")
		if code != 409 {
			t.Fatal("draft download permitted", code)
		}
		code, result, _ = f.call(t, "POST", issuerBase+"/"+issuedID+"/register", map[string]any{"state": "issued", "invoice_reference": "INV-TEST-001"}, uuid.NewString())
		mustOK(t, code, result)
		response, err := f.client.Get(f.server.URL + downloadPath)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || !bytes.Equal(data, workspaceInvoicePDF()) {
			t.Fatal("issued original unavailable", response.StatusCode, err)
		}
		code, result, _ = f.call(t, "POST", issuerBase+"/"+issuedID+"/register", map[string]any{"state": "voided", "reason": "Correct the title"}, uuid.NewString())
		mustOK(t, code, result)
		code, result, _ = f.call(t, "POST", buyerBase+"/"+issuedID+"/replacement", map[string]any{"details": map[string]string{"title": "Replacement buyer", "tax_id": "TEST-123", "email": "billing@example.test"}}, uuid.NewString())
		mustOK(t, code, result)
		replacement, err := f.a.Operations.ReadInvoiceApplication(t.Context(), f.a.Installation.OwnerStaffID, order.BuyerID, result["id"].(string), false, order.BuyerID)
		if err != nil || len(replacement.Orders) != 1 || replacement.Orders[0].OrderID != order.ID || replacement.Orders[0].AmountMoney.Cmp(order.DueMoney) != 0 {
			t.Fatal(replacement, err)
		}
		code, _, _ = f.call(t, "GET", "/v1/operators/"+channel+"/invoices/"+id+"/files", nil, "")
		if code != 403 {
			t.Fatal("unrelated issuer saw application", code)
		}
	}
}

func TestInvoiceApplicationDetailsAndSearchStayScoped(t *testing.T) {
	f, customer, orders := pooledInvoiceFixture(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	invoice, err := f.a.Operations.RequestInvoice(t.Context(), customer, op, uuid.NewString(), []operations.InvoiceLine{{OrderID: orders[0].ID}}, operations.InvoiceDetails{Title: "Private buyer", TaxID: "TEST", Email: "billing@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "customer", "pooled-invoice@example.test", "customer-password-test")
	code, application, _ := f.call(t, "GET", "/v1/me/invoices/"+invoice.ID+"/files", nil, "")
	mustOK(t, code, application)
	if application["files_available"] != false || application["orders"].([]any)[0].(map[string]any)["name"] != "Purchase A" {
		t.Fatal(application)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	// Over 50 newer rows cannot hide a name/email match behind the first page.
	for i := 0; i < 55; i++ {
		row := invoice
		row.ID = uuid.NewString()
		row.BusinessKey = uuid.NewString()
		row.CustomerID = uuid.NewString()
		row.CreatedAt = time.Now().UTC()
		if err = f.a.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	code, page, _ := f.call(t, "GET", "/v1/operators/"+op+"/invoices?q=pooled-invoice%40example.test", nil, "")
	mustOK(t, code, page)
	if len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["id"] != invoice.ID {
		t.Fatal(page)
	}
	if _, err = f.a.Operations.ReadInvoiceApplication(t.Context(), uuid.NewString(), op, invoice.ID, true, ""); err == nil {
		t.Fatal("another customer read private details")
	}
	if _, err = f.a.Operations.ReadInvoiceApplication(t.Context(), owner, uuid.NewString(), invoice.ID, false, ""); err == nil {
		t.Fatal("wrong issuer read private details")
	}
}

// Isolated private object storage. HTTP upload, encryption and downloads run
// through the real application and domain services.
type invoiceWorkspaceBlobs struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (b *invoiceWorkspaceBlobs) Put(_ context.Context, key string, data []byte, _ string, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = append([]byte{}, data...)
	return nil
}
func (b *invoiceWorkspaceBlobs) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.data[key]
	if !ok {
		return nil, operations.ErrNotReady
	}
	return append([]byte{}, data...), nil
}
func (b *invoiceWorkspaceBlobs) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, key)
	return nil
}
func workspaceInvoicePDF() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type/Pages/Count 0/Kids[]>>\nendobj\ntrailer\n<</Root 1 0 R>>\n%%EOF\n")
}
func uploadWorkspaceInvoice(t *testing.T, f *testApp, path string) string {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="invoice.pdf"`}, "Content-Type": {"application/pdf"}})
	if err != nil {
		t.Fatal(err)
	}
	part.Write(workspaceInvoicePDF())
	writer.Close()
	req, err := http.NewRequest("POST", f.server.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", f.csrf)
	req.Header.Set("Idempotency-Key", uuid.NewString())
	req.Header.Set("Origin", f.server.URL)
	response, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	mustOK(t, response.StatusCode, result)
	return result["id"].(string)
}
