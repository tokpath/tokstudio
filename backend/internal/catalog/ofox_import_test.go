package catalog

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadOfoxSnapshots(t *testing.T) {
	items, err := loadOfoxSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 100 {
		t.Fatalf("expected 100+ ofox models, got %d", len(items))
	}
	seen := map[string]bool{}
	kinds := map[string]int{}
	for _, item := range items {
		if item.ID == "" || item.DisplayName == "" {
			t.Fatalf("incomplete snapshot: %+v", item)
		}
		if seen[item.ID] {
			t.Fatalf("duplicate id %s", item.ID)
		}
		seen[item.ID] = true
		kinds[item.Kind]++
		if item.SellPrice != nil {
			raw, _ := json.Marshal(item.SellPrice)
			if json.Valid(raw) == false {
				t.Fatalf("bad sell_price for %s", item.ID)
			}
		}
	}
	if kinds["text"] == 0 || kinds["video"] == 0 || kinds["image"] == 0 {
		t.Fatalf("missing modalities: %+v", kinds)
	}
}

func TestPublicModelStatus(t *testing.T) {
	if PublicModelStatus("published") != "available" {
		t.Fatal(PublicModelStatus("published"))
	}
	if PublicModelStatus("deprecated") != "unavailable" {
		t.Fatal(PublicModelStatus("deprecated"))
	}
}

func TestPublicSiteContent(t *testing.T) {
	site := PublicSiteContent()
	if site["source"] != "https://ofox.ai" {
		t.Fatalf("source: %+v", site["source"])
	}
	if site["leaderboards"] == nil {
		t.Fatal("leaderboards missing")
	}
}

func TestOfoxSeedFilesExist(t *testing.T) {
	for _, p := range []string{"seed/ofox-models.json", "seed/ofox-site.json"} {
		if _, err := ofoxSeedFS.ReadFile(p); err != nil {
			t.Fatal(p, err)
		}
	}
	if _, err := os.Stat("seed/ofox-models.json"); err != nil {
		t.Fatal(err)
	}
}
