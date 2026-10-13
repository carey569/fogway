package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// desktopPicker is Claude Desktop's tIt (index.chunk-D3OyLXgG.js, 2.7032):
// whether the model id it was given gets a thinking-effort picker. IC is
// qS of the lowercased id; HFt's keys that have effort levels, and UFt.
func desktopPicker(id string) bool {
	e := strings.ToLower(id)
	t := regexp.MustCompile(`^arn:aws[a-z-]*:bedrock:[^/]+/`).ReplaceAllString(e, "")
	t = regexp.MustCompile(`^(?:[a-z][a-z0-9-]*\.)?anthropic\.`).ReplaceAllString(t, "")
	stripped := t != e || regexp.MustCompile(`^claude-(?:[a-z]+-)?\d`).MatchString(t)
	t = regexp.MustCompile(`\[[^\]]+\]$`).ReplaceAllString(t, "")
	if stripped {
		t = regexp.MustCompile(`-v\d+(?::\d+)?$`).ReplaceAllString(t, "")
	}
	t = regexp.MustCompile(`@\d{8}$`).ReplaceAllString(t, "")
	t = regexp.MustCompile(`-\d{8}$`).ReplaceAllString(t, "")
	switch t {
	case "claude-sonnet-4-6", "claude-sonnet-5", "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8", "claude-opus-5":
		return true
	}
	return regexp.MustCompile(`^(?:claude-)?(?:fable|mythos)(?:-|$)`).MatchString(t)
}

// Desktop's small_fast pick (_$n): an id with haiku, sonnet or opus in it
func desktopSmallFast(id string) bool {
	for _, w := range []string{"haiku", "sonnet", "opus"} {
		if strings.Contains(id, w) {
			return true
		}
	}
	return false
}

// the effort picked in Claude Desktop reaches the model, fitted to its
// levels: Desktop's Claude Code sends thinking plus output_config.effort
func TestClaudeDesktopEffortReachesModel(t *testing.T) {
	f := &fake{reply: sse(
		`event: message_start`+"\n"+`data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"m1","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
		`event: message_stop`+"\n"+`data: {"type":"message_stop"}`)}
	setup(t, provider.Anthropic, f)
	if err := provider.SetModelEfforts("fake/m1", []string{"low", "high"}); err != nil {
		t.Fatal(err)
	}
	alias := desktopEffortAlias + aliasNumber("fake/m1")
	for _, c := range []struct{ asked, sent string }{
		{"low", "low"}, {"high", "high"}, {"max", "high"}, {"xhigh", "high"},
	} {
		body := `{"model":"` + alias + `","max_tokens":32000,"stream":true,"thinking":{"type":"adaptive"},"output_config":{"effort":"` + c.asked + `"},` +
			`"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		req.Header.Set("x-api-key", Token+"-claude-desktop")
		rec := httptest.NewRecorder()
		New().Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", c.asked, rec.Code, rec.Body)
		}
		var sent struct {
			Model        string `json:"model"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(f.got, &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Model != "m1" || sent.OutputConfig.Effort != c.sent {
			t.Errorf("%s: sent %s at %q, want m1 at %q", c.asked, sent.Model, sent.OutputConfig.Effort, c.sent)
		}
	}
}

// a Claude model a provider spells with a dot is listed to Desktop by its
// dashed id, so Claude Code knows it for the Claude model it is, and a turn
// on that id reaches the provider's own spelling (Copilot's claude-opus-5.5
// listed as mythos-magpie-<n> ran as a model Claude Code doesn't know: 32000
// output tokens, effort high, and claude-haiku-4.5 asked for an effort Copilot
// answers with 400)
func TestClaudeDesktopDottedClaudeReachesItsModel(t *testing.T) {
	f := &fake{reply: sse(
		`event: message_start`+"\n"+`data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-opus-5.5","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
		`event: message_stop`+"\n"+`data: {"type":"message_stop"}`)}
	up := setup(t, provider.Anthropic, f)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Anthropic: up.URL,
		Models: []string{"claude-opus-5.5"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetModelEfforts("fake/claude-opus-5.5", []string{"low", "medium", "high"}); err != nil {
		t.Fatal(err)
	}
	listed := claudeLooking(provider.Entry{ID: "fake/claude-opus-5.5", Model: "claude-opus-5.5", Efforts: []string{"low", "medium", "high"}})
	if want := "magpie-" + aliasNumber("fake/claude-opus-5.5") + ".anthropic.claude-opus-5-5"; listed != want {
		t.Fatalf("listed as %q, want %q", listed, want)
	}
	for _, asked := range []string{listed, listed + "[1m]"} {
		body := `{"model":"` + asked + `","max_tokens":32000,"stream":true,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},` +
			`"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		req.Header.Set("x-api-key", Token+"-claude-desktop")
		rec := httptest.NewRecorder()
		New().Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", asked, rec.Code, rec.Body)
		}
		var sent struct {
			Model        string `json:"model"`
			OutputConfig struct {
				Effort string `json:"effort"`
			} `json:"output_config"`
		}
		if err := json.Unmarshal(f.got, &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Model != "claude-opus-5.5" || sent.OutputConfig.Effort != "high" {
			t.Errorf("%s: sent %s at %q, want claude-opus-5.5 at high", asked, sent.Model, sent.OutputConfig.Effort)
		}
	}
}
