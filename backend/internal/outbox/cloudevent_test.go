package outbox

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCloudEventEnvelope(t *testing.T) {
	event := CloudEvent{
		SpecVersion:     "1.0",
		ID:              "evt_1",
		Source:          "tokenhub/outbox",
		Type:            "tokenhub.audit.recorded.v1",
		Time:            time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC),
		DataContentType: "application/json",
		Data:            json.RawMessage(`{"ok":true}`),
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["specversion"] != "1.0" || decoded["source"] != "tokenhub/outbox" {
		t.Fatalf("unexpected envelope: %s", body)
	}
}
