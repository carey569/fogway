package gateway

import (
	"testing"

	"github.com/yetone/magpie/internal/usage"
)

// A record kept for an agent on another computer, passed on by its magpie,
// says nothing of whether this computer's agent of the name reached magpie.
func TestLastSeenNotVia(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	usage.Append(usage.Record{Agent: "remote-only-agent", Provider: "fake", Via: "office", Status: 200})
	if !usage.LastSeen("remote-only-agent").IsZero() {
		t.Error("an agent on another computer counted as this one's")
	}
}
