package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/edit"
)

// Codex signed in with ChatGPT, connected on one of fogway's models beside
// its sign-in, stays connected when Codex writes one of its own models in
// (its /model, or a Codex app still running on its old pick): its own
// models are in its list beside fogway's, as when it joined (#940: Codex
// read as not connected then, its config still on fogway's gateway).
// Disconnect leaves it on that model.
func TestCodexOwnPickAfterFogwayModelStaysConnected(t *testing.T) {
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "model = \"gpt-5.5\"\n")
	cx := codex(home)
	if err := cx.Apply("model", "fake/m1"); err != nil {
		t.Fatal(err)
	}
	if !cx.Wired() {
		t.Fatalf("not connected on fake/m1:\n%s", read())
	}
	if err := edit.SetTOMLTop(filepath.Join(home, ".codex", "config.toml"), edit.KV{Path: "model", Value: "gpt-6.1-sol"}); err != nil {
		t.Fatal(err)
	}
	if !cx.Wired() {
		t.Fatalf("Codex's own pick disconnected it:\n%s", read())
	}
	// told as a change from outside, the row still connected
	if d := cx.Drift(); d == nil || d.Kind != "replaced" || d.Now != "gpt-6.1-sol" {
		t.Fatalf("drift: %+v", d)
	}
	if err := cx.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); cx.Wired() || strings.Contains(cfg, "openai_base_url") || !strings.Contains(cfg, `model = "gpt-6.1-sol"`) {
		t.Fatalf("disconnected:\n%s", cfg)
	}
}
