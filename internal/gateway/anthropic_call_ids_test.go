package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// chatCallUpstream is a Chat upstream (a plugin's provider, a relay) that
// answers every request with one Bash call under id, and keeps the bodies
// it was sent.
func chatCallUpstream(t *testing.T, id string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, string(b))
		mu.Unlock()
		quoted, _ := json.Marshal(id)
		if !strings.Contains(string(b), `"stream":true`) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":`+string(quoted)+`,"type":"function","function":{"name":"Bash","arguments":"{\"command\":\"date\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":5}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range []string{
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"m1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":` + string(quoted) + `,"type":"function","function":{"name":"Bash","arguments":""}}]}}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"m1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"date\"}"}}]}}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"m1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":5}}`,
		} {
			io.WriteString(w, "data: "+c+"\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(up.Close)
	return up, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// anthropicCallID is the id of the tool_use in an Anthropic reply, streamed
// or whole.
func anthropicCallID(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev struct {
			ContentBlock struct{ Type, ID string } `json:"content_block"`
		}
		if json.Unmarshal([]byte(data), &ev) == nil && ev.ContentBlock.Type == "tool_use" {
			return ev.ContentBlock.ID
		}
	}
	var m struct{ Content []struct{ Type, ID string } }
	json.Unmarshal([]byte(body), &m)
	for _, p := range m.Content {
		if p.Type == "tool_use" {
			return p.ID
		}
	}
	t.Fatalf("no tool_use in the reply: %s", body)
	return ""
}

// chatIDsSent is the ids of the calls and of the results in a Chat request.
func chatIDsSent(t *testing.T, body string) (calls, answers []string) {
	t.Helper()
	var q struct {
		Messages []struct {
			ToolCalls  []struct{ ID string } `json:"tool_calls"`
			ToolCallID string                `json:"tool_call_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatalf("upstream request: %v: %s", err, body)
	}
	for _, m := range q.Messages {
		for _, c := range m.ToolCalls {
			calls = append(calls, c.ID)
		}
		if m.ToolCallID != "" {
			answers = append(answers, m.ToolCallID)
		}
	}
	return
}

// callAndAnswer asks for a call through /v1/messages, then answers it the
// way Claude Code does, under the id it was given: the id the client saw,
// and the upstream's second request.
func callAndAnswer(t *testing.T, s *Server, stream bool, sent func() []string) (string, string) {
	t.Helper()
	post := func(body string) string {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	st := "false"
	if stream {
		st = "true"
	}
	ask := `{"role":"user","content":"用 Bash 跑一下 date"}`
	id := anthropicCallID(t, post(`{"model":"up/m1","max_tokens":100,"stream":`+st+`,"messages":[`+ask+`]}`))
	before := len(sent())
	quoted, _ := json.Marshal(id)
	post(`{"model":"up/m1","max_tokens":100,"stream":` + st + `,"messages":[` + ask + `,
 {"role":"assistant","content":[{"type":"tool_use","id":` + string(quoted) + `,"name":"Bash","input":{"command":"date"}}]},
 {"role":"user","content":[{"type":"tool_result","tool_use_id":` + string(quoted) + `,"content":"Thu Oct  8 10:00:00 CST 2026"}]}]}`)
	got := sent()
	if len(got) != before+1 {
		t.Fatalf("upstream got %d requests for the answer, want 1", len(got)-before)
	}
	return id, got[len(got)-1]
}
