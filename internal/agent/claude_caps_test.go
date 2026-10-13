package agent

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
)

// TestClaudeCapabilitiesSync: a model added to the catalog later reaches
// Claude Code's capabilities with the rest of the catalog.
func TestClaudeCapabilitiesSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{ID: "v", Name: "V", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetModelEfforts("v/a", []string{"high", "xhigh"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, `{}`)
	a := claude(home)
	if err := a.Field("model").Set("v/a"); err != nil {
		t.Fatal(err)
	}
	caps := func() string { v, _ := edit.GetJSON(path, "env."+claudeCapsEnv); return v }
	if caps() != "v/a=effort,xhigh_effort" {
		t.Fatalf("v/a: %q", caps())
	}
	if err := provider.Save(provider.Provider{ID: "w", Name: "W", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetModelEfforts("w/b", []string{"low", "max"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if caps() != "v/a=effort,xhigh_effort;w/b=effort,max_effort" {
		t.Fatalf("after w/b was added: %q", caps())
	}
}
