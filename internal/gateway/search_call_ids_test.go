package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/provider"
)

// A search a translated vendor's model made goes back to Codex as a
// tool_search_call with a tsc_ id, the one kind OpenAI takes for it.
func TestToolSearchCallHasTscID(t *testing.T) {
	req := &Request{Model: "m", Namespaced: map[string]nsTool{"tool_search": {Search: true}}}
	rec := httptest.NewRecorder()
	enc := encoder(provider.Responses, newSSEWriter(rec), req)
	enc.event(Event{Kind: KToolStart, ID: "s2", Name: "tool_search"})
	enc.event(Event{Kind: KToolArgs, Text: `{"query":"calendar"}`})
	enc.event(Event{Kind: KToolStart, ID: "c1", Name: "shell"})
	enc.finish()
	ids := map[string]string{}
	for _, ev := range events(rec.Body.String()) {
		if ev["type"] == "response.output_item.done" {
			it := ev["item"].(map[string]any)
			ids[it["type"].(string)], _ = it["id"].(string)
		}
	}
	if !strings.HasPrefix(ids["tool_search_call"], "tsc_") || !strings.HasPrefix(ids["function_call"], "fc_") {
		t.Fatalf("streamed ids = %v", ids)
	}

	res := Result{Parts: []Part{{Kind: ToolCall, ID: "s3", Name: "tool_search", Args: json.RawMessage(`{"query":"mail"}`)}}}
	var out struct {
		Output []map[string]any `json:"output"`
	}
	if err := json.Unmarshal(render(provider.Responses, res, req), &out); err != nil {
		t.Fatal(err)
	}
	if id, _ := out.Output[0]["id"].(string); len(out.Output) != 1 || !strings.HasPrefix(id, "tsc_") {
		t.Fatalf("rendered = %v", out.Output)
	}
}

// Nothing to fix leaves the request byte for byte.
func TestSearchCallIDsUntouched(t *testing.T) {
	for _, b := range []string{
		`{"model":"gpt-5.5","input":[{"type":"function_call","id":"fc_1","call_id":"c","name":"shell","arguments":"{}"}]}`,
		`{"model":"gpt-5.5",  "input":[{"type":"tool_search_call","id":"tsc_1","call_id":"s","arguments":{}},{"type":"tool_search_call","call_id":"t","arguments":{}}]}`,
	} {
		if got := callItemIDs([]byte(b)); string(got) != b {
			t.Errorf("%s -> %s", b, got)
		}
	}
}
