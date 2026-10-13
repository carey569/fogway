package usage

import (
	"testing"
)

// Every request fogway sends by itself is fogway's own: the classifier, the
// search, the image description; an agent's is not.
func TestByFogway(t *testing.T) {
	for ua, want := range map[string]bool{"fogway-router/1": true, "fogway-search/1": true, "fogway-vision/1": true, "fogway": true, "claude-cli/2.1": false, "codex_cli_rs/0.50": false, "": false} {
		if got := (Record{Agent: ua}).ByFogway(); got != want {
			t.Errorf("%q: %v, want %v", ua, got, want)
		}
	}
}
