package usage

import (
	"path/filepath"
	"testing"

	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/sessions"
)

func sessionHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_CACHE_HOME": filepath.Join(home, ".cache"), "CLAUDE_CONFIG_DIR": filepath.Join(home, ".claude"), "CODEX_HOME": filepath.Join(home, ".codex"), "OPENAI_BASE_URL": ""} {
		t.Setenv(k, v)
	}
	return home
}

func TestSharedWorkspaceDoesNotIdentifyItsMember(t *testing.T) {
	r := &sessionResolver{identities: []provider.SessionIdentity{{Agent: "codex", AccountID: "team-ws", UserID: "u-me", User: "me@example.com"}}}
	for _, user := range []string{"", "colleague"} {
		c := sessions.Call{Agent: "codex", Upstream: "openai", AccountID: "team-ws", UserID: user}
		if got := r.resolve(c); got != ("") {
			t.Fatalf("workspace inferred its member: %+v", got)
		}
	}
}
