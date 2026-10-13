package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A Vertex AI provider with no project (providers.json edited by hand)
// says in its row that it needs one, as the app's does, not that Google
// credentials sign it: nothing is asked, and no agent has its models.
func TestTUIVertexWithoutProject(t *testing.T) {
	home(t)
	if err := os.MkdirAll(filepath.Dir(provider.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provider.Path(), []byte(`{"providers":[{"id":"vertex-bare","name":"Vertex Bare","preset":"google-vertex"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := press(t, model{w: 160, h: 40}, "2")
	m.reloadProviders()
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, " vertex-bare ") {
			if !strings.Contains(l, "○ needs a project") || strings.Contains(l, "Google credentials") {
				t.Errorf("its row: %q", l)
			}
			return
		}
	}
	t.Fatalf("no row for vertex-bare:\n%s", m.View())
}
