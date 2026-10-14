package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/agent"
	"github.com/carey569/fogway/internal/agentenv"
	"github.com/carey569/fogway/internal/provider"
)

// sandbox is a home with Claude Code and Codex in it and nothing on PATH.
func sandbox(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("PATH", "")
	for _, k := range agentenv.Vars {
		t.Setenv(k, "")
	}
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	for _, f := range []string{".claude/settings.json", ".codex/config.toml"} {
		write(t, filepath.Join(h, f), "")
	}
	return h
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// A profiles file of fields alone reads, lists and writes back as it was.
func TestOldProfiles(t *testing.T) {
	sandbox(t)
	old := `{
  "work": {
    "claude.model": "acme/m1",
    "codex.model": "acme/m2"
  }
}
`
	write(t, Path(), old)
	ps := must[map[string]Profile](t)(Load())
	p := ps["work"]
	if p.Fields["claude.model"] != "acme/m1" || len(p.Fields) != 2 {
		t.Fatalf("work: %+v", p)
	}
	if s := LongSummary(p); s != "claude acme/m1 · codex acme/m2" {
		t.Errorf("summary %q", s)
	}
	if err := store(ps); err != nil {
		t.Fatal(err)
	}
	if s := read(t, Path()); s != old {
		t.Errorf("written back as\n%s", s)
	}
}

// A profile can name agents this computer doesn't have: profiles sync
// between computers. Applying one leaves those agents alone, so it never
// writes a config that makes an agent look installed here (qtwaiter on X).
func TestApplySkipsUndetectedAgents(t *testing.T) {
	sandbox(t)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Chat: "https://relay.example.com/v1",
		Key: "synthetic-key", Models: []string{"pro"}}); err != nil {
		t.Fatal(err)
	}
	p := Profile{Fields: map[string]string{"claude.model": "relay/pro"}}
	for _, a := range agent.All() {
		if a.Detected() || a.Field("model") == nil {
			continue
		}
		p.Fields[a.ID+".model"] = "relay/pro"
	}
	if len(p.Fields) < 2 {
		t.Fatal("no undetected agent with a model field")
	}
	_, err := Apply(p)
	if got := must[*agent.Agent](t)(agent.Find("claude")).Field("model").Get(); got != "relay/pro" {
		t.Errorf("claude's model is %q", got)
	}
	for k := range p.Fields {
		a := must[*agent.Agent](t)(agent.Find(strings.TrimSuffix(k, ".model")))
		if a.ID != "claude" && a.Detected() {
			t.Errorf("applying the profile made %s look installed", a.ID)
		}
	}
	if err != nil {
		t.Error(err)
	}
}
