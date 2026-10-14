package gateway

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// orAnswers is an aggregator that answers as OpenRouter does (seen
// 2026-10-05): a Chat body and every chunk of its stream say the
// provider behind it in a top-level "provider", an Anthropic body too, and
// an Anthropic stream in message_start's message.
func orAnswers(proto provider.Protocol, upstream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		stream := strings.Contains(string(b), `"stream":true`)
		P := `"provider":"` + upstream + `"`
		if !stream {
			w.Header().Set("Content-Type", "application/json")
			if proto == provider.Chat {
				io.WriteString(w, `{"id":"gen-1","object":"chat.completion","model":"deepseek/deepseek-chat",`+P+`,"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`)
			} else {
				io.WriteString(w, `{"id":"gen-1","type":"message","role":"assistant","model":"deepseek/deepseek-chat","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2},`+P+`}`)
			}
			return
		}
		var events []string
		if proto == provider.Chat {
			events = []string{
				`data: {"id":"gen-1","model":"deepseek/deepseek-chat",` + P + `,"choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`,
				`data: {"id":"gen-1","model":"deepseek/deepseek-chat",` + P + `,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
				`data: [DONE]`,
			}
		} else {
			events = []string{
				"event: message_start\ndata: " + `{"type":"message_start","message":{"id":"gen-1","type":"message","role":"assistant","model":"deepseek/deepseek-chat","content":[],"usage":{"input_tokens":5,"output_tokens":0},` + P + `}}`,
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

// Only a name is an upstream: a reply without one, or one echoing a
// request's provider options (an object), says none.
func TestUpstreamOf(t *testing.T) {
	for in, want := range map[string]string{
		`{"id":"c1","provider":"Novita","choices":[]}`:                  "Novita",
		`{"type":"message_start","message":{"provider":"StreamLake"}}`:  "StreamLake",
		`{"id":"c1","choices":[]}`:                                      "",
		`{"provider":{"order":["deepseek"]},"choices":[]}`:              "",
		`{"provider":"  "}`:                                             "",
		`{"choices":[{"delta":{"content":"the \"provider\" field"}}]}`:  "",
		`not json "provider"`:                                           "",
		`{"type":"response.created","response":{"provider":"Baseten"}}`: "Baseten",
	} {
		if got := upstreamOf([]byte(in)); got != want {
			t.Errorf("upstreamOf(%s) = %q; want %q", in, got, want)
		}
	}
}
