package gateway

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/provider"
)

var anthropicHi = sse(
	`event: message_start`+"\n"+`data: {"type":"message_start","message":{"id":"msg_1","model":"m1","usage":{"input_tokens":7}}}`,
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
	`data: {"type":"content_block_stop","index":0}`,
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
	`data: {"type":"message_stop"}`)

var chatHi = sse(
	`data: {"id":"c1","model":"m1","choices":[{"delta":{"role":"assistant","content":"hi"}}]}`,
	`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
	`data: [DONE]`)

// responsesHi is the same reply from a Responses API, which reads its own
// events only: a Chat stream from it is no reply (#1933).
var responsesHi = sse(
	`data: {"type":"response.output_text.delta","item_id":"m1","output_index":0,"content_index":0,"delta":"hi"}`,
	`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":1}}}`)

// A vendor's model on its Anthropic API that won't stop thinking (Z.ai's
// GLM-5.3, #699) refuses the thinking off a Codex asking none is now sent
// (#1636): asked again thinking at its least, as before, and from then on
// asked so at once.
func TestTranslatedThinkingOffRefused(t *testing.T) {
	for i, refusal := range []string{
		`GLM-5.3 always engages in thinking and cannot be disabled`,
		// MiniMax's and Volcengine Ark's wordings (#1141)
		`invalid params, model \"MiniMax-M3.1-Flash-Preview\" requires adaptive thinking; thinking.type=\"disabled\" (including reasoning.effort=none) is not allowed (2013)`,
		"thinking.type `disabled` is not supported by this model Request id: 0217",
		// Sub2API's gpt-6.1-sol reads disabled as reasoning effort none (#1465)
		`gpt-6.1-sol does not support reasoning effort \"none\"; use low, medium, high, xhigh or max`,
		// #699's Chinese, escaped as \uXXXX the way a relay's
		// json.dumps sends it (#1669)
		`\u8be5\u6a21\u578b\u59cb\u7ec8\u652f\u6301\u601d\u8003\uff0c\u4e0d\u53ef\u5173\u95ed`,
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) { translatedThinkingOffRefused(t, refusal) })
	}
}

func translatedThinkingOffRefused(t *testing.T, refusal string) {
	var sent []string
	f := &fake{t: t, reply: anthropicHi}
	f.refuse = func(b []byte) (int, string) {
		var v struct {
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		json.Unmarshal(b, &v)
		sent = append(sent, v.Thinking.Type)
		if v.Thinking.Type == "disabled" {
			return 400, `{"type":"error","error":{"type":"invalid_request_error","message":"` + refusal + `"}}`
		}
		return 0, ""
	}
	setup(t, provider.Anthropic, f)
	s := New()
	for turn, want := range []string{"disabled,enabled", "enabled"} {
		sent = nil
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"m1","stream":true,"input":"hi","reasoning":{"effort":"none"}}`)))
		if rec.Code != 200 {
			t.Fatalf("turn %d: %d %s", turn+1, rec.Code, rec.Body)
		}
		if strings.Join(sent, ",") != want {
			t.Errorf("turn %d: thinking sent %v, want %s", turn+1, sent, want)
		}
		if r := lastRoute(s); len(r.Tries) != 1 || r.Tries[0].Effort != "low" {
			t.Errorf("turn %d: tries %+v, want it logged at low", turn+1, r.Tries)
		}
	}
}
