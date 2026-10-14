package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/local"
	"github.com/carey569/fogway/internal/provider"
)

// fogway local names the catalog, picks a model of it or of the user's
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
	// its window: up to the 256K Qwen3.5 was trained for, as 16k or 1m
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "context", "1m"}) }); err == nil || !strings.Contains(err.Error(), "256K") {
		t.Fatalf("a window past the model's own: %v", err)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "context", "lots"}) }); err == nil {
		t.Fatal("took a window that isn't one")
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "context", "16k"}) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := local.Load(); c.Context != 16<<10 {
		t.Fatalf("context: %+v", c)
	}
	out, err = stdoutOf(t, func() error { return localCmd([]string{"local", "context"}) })
	if err != nil || !strings.Contains(out, "Qwen3.5 4B runs at 16K") || !strings.Contains(out, "▸ 16K") || !strings.Contains(out, "256K") || strings.Contains(out, "512K") {
		t.Fatalf("context: %v %s", err, out)
	}
	if _, err := stdoutOf(t, func() error {
		return localCmd([]string{"local", "use", "Qwen/Qwen3-0.6B-GGUF", "Qwen3-0.6B-Q8_0.gguf"})
	}); err != nil {
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

// fogway local check measures this computer and names the model for it;
// local on with no model of its own keeps the one recommended; and setup
// with none named stops where none is worth running, saying why and how to
// install one anyway.
func TestLocalCheck(t *testing.T) {
	groupsHome(t)
	out, err := stdoutOf(t, func() error { return localCmd([]string{"local", "check", "--json"}) })
	var a local.Assessment
	if err != nil || json.Unmarshal([]byte(out), &a) != nil || a.Quick || len(a.Models) != len(local.Candidates) || a.Bench.Bandwidth <= 0 || a.Why == "" {
		t.Fatalf("check --json: %v %s", err, out)
	}
	out, err = stdoutOf(t, func() error { return localCmd([]string{"local", "check"}) })
	if err != nil || !strings.Contains(out, local.Spark17.ID()) || !strings.Contains(out, local.Spark4.ID()) || !strings.Contains(out, "judges a turn") {
		t.Fatalf("check: %v %s", err, out)
	}
	if _, err := stdoutOf(t, func() error { return localCmd([]string{"local", "on"}) }); err != nil {
		t.Fatal(err)
	}
	if c, _ := local.Load(); !c.On || c.Model.ID() != local.Recommend().ID() {
		t.Fatalf("on: %+v", c)
	}

	if err := local.Save(local.Config{}); err != nil {
		t.Fatal(err)
	}
	old := localAssess
	t.Cleanup(func() { localAssess = old })
	localAssess = func() local.Assessment {
		return local.Assessment{Why: "slow", Models: []local.Estimate{
			{Model: local.Spark17.ID(), Title: local.Spark17.Title, Judge: 9.5, Gen: 6, Grade: local.Slow},
			{Model: local.Spark4.ID(), Title: local.Spark4.Title, Judge: 21, Gen: 2, Grade: local.Slow},
		}}
	}
	_, err = stdoutOf(t, func() error { return localCmd([]string{"local", "setup"}) })
	if err == nil || !strings.Contains(err.Error(), "isn't recommended") || !strings.Contains(err.Error(), "9.5 s") || !strings.Contains(err.Error(), "fogway local setup "+local.Spark17.ID()) {
		t.Fatalf("setup where none is recommended: %v", err)
	}
	if c, _ := local.Load(); c.On || c.Model.Repo != "" {
		t.Fatalf("set up all the same: %+v", c)
	}
	out, err = stdoutOf(t, func() error { return localCmd([]string{"local", "check"}) })
	if err != nil || !strings.Contains(out, "isn't recommended here") || !strings.Contains(out, "To try anyway") {
		t.Fatalf("check where none is recommended: %v %s", err, out)
	}
}
