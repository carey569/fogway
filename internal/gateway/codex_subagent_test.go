package gateway

import (
	"net/http/httptest"
	"strings"
	"sync"
)

// chatgptSeen stands in for the ChatGPT backend, noting each request's
// account and model.
type chatgptSeen struct {
	mu     sync.Mutex
	models []string
}

func (c *chatgptSeen) asked() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.models...)
}

// codexAsks is a request of Codex's, a subagent's when kind is
// collab_spawn, a lead's when it is "".
func codexAsks(s *Server, model, kind, input string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	body := `{"model":"` + model + `","stream":true,"input":[` + input + `]}`
	req := httptest.NewRequest("POST", CodexPath+"/responses", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("chatgpt-account-id", "acct-1")
	req.Header.Set("User-Agent", "codex_cli_rs/0.159.0 (Mac OS 26.2.0; arm64)")
	req.Header.Set("session_id", "worker-1")
	if kind != "" {
		req.Header.Set("x-openai-subagent", kind)
		req.Header.Set("x-codex-parent-thread-id", "lead-1")
	}
	s.Handler().ServeHTTP(rec, req)
	return rec
}
