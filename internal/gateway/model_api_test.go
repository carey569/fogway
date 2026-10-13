package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fogway/fogway/internal/provider"
	"github.com/tidwall/gjson"
)

// A custom provider with a Gemini URL beside its chat and Anthropic ones
// (Kayphoon on Discord: 供应商为自定义的时候已经兼容 gemini 格式了，但模型的
// 详细配置里还是只能限定 chat 和 anthropic): with Gemini picked for a model,
// a chat, a Messages and a Gemini client's request for it each go to
// models/{model}:streamGenerateContent under the Gemini URL, with the key
// in x-goog-api-key, and chat completions and Messages are never tried;
// the provider's other model stays on chat.
func TestModelAPIGemini(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	var bodies [][]byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		model := gjson.GetBytes(b, "model").String()
		mu.Lock()
		hits = append(hits, r.URL.RequestURI()+" "+model+" "+r.Header.Get("x-goog-api-key"))
		bodies = append(bodies, b)
		mu.Unlock()
		switch {
		case r.URL.Path == "/v1/chat/completions" && model == "plain":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, sse(`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"from chat"}}]}`,
				`data: {"id":"c","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
				`data: [DONE]`))
		case r.URL.Path == "/v1beta/models/gm:streamGenerateContent":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, sse(
				`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"from "}]}}],"modelVersion":"gm"}`,
				`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`,
			))
		default:
			http.Error(w, `{"error":{"message":"no available channel for model `+model+` under group default"}}`, 503)
		}
	}))
	defer up.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k-1", Chat: up.URL + "/v1", Anthropic: up.URL, Gemini: up.URL + "/v1beta", Models: []string{"gm", "plain"}}); err != nil {
		t.Fatal(err)
	}
	// as the editor's Save sends it
	gem := "gemini"
	if err := provider.SetModelPrefs("relay", map[string]provider.ModelPref{"gm": {API: &gem}}); err != nil {
		t.Fatalf("Gemini picked for a model: %v", err)
	}
	gw := httptest.NewServer(New().Handler())
	defer gw.Close()
	const toGemini = "/v1beta/models/gm:streamGenerateContent?alt=sse  k-1"
	for _, c := range []struct{ path, body, want, hit string }{
		{"/v1/chat/completions", `{"model":"relay/gm","stream":true,"messages":[{"role":"user","content":"hi there"}]}`, "gemini", toGemini},
		{"/v1/chat/completions", `{"model":"relay/gm","messages":[{"role":"user","content":"hi there"}]}`, "from gemini", toGemini},
		{"/v1/messages", `{"model":"relay/gm","max_tokens":64,"messages":[{"role":"user","content":"hi there"}]}`, "from gemini", toGemini},
		{"/v1beta/models/relay/gm:streamGenerateContent?alt=sse", `{"contents":[{"role":"user","parts":[{"text":"hi there"}]}]}`, "gemini", toGemini},
		{"/v1/chat/completions", `{"model":"relay/plain","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "from chat", "/v1/chat/completions plain "},
	} {
		mu.Lock()
		hits, bodies = nil, nil
		mu.Unlock()
		res, err := http.Post(gw.URL+c.path, "application/json", strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		mu.Lock()
		h, bs := append([]string(nil), hits...), append([][]byte(nil), bodies...)
		mu.Unlock()
		if res.StatusCode != 200 || !strings.Contains(string(b), c.want) {
			t.Errorf("%s %s: %d %s (upstream %v)", c.path, c.body, res.StatusCode, b, h)
			continue
		}
		if len(h) != 1 || h[0] != c.hit {
			t.Errorf("%s %s: upstream asked %q, want only %q", c.path, c.body, h, c.hit)
			continue
		}
		if c.hit == toGemini && !strings.Contains(gjson.GetBytes(bs[0], "contents").Raw, "hi there") {
			t.Errorf("%s: Gemini body %s", c.path, bs[0])
		}
	}
}
