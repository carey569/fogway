package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/settings"
	"github.com/carey569/fogway/internal/usage"
)

// codexTitleRequest is a title request as Codex's TUI sends one on its own
// model, signed in to ChatGPT: a hidden thread_title turn, its schema in
// text.format; with subagent, as the app names one in x-openai-subagent.
func codexTitleRequest(t *testing.T, s *Server, subagent string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"gpt-5.6-luna","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Generate a concise title. User prompt:\nfix the login bug"}]}],` +
		`"text":{"format":{"type":"json_schema","name":"codex_output_schema","strict":true,"schema":{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", CodexPath+"/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("session_id", "title-thread")
	if subagent != "" {
		req.Header.Set("x-openai-subagent", subagent)
	} else {
		req.Header.Set("x-codex-turn-metadata", `{"thread_source":"thread_title","parent_thread_id":"main-chat"}`)
	}
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// outputText is the text of the message items in a Responses stream's
// response.completed.
func outputText(t *testing.T, body string) (string, int) {
	t.Helper()
	for _, ev := range events(body) {
		if ev["type"] != "response.completed" {
			continue
		}
		resp, _ := ev["response"].(map[string]any)
		out, _ := resp["output"].([]any)
		var text strings.Builder
		for _, it := range out {
			m, _ := it.(map[string]any)
			cs, _ := m["content"].([]any)
			for _, c := range cs {
				cm, _ := c.(map[string]any)
				s, _ := cm["text"].(string)
				text.WriteString(s)
			}
		}
		return text.String(), len(out)
	}
	t.Fatalf("no response.completed in %q", body)
	return "", 0
}

// codexProviderTitle is a title request as Codex's TUI sends one with
// fogway as its model provider (model_provider = "fogway", #743): to
// fogway's own /v1/responses, on the conversation's model, a fogway id —
// or, with backend, to the ChatGPT backend path on that fogway model.
func codexProviderTitle(t *testing.T, s *Server, backend bool) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"fake/m1","stream":true,"reasoning":{"effort":"low"},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Generate a concise title. User prompt:\nfix the login bug"}]}],` +
		`"text":{"format":{"type":"json_schema","name":"codex_output_schema","strict":true,"schema":{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":36}},"required":["title"],"additionalProperties":false}}}}`
	path := "/v1/responses"
	if backend {
		path = CodexPath + "/responses"
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("session_id", "title-thread")
	req.Header.Set("x-codex-turn-metadata", `{"thread_source":"thread_title","parent_thread_id":"main-chat"}`)
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestTitleJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"title":"Fix login bug"}`:                      `{"title":"Fix login bug"}`,
		"```json\n{\"title\": \"Fix login\"}\n```":       `{"title":"Fix login"}`,
		`"Fix login bug"`:                                `{"title":"Fix login bug"}`,
		"Title: Fix login\n\nmore words":                 `{"title":"Fix login"}`,
		"**修复登录问题**":                                     `{"title":"修复登录问题"}`,
		"<think>\nthe user wants\n</think>\n\nFix login": `{"title":"Fix login"}`,
		"":             "",
		`{"name":"x"}`: "",
	} {
		if got := titleJSON(in, titleShape{}); got != want {
			t.Errorf("titleJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

// #743: a Codex whose title schema requires a description beside the
// title, no other field allowed, is given both, each in its bounds; the
// model's own description when its JSON has one.
func TestTitleJSONFillsTheSchema(t *testing.T) {
	body := `{"text":{"format":{"type":"json_schema","strict":true,"schema":{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":36},"description":{"type":"string","minLength":1}},"required":["title","description"],"additionalProperties":false}}}}`
	shape := titleShapeOf([]byte(body))
	for in, want := range map[string]string{
		"Fix login bug": `{"description":"Fix login bug","title":"Fix login bug"}`,
		`{"title":"Fix login","description":"The login form rejects valid passwords"}`: `{"description":"The login form rejects valid passwords","title":"Fix login"}`,
		"Fix the login bug that rejects every valid password on mobile":                `{"description":"Fix the login bug that rejects every valid password on mobile","title":"Fix the login bug that rejects every"}`,
		"": "",
	} {
		if got := titleJSON(in, shape); got != want {
			t.Errorf("titleJSON(%q) = %s, want %s", in, got, want)
		}
	}
	if got := titleJSON("Fix login", titleShapeOf([]byte(`{}`))); got != `{"title":"Fix login"}` {
		t.Errorf("no schema: %s", got)
	}
}

func codexDescriptionRequest(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	// ChatGPT.app's thread_description schema: description is required and has
	// a minimum length, but no maximum.
	body := `{"model":"gpt-6-luna","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Describe the thread"}]}],` +
		`"text":{"format":{"type":"json_schema","strict":true,"schema":{"type":"object","properties":{"description":{"type":"string","minLength":1}},"required":["description"],"additionalProperties":false}}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", CodexPath+"/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("session_id", "description-thread")
	req.Header.Set("x-codex-turn-metadata", `{"thread_source":"thread_description"}`)
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestCodexDescriptionFallbackOnFogway(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/backend-api/codex/responses"} {
		t.Run(path, func(t *testing.T) {
			f := &fake{t: t, reply: sse(
				`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"Plain description"}}]}`,
				`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`,
				`data: [DONE]`)}
			setup(t, provider.Chat, f)
			s := New()
			body := `{"model":"fake/m1","stream":true,"input":"Describe the thread",` +
				`"text":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"description":{"type":"string","minLength":1}},"required":["description"]}}}}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("x-codex-turn-metadata", `{"thread_source":"thread_description"}`)
			s.Handler().ServeHTTP(rec, req)
			if text, _ := outputText(t, rec.Body.String()); rec.Code != 200 || text != `{"description":"Plain description"}` || f.calls != 1 {
				t.Fatalf("fallback: %d %q, calls %d", rec.Code, rec.Body.String(), f.calls)
			}
		})
	}
}

