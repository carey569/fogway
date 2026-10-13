package agent

import (
	"path/filepath"
	"testing"

	"github.com/fogway/fogway/internal/edit"
	"github.com/fogway/fogway/internal/provider"
)

// TestClaudeAutoModeReview: Claude Code in auto mode behind fogway on a
// model Anthropic doesn't review (1EFLOPS on X, on Codex's gpt-6.1-sol)
// said "this session isn't eligible because your requests go through
// 127.0.0.1:3725…". Anthropic's page says to turn the review off
// (CLAUDE_CODE_AUTO_MODE_SERVER=0) for a gateway that can't run it: fogway
// does while no model Claude Code can reach through it is reviewed, and
// leaves it on while one is, shown or behind a group or a fallback. A
// value of the user's is theirs, and fogway's goes when Claude Code is
// taken off fogway.
func TestClaudeAutoModeReview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{ID: "v", Name: "V", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"gpt-x"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, `{"theme":"dark"}`)
	a := claude(home)
	review := func() string { v, _ := edit.GetJSON(path, "env."+claudeReviewEnv); return v }
	set := func(key, v string) {
		t.Helper()
		if err := a.Field(key).Set(v); err != nil {
			t.Fatal(err)
		}
	}
	sync := func() {
		t.Helper()
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
	}

	// only a model no one reviews: off
	set("model", "v/gpt-x")
	if review() != "0" {
		t.Fatalf("on v/gpt-x alone: %q, want 0", review())
	}
	sync()
	if review() != "0" {
		t.Fatalf("Sync: %q", review())
	}

	// a Claude model on an Anthropic endpoint joins: on, as /model can take
	// the session there
	if err := provider.Save(provider.Provider{ID: "a", Name: "A", Anthropic: "http://127.0.0.1:1", Key: "k", Models: []string{"claude-sonnet-5"}}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "" {
		t.Fatalf("with a/claude-sonnet-5 shown: %q, want unset", review())
	}

	// hidden from Claude Code: off again; reachable through a group shown
	// to it, or a provider's fallback, on
	if err := provider.SetHiddenModels("claude", []string{"a/claude-sonnet-5"}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "0" {
		t.Fatalf("Claude hidden: %q, want 0", review())
	}
	if err := provider.SaveGroup(provider.Group{Name: "mix", Members: []string{"v/gpt-x", "a/claude-sonnet-5"}}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "" {
		t.Fatalf("a group with a Claude member: %q, want unset", review())
	}
	// a group that keeps its models out of the lists (#1783) still routes
	// to them, so its Claude member counts
	if err := provider.SetHiddenModels("claude", nil); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{Name: "mix", Members: []string{"v/gpt-x", "a/claude-sonnet-5"}, Unlists: true}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "" {
		t.Fatalf("a group unlisting its Claude member: %q, want unset", review())
	}
	if err := provider.SetHiddenModels("claude", []string{"a/claude-sonnet-5"}); err != nil {
		t.Fatal(err)
	}
	if err := provider.DeleteGroup("mix"); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "0" {
		t.Fatalf("group deleted: %q, want 0", review())
	}
	if err := provider.Save(provider.Provider{ID: "v", Name: "V", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"gpt-x"}, Fallback: []string{"a/claude-sonnet-5"}}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "" {
		t.Fatalf("falling back to Claude: %q, want unset", review())
	}
	if err := provider.Save(provider.Provider{ID: "v", Name: "V", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"gpt-x"}}); err != nil {
		t.Fatal(err)
	}
	sync()
	if review() != "0" {
		t.Fatalf("fallback gone: %q, want 0", review())
	}

	// off fogway, onto Claude Code's own model, and reset: fogway's goes
	set("model", "claude-opus-4-8")
	if review() != "" {
		t.Fatalf("Claude's own model kept fogway's value: %q", review())
	}
	set("model", "v/gpt-x")
	if review() != "0" {
		t.Fatalf("back on v/gpt-x: %q", review())
	}
	set("model", "")
	if review() != "" {
		t.Fatalf("reset kept fogway's value: %q", review())
	}

	// the user's own value is theirs, through a switch, Sync and reset
	for _, mine := range []string{"1", "0"} {
		if err := edit.SetJSON(path, edit.KV{Path: "env." + claudeReviewEnv, Value: mine}); err != nil {
			t.Fatal(err)
		}
		set("model", "v/gpt-x")
		sync()
		if err := provider.Save(provider.Provider{ID: "a", Name: "A", Anthropic: "http://127.0.0.1:1", Key: "k", Models: []string{"claude-sonnet-5"}}); err != nil {
			t.Fatal(err)
		}
		if err := provider.SetHiddenModels("claude", nil); err != nil {
			t.Fatal(err)
		}
		sync()
		if review() != mine {
			t.Fatalf("the user's %q became %q", mine, review())
		}
		set("model", "")
		if review() != mine {
			t.Fatalf("reset took the user's %q away: %q", mine, review())
		}
		if err := edit.DelJSON(path, "env."+claudeReviewEnv); err != nil {
			t.Fatal(err)
		}
		if err := provider.SetHiddenModels("claude", []string{"a/claude-sonnet-5"}); err != nil {
			t.Fatal(err)
		}
	}
	if v, _ := edit.GetJSON(path, "theme"); v != "dark" {
		t.Fatal("theme lost")
	}
}
