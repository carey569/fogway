package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// servedBy is a vendor that answers in its protocol, streamed when asked,
// naming model as the one that answered.
func servedBy(proto provider.Protocol, model string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Stream bool `json:"stream"`
		}
		json.Unmarshal(b, &req)
		m, _ := json.Marshal(model)
		M := string(m)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			switch proto {
			case provider.Chat:
				io.WriteString(w, `{"id":"c1","object":"chat.completion","model":`+M+`,"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)
			case provider.Responses:
				io.WriteString(w, `{"id":"r1","object":"response","model":`+M+`,"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":5,"output_tokens":2}}`)
			default:
				io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":`+M+`,"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`)
			}
			return
		}
		var events []string
		switch proto {
		case provider.Chat:
			events = []string{
				`data: {"id":"c1","model":` + M + `,"choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`,
				`data: {"id":"c1","model":` + M + `,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
				`data: [DONE]`,
			}
		case provider.Responses:
			events = []string{
				"event: response.created\ndata: " + `{"type":"response.created","response":{"id":"r1","model":` + M + `,"status":"in_progress","output":[]}}`,
				"event: response.output_item.added\ndata: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[]}}`,
				"event: response.output_text.delta\ndata: " + `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hi"}`,
				"event: response.output_item.done\ndata: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
				"event: response.completed\ndata: " + `{"type":"response.completed","response":{"id":"r1","model":` + M + `,"status":"completed","output":[],"usage":{"input_tokens":5,"output_tokens":2}}}`,
			}
		default:
			events = []string{
				"event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":` + M + `,"content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`,
				"event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
				"event: content_block_stop\ndata: " + `{"type":"content_block_stop","index":0}`,
				"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
				"event: message_stop\ndata: " + `{"type":"message_stop"}`,
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range events {
			io.WriteString(w, ev+"\n\n")
		}
	}
}

// swapped tells another model from the one asked for, and not the same
// one under its dated, pinned or prefixed name.
func TestSwappedModel(t *testing.T) {
	for _, c := range []struct {
		sent, served string
		want         bool
	}{
		{"sol", "luna", true},
		{"gpt-5", "gpt-5-mini", true},
		{"gpt-5", "gpt-5-mini-2025-08-07", true},
		{"gpt-4o", "gpt-4o-mini", true},
		{"claude-opus-4", "claude-opus-4-1-20250805", true},
		{"deepseek-v3", "deepseek-v2", true},
		{"gpt-5", "gpt-5.1", true},
		{"gpt-5", "gpt-5-2025-08-07", false},
		{"gpt-5", "GPT-5", false},
		{"gpt-4", "gpt-4-0613", false},
		{"claude-sonnet-4-5", "claude-sonnet-4-5-20250929", false},
		{"claude-3-5-sonnet-latest", "claude-3-5-sonnet-20241022", false},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5", false},
		{"gemini-2.5-pro", "models/gemini-2.5-pro", false},
		{"gemini-1.5-pro", "gemini-1.5-pro-002", false},
		{"gemini-2.5-pro", "gemini-2.5-pro-preview-06-05", false},
		{"openai/gpt-5", "gpt-5-2025-08-07", false},
		{"us.anthropic.claude-sonnet-4-20250514-v1:0", "claude-sonnet-4-20250514", false},
		{"auto", "gpt-5-mini", false}, // Copilot's Auto, Cursor's: the vendor was asked to pick
		{"copilot/auto", "claude-sonnet-5", false},
		// a remote magpie's routing group, answered by the member it routed to
		{"group/auto-deepseek-v4-1-flash", "deepseek/deepseek-v4.1-flash", false},
		{"group/fast", "luna", false},
		{"sol", "", false},
		{"", "luna", false},
	} {
		if got := swapped(c.sent, c.served); got != c.want {
			t.Errorf("swapped(%q, %q) = %v, want %v", c.sent, c.served, got, c.want)
		}
	}
}

// The sniffer reads the model a relayed reply names as it goes by.
func TestSniffServedModel(t *testing.T) {
	for _, c := range []struct {
		proto provider.Protocol
		ct    string
		body  string
	}{
		{provider.Chat, "text/event-stream", "data: {\"model\":\"luna\",\"choices\":[]}\n\ndata: [DONE]\n\n"},
		{provider.Chat, "application/json", `{"model":"luna","choices":[]}`},
		{provider.Responses, "text/event-stream", "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"model\":\"luna\"}}\n\n"},
		{provider.Responses, "application/json", `{"object":"response","model":"luna"}`},
		{provider.Anthropic, "text/event-stream", "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"luna\"}}\n\n"},
		{provider.Anthropic, "application/json", `{"type":"message","model":"luna"}`},
	} {
		s := newSniffer(c.proto, c.ct)
		s.write([]byte(c.body))
		if got := s.usage().Served; got != "luna" {
			t.Errorf("%s %s: served %q", c.proto, c.ct, got)
		}
	}
}