func TestDescriptionJSON(t *testing.T) {
	for in, want := range map[string]string{
		"A short note":                               `{"description":"A short note"}`,
		`{"description":"From JSON"}`:                `{"description":"From JSON"}`,
		"```json\n{\"description\":\"Fenced\"}\n```": `{"description":"Fenced"}`,
		"": "",
	} {
		if got := descriptionJSON(in); got != want {
			t.Errorf("descriptionJSON(%q) = %s, want %s", in, got, want)
		}
	}
}

// A title Codex asks one of fogway's models for is one it can read
// (#743): with fogway as Codex's provider the request comes to /v1, where
// the Codex titles setting is heeded too, and with no model set there the
// conversation's model's plain answer — a Chat model is never shown the
// schema — is handed back as {"title": …}, as it is on the ChatGPT path.
// An answer with no title in it is in the Routing and Usage views as the
// failure it is to Codex.
func TestCodexTitleOnFogwayProvider(t *testing.T) {
	answer := "Fix login bug"
	f := &fake{t: t}
	setup(t, provider.Chat, f)
	reply := func() string {
		return sse(
			`data: {"id":"c1","choices":[{"index":0,"delta":{"content":`+strconvQuote(answer)+`}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":4}}`,
			`data: [DONE]`)
	}
	save := func(v string) {
		t.Helper()
		st := settings.Load()
		st.CodexTitles = v
		if err := settings.Save(st); err != nil {
			t.Fatal(err)
		}
	}
	s := New()
	for _, backend := range []bool{false, true} {
		f.reply = reply()
		rec := codexProviderTitle(t, s, backend)
		if text, _ := outputText(t, rec.Body.String()); rec.Code != 200 || text != `{"title":"Fix login bug"}` {
			t.Fatalf("backend %v: %d %q", backend, rec.Code, rec.Body.String())
		}
	}
	calls := f.calls

	// the setting holds on /v1 as well
	save("off")
	rec := codexProviderTitle(t, s, false)
	if text, n := outputText(t, rec.Body.String()); rec.Code != 200 || n != 0 || text != "" || f.calls != calls {
		t.Fatalf("off on /v1: %d %q, upstream calls %d", rec.Code, rec.Body.String(), f.calls-calls)
	}
	save("")

	// a turn of the conversation on /v1 is left as it was
	f.reply = reply()
	if code, body := post(t, "/v1/responses", `{"model":"fake/m1","stream":true,"input":"hi"}`); code != 200 || strings.Contains(body, `{\"title\"`) {
		t.Fatalf("a plain turn: %d %q", code, body)
	}

	// no title in the answer: none for Codex, and the call says why
	answer = ""
	f.reply = reply()
	rec = codexProviderTitle(t, s, false)
	if text, n := outputText(t, rec.Body.String()); rec.Code != 200 || n != 0 || text != "" {
		t.Fatalf("no title: %d %q", rec.Code, rec.Body.String())
	}
	var last usage.Record
	for _, u := range usage.Load(time.Time{}) {
		last = u
	}
	if last.Kind != "thread_title" || !strings.HasPrefix(last.Error, "title: ") || !last.Failed() {
		t.Errorf("the ledger's no-title call %+v", last)
	}
	st := s.Trace(t.Context(), 0, 0)
	var r Route
	for _, c := range st.Routes {
		if c.Seq > r.Seq {
			r = c
		}
	}
	if !isTitleKind(r.Kind) || !strings.HasPrefix(r.Error, "title: ") {
		t.Errorf("the Routing view's no-title call %+v", r)
	}
}

func TestNoDescription(t *testing.T) {
	f := &fake{t: t, reply: sse(
		`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"   "}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`,
		`data: [DONE]`)}
	setup(t, provider.Chat, f)
	st := settings.Load()
	st.CodexDescriptions = "fake/m1"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	s := New()
	rec := codexDescriptionRequest(t, s)
	text, n := outputText(t, rec.Body.String())
	if rec.Code != 200 || n != 0 || text != "" {
		t.Fatalf("empty reply: %d %q, %d items", rec.Code, rec.Body.String(), n)
	}
	var last usage.Record
	for _, u := range usage.Load(time.Time{}) {
		last = u
	}
	if last.Kind != "thread_description" || !strings.HasPrefix(last.Error, "description: ") || !last.Failed() {
		t.Fatalf("unusable reply was not flagged: %+v", last)
	}
}
