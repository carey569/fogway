package gateway

import (
	"testing"
)

func TestParseResponsesAllowedNamespacedTool(t *testing.T) {
	body := []byte(`{"model":"m","input":"hi",` + namespacedTools + `,"tool_choice":{"type":"allowed_tools","mode":"required","tools":[{"type":"function","namespace":"collaboration","name":"spawn_agent"}]}}`)
	r, err := parseResponses(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tools) != 1 || r.Tools[0].Name != "collaboration__spawn_agent" || len(r.Namespaced) != 1 {
		t.Fatalf("tools = %v, namespaced = %v", r.Tools, r.Namespaced)
	}
}
