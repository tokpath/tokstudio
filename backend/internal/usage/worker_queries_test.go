package usage

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

func TestPostgresPendingRequestsPageDiscoversDueUsageAndMedia(t *testing.T) {
	f := setup(t, "10", nil)
	var now time.Time
	if err := f.db.Raw("SELECT statement_timestamp()").Row().Scan(&now); err != nil {
		t.Fatal(err)
	}
	// Ordinary deadlines are evaluated by PostgreSQL even if worker time differs.
	f.s.now = func() time.Time { return now.Add(-time.Hour) }
	accepted := now.Add(-2 * time.Hour)
	recent := now.Add(-time.Minute)
	beforeRetention := f.s.wallNow().Add(-f.s.ResultRetention - time.Hour)

	type sample struct {
		name, status string
		operation    modelruntime.Operation
		asyncImage   bool
		completedAt  *time.Time
		resultState  string
		discovered   bool
	}
	var sequence int
	insert := func(s sample) string {
		t.Helper()
		sequence++
		row := Request{
			ID:         fmt.Sprintf("00000000-0000-4000-8000-%012d", sequence),
			CustomerID: f.customer, OperatorID: f.operator, BrandID: f.brand,
			OwnershipVersion: 1, KeyID: f.key.Key.ID, BusinessKey: s.name,
			Fingerprint: s.name, ModelID: "public-model", Operation: s.operation,
			Streaming:  s.operation == modelruntime.Chat && s.status == "executing",
			AsyncImage: s.asyncImage, Status: s.status, CompletedAt: s.completedAt,
			AcceptedAt: accepted, UsageDeadline: now.Add(time.Hour), WindowStart: accepted,
			SupplyHolds: []byte("[]"), PriceSnapshot: []byte("{}"), Candidates: []byte("[]"),
		}
		if s.asyncImage {
			row.DeliveredAt = &accepted
		}
		if terminal(s.status) {
			row.UsageDeadline = recent
		}
		if err := f.db.Create(&row).Error; err != nil {
			t.Fatal(s.name, err)
		}
		if s.resultState != "" {
			result := resultRow{RequestID: row.ID, ContentType: "video/mp4", ObjectKey: s.name,
				Digest: "fixture", State: s.resultState, ExpiresAt: now.Add(time.Hour), CreatedAt: recent}
			if err := f.db.Create(&result).Error; err != nil {
				t.Fatal(s.name, err)
			}
		}
		return row.ID
	}

	var ordinary, media []string
	for _, status := range []string{"accepted", "executing", "submitted", "awaiting_usage"} {
		ordinary = append(ordinary, insert(sample{name: status, status: status, operation: modelruntime.Chat}))
	}
	ordinary = append(ordinary, insert(sample{name: "sync-image", status: "accepted", operation: modelruntime.Image}))
	asyncImageIDs := make(map[string]bool)
	for _, candidate := range []sample{
		{name: "async-image", status: "accepted", operation: modelruntime.Image, asyncImage: true, discovered: true},
		{name: "async-image-reconcile", status: "awaiting_usage", operation: modelruntime.Image, asyncImage: true, discovered: true},
		{name: "video-poll", status: "submitted", operation: modelruntime.Video, discovered: true},
		{name: "video-recover", status: "completed", operation: modelruntime.Video, completedAt: &recent, discovered: true},
		{name: "video-result-pending", status: "completed", operation: modelruntime.Video, completedAt: &recent, resultState: "pending", discovered: true},
		{name: "video-result-ready", status: "completed", operation: modelruntime.Video, completedAt: &recent, resultState: "ready"},
		{name: "video-retention-ended", status: "completed", operation: modelruntime.Video, completedAt: &beforeRetention},
		{name: "chat-completed", status: "completed", operation: modelruntime.Chat, completedAt: &recent},
		{name: "video-failed", status: "failed", operation: modelruntime.Video, completedAt: &recent},
		{name: "async-image-free", status: "free", operation: modelruntime.Image, asyncImage: true, completedAt: &recent},
		{name: "chat-cancelled", status: "cancelled", operation: modelruntime.Chat, completedAt: &recent},
	} {
		id := insert(candidate)
		if candidate.discovered {
			media = append(media, id)
			asyncImageIDs[id] = candidate.asyncImage
		}
	}

	checkPage := func(cursor string, limit int, want []string) string {
		t.Helper()
		page, err := f.s.PendingRequestsPage(f.ctx, cursor, limit)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, work := range page {
			ids = append(ids, work.ID)
			if work.CustomerID != f.customer || work.AsyncImage != asyncImageIDs[work.ID] {
				t.Fatalf("incorrect worker routing fields for request %s", work.ID)
			}
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("page after %q: got %v, want %v", cursor, ids, want)
		}
		if len(ids) != 0 {
			return ids[len(ids)-1]
		}
		return cursor
	}

	checkPage("", 100, media)
	if err := f.db.Model(&Request{}).Where("id IN ?", ordinary).
		Update("usage_deadline", gorm.Expr("statement_timestamp()")).Error; err != nil {
		t.Fatal(err)
	}
	want := append(ordinary, media...)
	const pageSize = 3
	cursor := ""
	for start := 0; start < len(want); start += pageSize {
		cursor = checkPage(cursor, pageSize, want[start:min(start+pageSize, len(want))])
	}
	checkPage(cursor, pageSize, nil)
}
