package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/catalog"
	"github.com/fogway/fogway/internal/provider"
)

func TestFitEffort(t *testing.T) {
	glm := []string{"low", "high", "max"}
	for _, c := range []struct {
		want   string
		levels []string
		got    string
	}{
		{"medium", glm, "high"}, // a tie goes up
		{"low", glm, "low"},
		{"xhigh", glm, "max"},
		{"max", []string{"low", "medium", "high"}, "high"},
		{"low", []string{"none", "minimal", "medium"}, "medium"},
		{"medium", nil, "medium"}, // levels not known: as asked
		{"medium", []string{"none"}, "medium"},
	} {
		if got := fitEffort(c.want, c.levels); got != c.got {
			t.Errorf("fitEffort(%q, %v) = %q, want %q", c.want, c.levels, got, c.got)
		}
	}
}

// Codex's effort reaches a custom OpenAI-compatible provider as
// reasoning_effort, at a level the model takes: glm-5.3-flash takes low,
// high and max, as models.dev has it from those serving it, so Codex's
// "medium" goes as "high"; a model no one gives levels for gets what was
// asked.
func TestResponsesEffortReachesChatVendor(t *testing.T) {
	f := &fake{t: t, reply: sse(
		`data: {"id":"c1","model":"glm-5.3-flash","choices":[{"delta":{"role":"assistant","content":"hi"}}]}`,
		`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`)}
	up := setup(t, provider.Chat, f)
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"zai":{"models":{"glm-5.3-flash":{"id":"glm-5.3-flash","reasoning_options":[{"type":"effort","values":["low","high","max"]}]}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "volc", Name: "Volc", Key: "k", Chat: up.URL + "/v1", Models: []string{"glm-5.3-flash", "other"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ model, effort, sent string }{
		{"volc/glm-5.3-flash", "medium", "high"},
		{"volc/glm-5.3-flash", "max", "max"},
		{"volc/glm-5.3-flash", "low", "low"},
		{"volc/other", "medium", "medium"},
	} {
		code, body := post(t, "/v1/responses", `{"model":"`+c.model+`","stream":true,"input":"hi","reasoning":{"effort":"`+c.effort+`"}}`)
		if code != 200 {
			t.Fatalf("status %d: %s", code, body)
		}
		var got map[string]any
		json.Unmarshal(f.got, &got)
		if got["reasoning_effort"] != c.sent || !strings.HasSuffix(f.path, "/chat/completions") {
			t.Errorf("%s at %s: sent %s", c.model, c.effort, f.got)
		}
	}
}

// A model its maker lists with a thinking switch alone — Xiaomi's
// mimo-v2.6-flash, served here by a relay — is sent no more than high: an
// agent asking max or xhigh of it got Xiaomi's 400 (#214). What it may take
// below that goes as asked, as does max to a model no one lists.
func TestSwitchModelSentNoMoreThanHigh(t *testing.T) {
	f := &fake{t: t, reply: sse(
		`data: {"id":"c1","model":"mimo-v2.6-flash","choices":[{"delta":{"role":"assistant","content":"hi"}}]}`,
		`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`)}
	up := setup(t, provider.Chat, f)
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{
	  "xiaomi":{"models":{"mimo-v2.6-flash":{"id":"mimo-v2.6-flash","reasoning":true,"reasoning_options":[{"type":"toggle"}]}}},
	  "llmgateway":{"models":{"mimo-v2.6-flash":{"id":"mimo-v2.6-flash","reasoning_options":[{"type":"effort","values":["none","low","medium","high","xhigh","max"]}]}}},
	  "zai":{"models":{"glm-5.3-flash":{"id":"glm-5.3-flash","reasoning_options":[{"type":"effort","values":["low","high","max"]}]}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "volc", Name: "Volc", Key: "k", Chat: up.URL + "/v1", Models: []string{"mimo-v2.6-flash", "glm-5.3-flash", "other"}}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ model, effort, sent string }{
		{"volc/mimo-v2.6-flash", "max", "high"},
		{"volc/mimo-v2.6-flash", "xhigh", "high"},
		{"volc/mimo-v2.6-flash", "low", "low"},
		{"volc/glm-5.3-flash", "medium", "high"},
		{"volc/other", "max", "max"},
	} {
		code, body := post(t, "/v1/responses", `{"model":"`+c.model+`","stream":true,"input":"hi","reasoning":{"effort":"`+c.effort+`"}}`)
		if code != 200 {
			t.Fatalf("status %d: %s", code, body)
		}
		var got map[string]any
		json.Unmarshal(f.got, &got)
		if got["reasoning_effort"] != c.sent {
			t.Errorf("%s at %s: sent %s", c.model, c.effort, f.got)
		}
	}
}

// requestEffort reads the reasoning a request asks for in each API's words.
func TestRequestEffort(t *testing.T) {
	for _, c := range []struct {
		proto provider.Protocol
		body  string
		want  string
	}{
		{provider.Chat, `{"reasoning_effort":"low"}`, "low"},
		{provider.Chat, `{"model":"m"}`, ""},
		{provider.Responses, `{"reasoning":{"effort":"xhigh","summary":"auto"}}`, "xhigh"},
		{provider.Responses, `{"reasoning":{"summary":"auto"}}`, ""},
		{provider.Anthropic, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`, "medium"},
		{provider.Anthropic, `{"thinking":{"type":"enabled","budget_tokens":4096}}`, "low"},
		{provider.Anthropic, `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, ""},
		{provider.Anthropic, `{"output_config":{"effort":"high"}}`, ""},
	} {
		if got := requestEffort(c.proto, []byte(c.body)); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.proto, c.body, got, c.want)
		}
	}
}
