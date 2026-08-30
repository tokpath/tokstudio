package app

import (
	"strings"
	"testing"
)

func TestDocsExamplesUseBrandAndPlaceholderKey(t *testing.T) {
	examples := docsExamples("api.oem.localhost", "tokenhub/oem-demo")
	curl, _ := examples["curl"].(string)
	for _, part := range []string{"https://api.oem.localhost/v1/chat/completions", "tokenhub/oem-demo", "$TOKENHUB_API_KEY"} {
		if !strings.Contains(curl, part) {
			t.Fatalf("curl missing %q: %s", part, curl)
		}
	}
	if strings.Contains(curl, "sk-live") || strings.Contains(curl, "thsk_") {
		t.Fatal("examples must not embed a real key")
	}
	messages, _ := examples["messages"].(string)
	if !strings.Contains(messages, "/v1/messages") || !strings.Contains(messages, "$TOKENHUB_API_KEY") {
		t.Fatalf("messages: %s", messages)
	}
	if notes := docsNotes(); notes["webhook"] == "" || notes["errors"] == "" {
		t.Fatalf("notes: %+v", notes)
	}
}
