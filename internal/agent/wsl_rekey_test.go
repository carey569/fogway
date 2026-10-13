package agent

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/access"
)

// Rekey leaves an agent on this machine's loopback as it is: its key
// doesn't follow sharing.
func TestRekeyLeavesLoopback(t *testing.T) {
	t.Setenv("MAGPIE_ADDR", "")
	_, read := codexHome(t, "", "model = \"gpt-5.5\"\n")
	var as []*Agent
	for _, a := range All() {
		if a.ID == "codex" {
			as = append(as, a)
		}
	}
	if len(as) != 1 {
		t.Fatalf("codex: %d", len(as))
	}
	if err := access.ConfigureLAN(true, false); err != nil {
		t.Fatal(err)
	}
	if moved, err := Rekey(as); err != nil || len(moved) != 0 {
		t.Fatalf("moved %v, %v", moved, err)
	}
	if cfg := read(); strings.Contains(cfg, "magpie") {
		t.Fatalf("written:\n%s", cfg)
	}
}
