package operations

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"os"
	"sync"
	"testing"
	"time"
)

type operationBlobs struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (b *operationBlobs) Put(_ context.Context, key string, data []byte, _ string, _ time.Time) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = append([]byte{}, data...)
	return nil
}
func (b *operationBlobs) Get(_ context.Context, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.data[key]
	if !ok {
		return nil, ErrNotReady
	}
	return append([]byte{}, d...), nil
}
func (b *operationBlobs) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, key)
	return nil
}
func invoicePDFFixture() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type/Pages/Count 0/Kids[]>>\nendobj\ntrailer\n<</Root 1 0 R>>\n%%EOF\n")
}
func TestPrivateInvoiceFilesAndVoidReplacement(t *testing.T) { testPrivateInvoiceFiles(t, false) }
func TestExternalS3PrivateInvoiceFilesAndVoidReplacement(t *testing.T) {
	if os.Getenv("TOKENHUB_TEST_S3_ENDPOINT") == "" {
		t.Skip("real private S3-compatible test bucket required")
	}
	testPrivateInvoiceFiles(t, true)
}
func testPrivateInvoiceFiles(t *testing.T, realS3 bool) {
	s, a, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	if realS3 {
		store, e := objectstore.New(ctx, objectstore.Settings{Endpoint: os.Getenv("TOKENHUB_TEST_S3_ENDPOINT"), Region: "us-east-1", Bucket: os.Getenv("TOKENHUB_TEST_S3_BUCKET"), AccessKey: os.Getenv("TOKENHUB_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TOKENHUB_TEST_S3_SECRET_KEY"), ForcePathStyle: true})
		if e != nil {
			t.Fatal(e)
		}
		s.InvoiceStore = store
	}
	basis := &invoiceBasis{value: ledger.FromInt64(10)}
	s.Invoices = basis
	if _, e := s.SaveInvoicePolicy(ctx, a.actor, InvoicePolicy{OperatorID: operator, Enabled: true, Stage: "receipt", MaximumRatio: ledger.FromInt64(1), RulesReference: "published-invoice-policy"}, 0); e != nil {
		t.Fatal(e)
	}
	receipt := uuid.NewString()
	lines := []InvoiceLine{{receipt, ledger.FromInt64(6)}}
	i, e := s.RequestInvoice(ctx, customer, operator, "invoice-one", lines, InvoiceDetails{Title: "Verified Buyer"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RegisterInvoice(ctx, a.actor, operator, i.ID, "issued", "legal-001", "external-invoice-proof", "verified external issue"); !errors.Is(e, ErrInvoiceDocument) {
		t.Fatalf("reference string stood in for an actual invoice document: %v", e)
	}
	upload := InvoiceUpload{Filename: "legal-invoice.pdf", ContentType: "application/pdf", Data: invoicePDFFixture()}
	f, e := s.UploadInvoiceFile(ctx, a.actor, operator, i.ID, "legal-document", upload)
	if e != nil || f.State != "ready" {
		t.Fatal(f, e)
	}
	t.Cleanup(func() { _ = s.InvoiceStore.Delete(context.Background(), f.ObjectKey) })
	stored, e := s.InvoiceStore.Get(ctx, f.ObjectKey)
	if e != nil || bytes.Contains(stored, []byte("%PDF")) {
		t.Fatal("private legal file was missing or stored plaintext", e)
	}
	upload.Data = append(upload.Data, ' ')
	if _, e = s.UploadInvoiceFile(ctx, a.actor, operator, i.ID, "legal-document", upload); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed legal bytes replaced immutable key: %v", e)
	}
	if _, e = s.RegisterInvoice(ctx, a.actor, operator, i.ID, "issued", "legal-001", "external-invoice-proof", "verified external issue"); e != nil {
		t.Fatal(e)
	}
	download, e := s.DownloadInvoiceFile(ctx, customer, operator, i.ID, f.ID, true)
	if e != nil || !bytes.Equal(download.Data, invoicePDFFixture()) {
		t.Fatal("customer did not receive original document", e)
	}
	if _, e = s.DownloadInvoiceFile(ctx, uuid.NewString(), operator, i.ID, f.ID, true); !errors.Is(e, ErrForbidden) {
		t.Fatal("another customer downloaded legal file", e)
	}
	a.deny.Store(true)
	if _, e = s.DownloadInvoiceFile(ctx, a.actor, operator, i.ID, f.ID, false); !errors.Is(e, ErrForbidden) {
		t.Fatal("revoked staff retained file access", e)
	}
	a.deny.Store(false)
	if _, e = s.RegisterInvoice(ctx, a.actor, operator, i.ID, "voided", "legal-001", "void-proof", "verified legal cancellation"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DownloadInvoiceFile(ctx, customer, operator, i.ID, f.ID, true); e != nil {
		t.Fatal("void erased original legal history", e)
	}
	replacement, e := s.RequestInvoiceReplacement(ctx, customer, operator, i.ID, "replacement", lines, InvoiceDetails{Title: "Corrected Buyer"})
	if e != nil || replacement.ReplacesInvoiceID == nil || *replacement.ReplacesInvoiceID != i.ID {
		t.Fatal("replacement link missing", replacement, e)
	}
	if _, e = s.RequestInvoiceReplacement(ctx, customer, operator, i.ID, "second-replacement", lines, InvoiceDetails{Title: "Duplicate Buyer"}); !errors.Is(e, ErrInvoiceOrderUsed) {
		t.Fatalf("voided source invoiced twice via replacement: %v", e)
	}
	if e = s.DB.Exec("UPDATE operations_invoice_transitions SET reason='rewrite'").Error; e == nil {
		t.Fatal("legal transition history mutable")
	}
	var accesses int64
	if e = s.DB.Table("operations_invoice_file_access").Count(&accesses).Error; e != nil || accesses != 2 {
		t.Fatalf("private invoice file access not audited: %d %v", accesses, e)
	}
}

func TestInvoiceFileCanBeRemovedBeforeTheResultIsRegistered(t *testing.T) {
	s, a, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	s.Invoices = &invoiceBasis{value: ledger.FromInt64(10)}
	if _, e := s.SaveInvoicePolicy(ctx, a.actor, InvoicePolicy{OperatorID: operator, Enabled: true, Stage: "receipt", MaximumRatio: ledger.FromInt64(1), RulesReference: "published-invoice-policy"}, 0); e != nil {
		t.Fatal(e)
	}
	receipt := uuid.NewString()
	lines := []InvoiceLine{{receipt, ledger.FromInt64(6)}}
	invoice, e := s.RequestInvoice(ctx, customer, operator, "invoice-remove-file", lines, InvoiceDetails{Title: "Verified Buyer"})
	if e != nil {
		t.Fatal(e)
	}
	uploaded, e := s.UploadInvoiceFile(ctx, a.actor, operator, invoice.ID, "wrong-file", InvoiceUpload{Filename: "dot.png", ContentType: "image/png", Data: pngFixture()})
	if e != nil || uploaded.State != "ready" {
		t.Fatal(uploaded, e)
	}
	if e = s.DeleteInvoiceFile(ctx, a.actor, operator, invoice.ID, uploaded.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteInvoiceFile(ctx, a.actor, operator, invoice.ID, uploaded.ID); e != nil {
		t.Fatal("removing the same file again failed", e)
	}
	rows, e := s.ListInvoiceFiles(ctx, a.actor, operator, invoice.ID, false)
	if e != nil || len(rows) != 0 {
		t.Fatal("removed file stayed on the invoice", rows, e)
	}
	if _, e = s.InvoiceStore.Get(ctx, uploaded.ObjectKey); e == nil {
		t.Fatal("removed file remained in private storage")
	}
	replayed, replayErr := s.UploadInvoiceFile(ctx, a.actor, operator, invoice.ID, "wrong-file", InvoiceUpload{Filename: "dot.png", ContentType: "image/png", Data: pngFixture()})
	if !errors.Is(replayErr, ErrConflict) {
		t.Fatal("removed file resurrected", replayed, replayErr)
	}
	kept, e := s.UploadInvoiceFile(ctx, a.actor, operator, invoice.ID, "kept-file", InvoiceUpload{Filename: "legal-invoice.pdf", ContentType: "application/pdf", Data: invoicePDFFixture()})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RegisterInvoice(ctx, a.actor, operator, invoice.ID, "issued", "legal-001", "external-invoice-proof", "verified external issue"); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteInvoiceFile(ctx, a.actor, operator, invoice.ID, kept.ID); !errors.Is(e, ErrInvoiceFileKept) {
		t.Fatal("issued invoice file was removed", e)
	}
}

func pngFixture() []byte {
	return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
}

type delayedInvoiceBlobs struct {
	operationBlobs
	started chan struct{}
	finish  chan struct{}
}

func (b *delayedInvoiceBlobs) Put(ctx context.Context, key string, data []byte, mime string, at time.Time) error {
	close(b.started)
	<-b.finish
	return b.operationBlobs.Put(ctx, key, data, mime, at)
}
func TestInvoiceFileDeletedDuringUploadStaysRemoved(t *testing.T) {
	s, a, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	s.Invoices = &invoiceBasis{value: ledger.FromInt64(10)}
	if _, err := s.SaveInvoicePolicy(ctx, a.actor, InvoicePolicy{OperatorID: operator, Enabled: true, RulesReference: "invoice rules"}, 0); err != nil {
		t.Fatal(err)
	}
	inv, err := s.RequestInvoice(ctx, customer, operator, "invoice", []InvoiceLine{{OrderID: uuid.NewString()}}, InvoiceDetails{Title: "Buyer"})
	if err != nil {
		t.Fatal(err)
	}
	blobs := &delayedInvoiceBlobs{operationBlobs: operationBlobs{data: map[string][]byte{}}, started: make(chan struct{}), finish: make(chan struct{})}
	s.InvoiceStore = blobs
	done := make(chan error, 1)
	go func() {
		_, err := s.UploadInvoiceFile(ctx, a.actor, operator, inv.ID, "upload", InvoiceUpload{Filename: "invoice.pdf", Data: invoicePDFFixture()})
		done <- err
	}()
	select {
	case <-blobs.started:
	case <-time.After(10 * time.Second):
		t.Fatal("upload did not start")
	}
	var file InvoiceFile
	if err := s.DB.Where("invoice_id=?", inv.ID).Take(&file).Error; err != nil {
		close(blobs.finish)
		t.Fatal(err)
	}
	if err := s.DeleteInvoiceFile(ctx, a.actor, operator, inv.ID, file.ID); err != nil {
		close(blobs.finish)
		t.Fatal(err)
	}
	close(blobs.finish)
	if err := <-done; !errors.Is(err, ErrConflict) {
		t.Fatal("late upload restored a removed file", err)
	}
	if err := s.DB.First(&file, "id=?", file.ID).Error; err != nil || file.State != "removed" {
		t.Fatal(file, err)
	}
	if _, err := blobs.Get(ctx, file.ObjectKey); err == nil {
		t.Fatal("late private object retained")
	}
}
