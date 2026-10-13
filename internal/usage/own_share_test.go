package usage

import (
	"testing"
)

// Every request magpie sends by itself is magpie's own: the classifier, the
// search, the image description; an agent's is not.
func TestByMagpie(t *testing.T) {
	for ua, want := range map[string]bool{"magpie-router/1": true, "magpie-search/1": true, "magpie-vision/1": true, "magpie": true, "claude-cli/2.1": false, "codex_cli_rs/0.50": false, "": false} {
		if got := (Record{Agent: ua}).ByMagpie(); got != want {
			t.Errorf("%q: %v, want %v", ua, got, want)
		}
	}
}
