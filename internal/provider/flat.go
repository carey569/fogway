package provider

import "strings"

// FlatSep stands for each slash of a catalog id in its flat spelling.
//
// MiniMax Code takes a SubAgent's model only as "<provider>/<model>" with
// one slash in the whole (/^[^/\s]+\/[^/\s]+$/, #1387), and its provider
// for magpie is "custom_provider:magpie", so a model under it can't hold a
// slash: "bb-codex/gpt-6.1-sol" is written there as "bb-codex~gpt-6.1-sol",
// and the gateway takes that back to the catalog id (Unflat). A tilde is in
// no id a provider was seen to serve but OpenRouter's "~anthropic/…"
// latest-aliases, which flatten to "openrouter~~anthropic~…", still one
// entry's.
const FlatSep = "~"

// FlatID is a catalog id with no slash in it.
func FlatID(id string) string { return strings.ReplaceAll(id, "/", FlatSep) }

// Unflat is the served id (a provider's model or a routing group) whose
// flat spelling is id, when exactly one has it. An id with a slash, or
// with no tilde, is never a flat one.
func Unflat(id string) (string, bool) {
	if !strings.Contains(id, FlatSep) || strings.Contains(id, "/") {
		return "", false
	}
	return unflatIn(Served(), id)
}

func unflatIn(entries []Entry, id string) (string, bool) {
	found := ""
	for _, e := range entries {
		if e.ID == found || FlatID(e.ID) != id {
			continue
		}
		if found != "" {
			return "", false // two ids flatten alike: neither is meant
		}
		found = e.ID
	}
	return found, found != ""
}
