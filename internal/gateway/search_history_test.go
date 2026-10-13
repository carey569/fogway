package gateway

import (
	"encoding/json"
	"testing"
)

// searchedHistory is a conversation that searched the web, its items the
// ChatGPT backend's own (gpt-6.1-sol, a ChatGPT Pro account, 2026-10-09):
// a commentary message, the web_search_call, the answer citing what it
// found.
const searchedHistory = `{"type":"message","role":"user","content":[{"type":"input_text","text":"Use web search: what is the newest Go release announced on go.dev? Answer in one line."}]},` +
	`{"id":"msg_0c7e789f31e92725016ac8f910d47087d0a126fed4f3afdc86","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":"I’ll check go.dev for the latest release announcement.\n"}],"phase":"commentary","role":"assistant"},` +
	`{"id":"ws_0c7e789f31e92725016ac8f91213cc87d0ac350b2fd39a94a2","type":"web_search_call","status":"completed","action":{"type":"search","queries":["site:go.dev newest Go release October 2026 release"],"query":"site:go.dev newest Go release October 2026 release"}},` +
	`{"id":"msg_0c7e789f31e92725016ac8f915990c87d0987c34e2e7b8b409","type":"message","status":"completed","content":[{"type":"output_text","annotations":[{"type":"url_citation","end_index":152,"start_index":90,"title":"Release History - The Go Programming Language","url":"https://go.dev/doc/devel/release?utm_source=openai"}],"logprobs":[],"text":"The newest Go release listed on go.dev is **Go 1.27.2**, released on **October 8, 2026**. ([go.dev](https://go.dev/doc/devel/release?utm_source=openai))"}],"phase":"final_answer","role":"assistant"}`

// sentTools is what a request sent upstream declared: its tools' types,
// its additional_tools items' tools' types, and its tool_choice.
func sentTools(t *testing.T, b []byte) (tools, additional []string, choice any, last string) {
	t.Helper()
	var q struct {
		Input []struct {
			Type  string           `json:"type"`
			Tools []map[string]any `json:"tools"`
		} `json:"input"`
		Tools      []map[string]any `json:"tools"`
		ToolChoice any              `json:"tool_choice"`
	}
	if err := json.Unmarshal(b, &q); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	for _, tl := range q.Tools {
		tools = append(tools, tl["type"].(string))
	}
	for _, it := range q.Input {
		for _, tl := range it.Tools {
			additional = append(additional, tl["type"].(string))
		}
	}
	if n := len(q.Input); n > 0 {
		last = q.Input[n-1].Type
	}
	return tools, additional, q.ToolChoice, last
}
