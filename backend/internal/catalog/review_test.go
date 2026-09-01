package catalog

import "testing"

func TestEffectiveSyncState(t *testing.T) {
	cases := []struct {
		status, sync, want string
	}{
		{"draft", "", SyncDraft},
		{"draft", "reviewed", SyncReviewed},
		{"published", "", SyncPublished},
		{"deprecated", "", SyncPublished},
		{"published", "reviewed", SyncReviewed},
	}
	for _, tc := range cases {
		got := EffectiveSyncState(tc.status, tc.sync)
		if got != tc.want {
			t.Fatalf("EffectiveSyncState(%q, %q)=%q want %q", tc.status, tc.sync, got, tc.want)
		}
	}
}

func TestFilterAdminModels(t *testing.T) {
	items := []ModelView{
		{ID: "tokenhub/a", Status: "draft", SyncState: "", Vendor: "tokenhub", DisplayName: "A"},
		{ID: "tokenhub/b", Status: "draft", SyncState: SyncReviewed, Vendor: "tokenhub", DisplayName: "B"},
		{ID: "tokenhub/c", Status: "draft", SyncState: SyncRejected, Vendor: "tokenhub", DisplayName: "C"},
		{ID: "tokenhub/echo-1", Status: "published", SyncState: SyncPublished, Vendor: "tokenhub", DisplayName: "Echo"},
	}
	drafts := FilterAdminModels(items, "", SyncDraft, "")
	if len(drafts) != 1 || drafts[0].ID != "tokenhub/a" {
		t.Fatalf("draft queue: %+v", drafts)
	}
	reviewed := FilterAdminModels(items, "", SyncReviewed, "")
	if len(reviewed) != 1 || reviewed[0].ID != "tokenhub/b" {
		t.Fatalf("reviewed queue: %+v", reviewed)
	}
	rejected := FilterAdminModels(items, "", SyncRejected, "tokenhub/c")
	if len(rejected) != 1 {
		t.Fatalf("rejected+q: %+v", rejected)
	}
	published := FilterAdminModels(items, "published", "", "")
	if len(published) != 1 || published[0].ID != "tokenhub/echo-1" {
		t.Fatalf("published: %+v", published)
	}
}
