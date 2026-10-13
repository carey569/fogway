package gateway

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// uaVendor is a key+URL vendor on every API magpie forwards to one: it
// keeps the User-Agent each path was asked with, and answers each as its
// API does.
type uaVendor struct {
	mu  sync.Mutex
	got map[string]string
}

func (v *uaVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	v.mu.Lock()
	v.got[r.URL.Path] = r.Header.Get("User-Agent")
	v.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch p := r.URL.Path; {
	case strings.HasSuffix(p, "/count_tokens"):
		io.WriteString(w, `{"input_tokens":5}`)
	case p == "/v1/messages":
		io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"m1","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	case p == "/v1/chat/completions":
		io.WriteString(w, `{"id":"c","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	case p == "/v1/responses":
		io.WriteString(w, `{"id":"r1","object":"response","status":"completed","model":"m1","output":[{"type":"message","id":"msg1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hi","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	case strings.HasSuffix(p, ":streamGenerateContent"):
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`+"\n\n")
	case p == "/v1/embeddings":
		io.WriteString(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,-0.2]}],"model":"embed-1","usage":{"prompt_tokens":1,"total_tokens":1}}`)
	case p == "/v1/images/generations":
		io.WriteString(w, `{"created":1,"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(pngBytes)+`"}]}`)
	default:
		w.WriteHeader(404)
	}
}

func (v *uaVendor) sent(path string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	ua, ok := v.got[path]
	delete(v.got, path)
	return ua, ok
}

// uaRoutes are the routes a request reaches a key+URL provider by: what an
// agent posts magpie, and the path the vendor is asked at.
var uaRoutes = []struct{ name, path, body, upstream string }{
	{"anthropic", "/v1/messages", `{"model":"ua/m1","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, "/v1/messages"},
	{"count_tokens", "/v1/messages/count_tokens", `{"model":"ua/m1","messages":[{"role":"user","content":"hi"}]}`, "/v1/messages/count_tokens"},
	{"chat", "/v1/chat/completions", `{"model":"ua/m1","messages":[{"role":"user","content":"hi"}]}`, "/v1/chat/completions"},
	{"responses", "/v1/responses", `{"model":"ua/m1","input":"hi"}`, "/v1/responses"},
	{"gemini", "/v1beta/models/ug/m1:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`, "/v1beta/models/m1:streamGenerateContent"},
	{"embeddings", "/v1/embeddings", `{"model":"ua/embed-1","input":"hi"}`, "/v1/embeddings"},
	{"images", "/v1/images/generations", `{"model":"ua/gpt-image-1","prompt":"a magpie"}`, "/v1/images/generations"},
}

// uaSetup saves a key+URL provider "ua" on every API, at a uaVendor, with
// pass for its PassUserAgent and headers for the user's own, and "ug" the
// same on Gemini's API alone.
func uaSetup(t *testing.T, pass bool, headers map[string]string) *uaVendor {
	t.Helper()
	fresh(t)
	v := &uaVendor{got: map[string]string{}}
	up := httptest.NewServer(v)
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "ua", Name: "UA", Key: "k", Chat: up.URL + "/v1", Responses: up.URL + "/v1", Anthropic: up.URL, Gemini: up.URL + "/v1beta",
		Models: []string{"m1", "embed-1", "gpt-image-1"}, PassUserAgent: pass, Headers: headers}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("ua", up.URL+"/v1", []catalog.Model{{ID: "m1"}, {ID: "embed-1"}, {ID: "gpt-image-1"}}); err != nil {
		t.Fatal(err)
	}
	// Gemini's own wire, which a provider on Chat too isn't asked on
	g := provider.Provider{ID: "ug", Name: "UG", Key: "k", Gemini: up.URL + "/v1beta", Models: []string{"m1"}, PassUserAgent: pass, Headers: headers}
	if err := provider.Save(g); err != nil {
		t.Fatal(err)
	}
	return v
}

// askAs posts body to path as an agent whose User-Agent is ua ("" sends
// none).
func askAs(t *testing.T, s *Server, path, body, ua string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("User-Agent")
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s: status %d: %s", path, rec.Code, rec.Body)
	}
}

// Claude Code's and Codex's own headers still go on to a key+URL provider
// as before (#179, #1758), whether it passes the agent's User-Agent or
// not: the passing on is the same User-Agent.
func TestUserAgentClaudeCodeAsBefore(t *testing.T) {
	const cc = "claude-cli/2.1.0 (external, cli)"
	for _, pass := range []bool{false, true} {
		v := uaSetup(t, pass, nil)
		askAs(t, New(), "/v1/messages", uaRoutes[0].body, cc)
		if got, _ := v.sent("/v1/messages"); got != cc {
			t.Errorf("pass %v: Claude Code's request went as %q", pass, got)
		}
	}
}
