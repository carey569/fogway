package agent

import (
	"path/filepath"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// listsSandbox is a HOME of its own with one provider of two models.
func listsSandbox(t *testing.T) (home, cfg string) {
	t.Helper()
	home = t.TempDir()
	cfg = filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"pro", "flash"}}); err != nil {
		t.Fatal(err)
	}
	return home, cfg
}
