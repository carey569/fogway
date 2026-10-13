package gateway

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A key's weight is saved with it, the first key's too, and 1 is the
// default left out of the file.
func TestSetKeyWeight(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	if err := provider.Save(provider.Provider{ID: "wk", Name: "WK", Chat: "https://wk.example/v1", Key: "ka", Keys: []provider.KeyAccount{{Key: "kb"}}, Routing: provider.Weighted}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetKeyWeight("wk", provider.KeyID("ka"), 3); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetKeyWeight("wk", provider.KeyID("kb"), 5); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetKeyWeight("wk", provider.KeyID("kb"), -1); err == nil {
		t.Fatal("a weight below 0 was taken")
	}
	p, err := provider.Find("wk")
	if err != nil {
		t.Fatal(err)
	}
	if p.Routing != provider.Weighted {
		t.Fatalf("routing %q", p.Routing)
	}
	ks := p.KeyList()
	if ks[0].Weight != 3 || ks[1].Weight != 5 {
		t.Fatalf("weights %d %d", ks[0].Weight, ks[1].Weight)
	}
	// the weight goes with its key when another goes first
	if err := provider.UseKey("wk", provider.KeyID("kb")); err != nil {
		t.Fatal(err)
	}
	p, _ = provider.Find("wk")
	if p.KeyWeight != 5 || p.Keys[0].Weight != 3 {
		t.Fatalf("after Make first: first %d, other %d", p.KeyWeight, p.Keys[0].Weight)
	}
	if err := provider.SetKeyWeight("wk", provider.KeyID("kb"), 1); err != nil {
		t.Fatal(err)
	}
	p, _ = provider.Find("wk")
	if p.KeyWeight != 0 {
		t.Fatalf("1 kept as %d", p.KeyWeight)
	}
}
