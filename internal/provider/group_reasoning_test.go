package provider

import (
	"testing"
)

// A group of members that don't think reasons for none of them.
func TestGroupReasoningFromNoMember(t *testing.T) {
	groupReasoningHome(t)
	g := groupEntries([]Entry{quiet("q"), quiet("r")})
	if len(g) != 1 {
		t.Fatalf("groups: %+v", g)
	}
	if g[0].Reasoning || len(g[0].Efforts) != 0 {
		t.Fatalf("no member thinks: reasoning %v, levels %v", g[0].Reasoning, g[0].Efforts)
	}
}

// levels are the levels of the member that thinks in these tests.
var levels = []string{"low", "high", "max"}

// thinksMember is a member that thinks, with levels.
func thinksMember() Entry { return thinks("a", levels) }

// thinks is a model that does: its own levels as given. Same model id as
// quietMember's, so the two are members of one group.
func thinks(p string, efforts []string) Entry {
	return Entry{ID: p + "/m", Model: "m", Name: "m", Provider: Provider{ID: p}, Efforts: efforts, Reasoning: true}
}

// quiet is a model that doesn't think: its provider gives no levels for it,
// and nothing says it reasons.
func quiet(p string) Entry {
	return Entry{ID: p + "/m", Model: "m", Name: "m", Provider: Provider{ID: p}}
}

// groupReasoningHome is a machine of its own with a provider whose model
// has levels the user set.
func groupReasoningHome(t *testing.T) {
	t.Helper()
	effortHome(t)
	if err := SetModelEfforts("a/m", levels); err != nil {
		t.Fatal(err)
	}
}
