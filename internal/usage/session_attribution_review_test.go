package usage

import (
	"path/filepath"
	"testing"
)

func sessionHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_CACHE_HOME": filepath.Join(home, ".cache"), "CLAUDE_CONFIG_DIR": filepath.Join(home, ".claude"), "CODEX_HOME": filepath.Join(home, ".codex"), "OPENAI_BASE_URL": ""} {
		t.Setenv(k, v)
	}
	return home
}
