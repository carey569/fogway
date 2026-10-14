package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
)

// Google's API takes a part's fields by their proto names as well
// (inline_data, mime_type, file_data, function_call), which agy and raw
// SDK calls send: read only as camelCase, an image or a PDF was left out
// and the model made up what it held (#934). The caller's own keys — a
// call's args, a declaration's schema — are kept as they were.
func TestGeminiSnakeCaseParts(t *testing.T) {
	body := `{"system_instruction":{"parts":[{"text":"be brief"}]},
	"generation_config":{"max_output_tokens":77,"thinking_config":{"thinking_level":"high"}},
	"tools":[{"function_declarations":[{"name":"read","parameters":{"type":"object","properties":{"file_path":{"type":"string"}}}}]}],
	"contents":[
		{"role":"user","parts":[{"text":"Transcribe this."},{"inline_data":{"mime_type":"image/jpeg","data":"SU1H"}},
			{"inline_data":{"mime_type":"application/pdf","data":"UERG"}},{"file_data":{"mime_type":"application/pdf","file_uri":"gs://b/doc.pdf"}}]},
		{"role":"model","parts":[{"function_call":{"id":"c1","name":"read","args":{"file_path":"/a_b"}}}]},
		{"role":"user","parts":[{"function_response":{"id":"c1","name":"read","response":{"line_count":3}}}]}]}`
	r, err := parseGemini([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if r.System != "be brief" || r.MaxTokens != 77 || r.Effort != "high" {
		t.Fatalf("system %q, max %d, effort %q", r.System, r.MaxTokens, r.Effort)
	}
	if len(r.Tools) != 1 || !strings.Contains(string(r.Tools[0].Schema), `"file_path"`) {
		t.Fatalf("tools: %+v", r.Tools)
	}
	if len(r.Messages) != 3 {
		t.Fatalf("messages: %+v", r.Messages)
	}
	got := r.Messages[0].Parts
	if len(got) != 4 || got[1].Kind != Image || got[1].MediaType != "image/jpeg" || got[1].Data != "SU1H" ||
		got[2].Kind != File || got[2].MediaType != "application/pdf" || got[2].Data != "UERG" ||
		got[3].Kind != File || got[3].URL != "gs://b/doc.pdf" {
		t.Fatalf("the user's parts: %+v", got)
	}
	call := r.Messages[1].Parts
	if len(call) != 1 || call[0].Kind != ToolCall || call[0].Name != "read" || !strings.Contains(string(call[0].Args), `"file_path":"/a_b"`) {
		t.Fatalf("the call: %+v", call)
	}
	res := r.Messages[2].Parts
	if len(res) != 1 || res[0].Kind != ToolResult || !strings.Contains(res[0].Text, "line_count") {
		t.Fatalf("the result: %+v", res)
	}
}

// Through the gateway: an image sent as inline_data to /v1beta reaches a
// Chat upstream as the image it is.
func TestGeminiSnakeCaseImageReachesUpstream(t *testing.T) {
	setHome(t, t.TempDir())
	f := &fake{t: t, reply: sse(
		`data: {"id":"c1","choices":[{"delta":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
		`data: [DONE]`)}
	setup(t, provider.Chat, f)
	code, out := post(t, "/v1beta/models/fake/m1:generateContent",
		`{"contents":[{"role":"user","parts":[{"text":"Transcribe this."},{"inline_data":{"mime_type":"image/png","data":"iVBORw0KGgo="}}]}]}`)
	if code != 200 || !strings.Contains(out, "OK") {
		t.Fatalf("%d %s", code, out)
	}
	if !strings.Contains(string(f.got), "data:image/png;base64,iVBORw0KGgo=") {
		t.Fatalf("the upstream was not given the image: %s", f.got)
	}
}

// A held Gemini stream is kept alive with nothing: agy fails on an SSE
// comment ("invalid stream chunk: : keepalive", #934), as Google's SDK
// does (geminiEncoder.keepalive). The other protocols still get theirs.
func TestHeldGeminiStreamSendsNoComment(t *testing.T) {
	for _, c := range []struct {
		proto provider.Protocol
		want  bool
	}{{provider.Gemini, false}, {provider.Anthropic, true}, {provider.Chat, true}} {
		rec := httptest.NewRecorder()
		h := newHoldWriter(rec, true)
		h.alive = &keptAlive{proto: c.proto, at: time.Now().Add(-time.Hour)}
		h.keepAlive()
		if got := strings.Contains(rec.Body.String(), ": keepalive"); got != c.want {
			t.Fatalf("%s: %q", c.proto, rec.Body.String())
		}
	}
}
