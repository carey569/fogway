package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/provider"
)

// keptEffortHome is a sandbox HOME with a provider of thinking models: deep
// (none, low, high: fogway's default high) and wide (low, medium, high:
// fogway's default medium).
func keptEffortHome(t *testing.T, models ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("WORKBUDDY_CONFIG_DIR", "")
	t.Setenv("CODEBUDDY_CONFIG_DIR", "")
	t.Setenv("KIMI_SHARE_DIR", "")
	t.Setenv("KIMI_CODE_HOME", "")
	t.Setenv("MINIMAX_DATA_DIR", "")
	keptEffortModels(t, models...)
	return home
}

func keptEffortModels(t *testing.T, models ...string) {
	t.Helper()
	if len(models) == 0 {
		models = []string{"deep", "wide"}
	}
	if err := provider.Save(provider.Provider{ID: "think", Name: "Think", Chat: "https://example.test/v1", Key: "k", Models: models}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("think", "https://example.test/v1", []catalog.Model{
		{ID: "deep", Efforts: []string{"none", "low", "high"}},
		{ID: "wide", Efforts: []string{"low", "medium", "high"}},
		{ID: "fresh", Efforts: []string{"low", "medium", "high"}},
	}); err != nil {
		t.Fatal(err)
	}
}

// Kimi Code's default_effort set by hand on one of fogway's models stays
// through a sync.
func TestKimiKeepsTheUsersDefaultEffort(t *testing.T) {
	home := keptEffortHome(t)
	path := filepath.Join(home, ".kimi-code", "config.toml")
	writeFile(t, path, "")
	a := kimi(home)
	if err := a.Field("model").Set("fogway/think/deep"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	at := strings.Index(string(b), `[models."fogway/think/wide"]`)
	if at < 0 || !strings.Contains(string(b[at:]), `default_effort = "high"`) {
		t.Fatalf("fogway's default: %s", b)
	}
	s := string(b[:at]) + strings.Replace(string(b[at:]), `default_effort = "high"`, `default_effort = "low"`, 1)
	os.WriteFile(path, []byte(s), 0o600)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	var c struct {
		Models map[string]struct {
			DefaultEffort string `toml:"default_effort"`
		}
	}
	b, _ = os.ReadFile(path)
	if err := toml.Unmarshal(b, &c); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if c.Models["fogway/think/wide"].DefaultEffort != "low" || c.Models["fogway/think/deep"].DefaultEffort != "high" {
		t.Fatalf("after sync: %s", b)
	}
}
