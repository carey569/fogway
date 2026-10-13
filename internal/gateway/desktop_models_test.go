package gateway

import (
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// desktopNames: the agents' list naming (provider.Labels, the Provider in
// model names setting) with the whole catalog id after two entries that
// still read the same — Desktop's menu shows the name alone, so two models
// of one provider named alike stay apart there.
func TestDesktopNames(t *testing.T) {
	fresh(t)
	es := []provider.Entry{
		{ID: "a/x", Name: "X", Provider: provider.Provider{ID: "a", Name: "A"}},
		{ID: "b/x", Name: "x", Provider: provider.Provider{ID: "b", Name: "B"}},
		{ID: "c/y", Name: "Y", Provider: provider.Provider{ID: "c", Name: "C"}},
		{ID: "d/z", Name: "Z", Provider: provider.Provider{ID: "d", Name: "D"}},
		{ID: "d/z2", Name: "Z", Provider: provider.Provider{ID: "d", Name: "D"}},
	}
	if got, want := desktopNames(es), []string{"X · A", "x · B", "Y · C", "Z · D (d/z)", "Z · D (d/z2)"}; !slices.Equal(got, want) {
		t.Fatalf("desktopNames = %q, want %q", got, want)
	}
	if err := provider.SetSuffixMode(provider.SuffixOff); err != nil {
		t.Fatal(err)
	}
	if got, want := desktopNames(es), []string{"X · A", "x · B", "Y", "Z · D (d/z)", "Z · D (d/z2)"}; !slices.Equal(got, want) {
		t.Fatalf("desktopNames (names plain) = %q, want %q", got, want)
	}
}
