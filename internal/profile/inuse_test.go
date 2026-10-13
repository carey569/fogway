package profile

import (
	"testing"

	"github.com/fogway/fogway/internal/agent"
)

// inUse is which of the saved profiles InUse names, read as the GUI's state
// reads them: every detected agent's values once, passed in.
func inUse(t *testing.T) []string {
	t.Helper()
	detected := agent.Detected()
	now := map[string]map[string]string{}
	for _, a := range detected {
		now[a.ID] = a.Values()
	}
	ps := must[map[string]Profile](t)(Load())
	var out []string
	for _, n := range Names(ps) {
		if InUse(ps[n], detected, now) {
			out = append(out, n)
		}
		// read here, the same answer (the TUI and the CLI pass none)
		if InUse(ps[n], detected, now) != InUse(ps[n], detected, nil) {
			t.Errorf("%s: in use with the values passed differs from read", n)
		}
	}
	return out
}

func setModel(t *testing.T, id, model string) {
	t.Helper()
	for _, a := range agent.Detected() {
		if a.ID == id {
			if err := a.Apply("model", model); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no %s", id)
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
