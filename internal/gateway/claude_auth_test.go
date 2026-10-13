package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/testenv"
)

// claudeRefuses has the fake Claude Code fail as Claude Code does on a
// sign-in Anthropic refused, on the accounts whose token is one of toks
// (all of them with none), and answer on the others.
func claudeRefuses(t *testing.T, calls string, toks ...string) {
	t.Helper()
	refused := `[ -z "` + strings.Join(toks, "") + `" ]`
	for _, tok := range toks {
		refused += ` || [ "$tok" = "` + tok + `" ]`
	}
	script := `#!/bin/sh
case "$1" in auth) exit 1;; esac
creds="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/.credentials.json"
while read -r line; do
  tok=$(grep -o 'tok-[a-z]*' "$creds" | head -1)
  echo "$tok" >> '` + calls + `'
  if ` + refused + `; then
    echo '{"type":"result","is_error":true,"result":"Failed to authenticate: OAuth session expired and could not be refreshed"}'
  else
    echo '{"type":"stream_event","event":{"type":"message_start","message":{"id":"m","model":"claude-sonnet-5","usage":{"input_tokens":1}}}}'
    echo '{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"healthy account"}}}'
    echo '{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}}'
    echo '{"type":"stream_event","event":{"type":"message_stop"}}'
    echo '{"type":"result","is_error":false,"result":""}'
  fi
done
`
	binary := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	testenv.Program(t, filepath.Join(binary, "claude"), script)
}

func askClaudeModel(s *Server, text string) *httptest.ResponseRecorder {
	body := `{"model":"claude/claude-sonnet-5","max_tokens":100,"messages":[{"role":"user","content":"` + text + `"}]}`
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
	return rec
}

// Another vendor saying a Claude sign-in's words (an OpenCode plugin on
// Anthropic's OAuth: "OAuth access token has been revoked") isn't a Claude
// account Anthropic refused: it rests as any failure does, and isn't told
// as needing a sign-in that magpie would never try again (#716 review).
func TestSignInWordsFromAnotherVendorRest(t *testing.T) {
	fresh(t)
	revoked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"OAuth access token has been revoked"}}`)
	})
	serveOn(t, "a", "ka", []string{"m"}, revoked)
	serveOn(t, "b", "kb", []string{"m"}, &keyed{})
	if err := provider.SaveGroup(provider.Group{Name: "Two", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	if code, body := postAs(t, s, "", `{"model":"group/two","messages":[{"role":"user","content":"hi"}]}`); code != 200 || !strings.Contains(body, "from kb") {
		t.Fatalf("%d %s", code, body)
	}
	if r := lastRoute(s); len(r.Tries) != 2 || r.Tries[0].Fail == failAuth || r.Tries[0].Rest == nil {
		t.Fatalf("not rested as before: %+v", r.Tries)
	}
}
