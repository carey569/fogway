package cli

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/local"
	"github.com/yetone/magpie/internal/provider"
)

// magpie local names the catalog, picks a model of it or of the user's
// own, switches it on and off, and switches the edge-cloud group's masking
// once there is one.
func TestLocalCLI(t *testing.T) {
	groupsHome(t)
	out, err := stdoutOf(t, func() error { return localCmd([]string{"local", "models"}) })
	if err != nil || !strings.Contains(out, "qwen3.5-2b") || !strings.Contains(out, "recommended") {
		t.Fatalf("models: %v %s", err, out)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "use", "qwen3.5-4b"}) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := local.Load(); c.Model.ID() != "qwen3.5-4b" || c.On {
		t.Fatalf("use: %+v", c)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "use", "Qwen/Qwen3-0.6B-GGUF", "Qwen3-0.6B-Q8_0.gguf"}) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := local.Load(); c.Model.Repo != "Qwen/Qwen3-0.6B-GGUF" {
		t.Fatalf("use a repo: %+v", c)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "use", "nope"}) }); err == nil {
		t.Fatal("a model not in the catalog was taken")
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "on"}) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := local.Load(); !c.On {
		t.Fatal("not on")
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "mask", "on"}) }); err == nil || !strings.Contains(err.Error(), "edge-cloud group") {
		t.Fatalf("mask without a group: %v", err)
	}
	if err := provider.SaveGroup(provider.Group{ID: provider.HybridGroupID, Name: "Edge-cloud auto", Members: []string{"local/q"}, Hybrid: &provider.Hybrid{Local: "local/q"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "mask", "on"}) }); err != nil {
		t.Fatal(err)
	}
	if g, _, _ := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID); !g.Hybrid.Mask {
		t.Fatal("masking not on")
	}
	out, err = stdoutOf(t, func() error { return localCmd([]string{"local", "status"}) })
	if err != nil || !strings.Contains(out, "masking on") || !strings.Contains(out, "local/qwen3-0.6b-q8_0") {
		t.Fatalf("status: %v %s", err, out)
	}
}
