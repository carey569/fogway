package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/edit"
	"github.com/carey569/fogway/internal/gateway"
	"github.com/carey569/fogway/internal/settings"
)

// setPort is Settings' gateway port set to p, as the Settings page saves it.
func setPort(t *testing.T, p int) {
	t.Helper()
	s := settings.Load()
	s.Port = p
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
}

// filesWith are the files under home holding s.
func filesWith(home, s string) []string {
	var out []string
	filepath.WalkDir(home, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), s) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func containsAgent(as []*Agent, id string) bool {
	for _, a := range as {
		if a.ID == id {
			return true
		}
	}
	return false
}

// Codex signed in with ChatGPT is joined to fogway beside its own models:
// the move writes its base URL on the new port and keeps the model it was
// on, rather than putting it on one of fogway's.
func TestPortMovesJoinedCodex(t *testing.T) {
	t.Setenv("FOGWAY_ADDR", "")
	home, read := codexHome(t, `{"auth_mode":"chatgpt","tokens":{"access_token":"at","refresh_token":"rt","id_token":"x.e30.x","account_id":"acc"}}`, "model = \"gpt-5.1\"\n")
	setPort(t, 3591)
	cx := codex(home)
	c, err := cx.ConnectHow()
	if err != nil || c.How != "joined" {
		t.Fatalf("not joined: %+v %v\n%s", c, err, read())
	}
	on := OnGateway()
	setPort(t, 3592)
	if _, err := Rewire(on); err != nil {
		t.Fatal(err)
	}
	cfg := read()
	if !strings.Contains(cfg, "127.0.0.1:3592") || strings.Contains(cfg, "127.0.0.1:3591") {
		t.Fatalf("Codex not moved:\n%s", cfg)
	}
	if !strings.Contains(cfg, `model = "gpt-5.1"`) || !cx.Joined() {
		t.Fatalf("Codex's own model lost:\n%s", cfg)
	}
	if d := cx.Drift(); d != nil {
		t.Fatalf("off after the move: %+v", d)
	}
}

// An agent taken off fogway before the port moved is left as it is: it is
// the user's to set again, not the move's.
func TestPortLeavesAnUnwiredAgent(t *testing.T) {
	t.Setenv("FOGWAY_ADDR", "")
	home, read := codexHome(t, "", "")
	setPort(t, 3591)
	cx := codex(home)
	if err := cx.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, ".codex", "config.toml")
	os.WriteFile(cfg, []byte(strings.ReplaceAll(read(), "127.0.0.1:3591", "127.0.0.1:9")), 0o644)
	on := OnGateway()
	setPort(t, 3592)
	if _, err := Rewire(on); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(), "127.0.0.1:3592") {
		t.Fatalf("an agent taken off fogway was moved:\n%s", read())
	}
}

// A fogway FOGWAY_ADDR puts beside the installed one tells the installed
// one's wiring by the port the installed one's Settings have, not 3725.
func TestDriftInstalledFogwayOnItsPort(t *testing.T) {
	home, _ := codexHome(t, "", "")
	setPort(t, 3591)
	t.Setenv("FOGWAY_ADDR", "127.0.0.1:3726")
	cx := codex(home)
	if err := cx.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, ".codex", "config.toml")
	os.WriteFile(cfg, []byte("openai_base_url = \"http://127.0.0.1:3591"+gateway.CodexPath+"\"\nmodel = \"fake/m1\"\n"), 0o644)
	if d := cx.Drift(); d != nil {
		t.Fatalf("the installed fogway's wiring taken for drift: %+v", d)
	}
}

// Moved, an agent still puts back what it had before fogway when switched
// off: its own endpoint and token, not fogway's at the old port, which
// setting it again there would otherwise have noted as its own.
func TestPortKeepsWhatAnAgentHadBefore(t *testing.T) {
	t.Setenv("FOGWAY_ADDR", "")
	home, _ := codexHome(t, "", "")
	managed := claudeManaged
	claudeManaged = func() string { return filepath.Join(home, "managed-settings.json") }
	t.Cleanup(func() { claudeManaged = managed })
	path := filepath.Join(home, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://relay.example","ANTHROPIC_AUTH_TOKEN":"own-token"}}`), 0o644)
	setPort(t, 3591)
	cc := claude(home)
	var want string
	for _, o := range cc.Field("model").Options(cc.Values()) {
		if o.Ref == "fake/m1" {
			want = o.Value
		}
	}
	if err := cc.Apply("model", want); err != nil {
		t.Fatal(err)
	}
	on := OnGateway()
	setPort(t, 3592)
	if _, err := Rewire(on); err != nil {
		t.Fatal(err)
	}
	if u, _ := edit.GetJSON(path, "env.ANTHROPIC_BASE_URL"); u != "http://127.0.0.1:3592" {
		t.Fatalf("Claude Code not moved: %q", u)
	}
	if err := cc.Disconnect(); err != nil {
		t.Fatal(err)
	}
	u, _ := edit.GetJSON(path, "env.ANTHROPIC_BASE_URL")
	tok, _ := edit.GetJSON(path, "env.ANTHROPIC_AUTH_TOKEN")
	if u != "https://relay.example" || tok != "own-token" {
		t.Fatalf("switched off, Claude Code got %q %q back, not its own", u, tok)
	}
}
