package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

func TestOpenCodeVariants(t *testing.T) {
	for _, c := range []struct {
		efforts []string
		want    string
	}{
		{[]string{"none", "high", "max"}, `{"none":{"reasoningEffort":"none"},"high":{"reasoningEffort":"high"},"max":{"reasoningEffort":"max"}}`},
		{[]string{"low", "medium", "high", "xhigh"}, `{"low":{"reasoningEffort":"low"},"medium":{"reasoningEffort":"medium"},"high":{"reasoningEffort":"high"},"xhigh":{"reasoningEffort":"xhigh"}}`},
		// weakest first whatever order the model lists them in (#713)
		{[]string{"max", "ultra", "low", "xhigh", "medium", "high"}, `{"low":{"reasoningEffort":"low"},"medium":{"reasoningEffort":"medium"},"high":{"reasoningEffort":"high"},"xhigh":{"reasoningEffort":"xhigh"},"max":{"reasoningEffort":"max"},"ultra":{"reasoningEffort":"ultra"}}`},
		// none at all: an empty set, so OpenCode 2 doesn't make low, medium
		// and high of its own
		{nil, `{}`},
	} {
		b, err := json.Marshal(openCodeVariants(c.efforts))
		if err != nil || string(b) != c.want {
			t.Errorf("%v: got %s (%v), want %s", c.efforts, b, err, c.want)
		}
	}
}

// variantOrder is the variants of the one model in an opencode.json, in the
// order the file's bytes list them.
func variantOrder(t *testing.T, b []byte) []string {
	t.Helper()
	i := bytes.Index(b, []byte(`"variants"`))
	if i < 0 {
		t.Fatalf("no variants: %s", b)
	}
	rest := b[i:]
	at := map[string]int{}
	var got []string
	for _, e := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		if j := bytes.Index(rest, []byte(`"`+e+`":`)); j >= 0 {
			at[e] = j
			got = append(got, e)
		}
	}
	slices.SortFunc(got, func(x, y string) int { return at[x] - at[y] })
	return got
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestOpenCodeWritesVariants: each model of magpie's in opencode.json names
// the reasoning levels it has as variants — OpenCode 2 otherwise offers low,
// medium and high for every one — and the rest of the entry is as before.
func TestOpenCodeWritesVariants(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{
		ID: "think", Name: "Think", Chat: "https://example.test/v1", Key: "key",
		Models: []string{"deep", "wide", "plain"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("think", "https://example.test/v1", []catalog.Model{
		{ID: "deep", Efforts: []string{"none", "high", "max"}, Context: 1000000, Output: 64000},
		{ID: "wide", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{ID: "plain"},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeFile(t, path, `{"theme":"dark"}`)
	if err := opencode(home, filepath.Join(home, ".config")).Field("model").Set("magpie/think/deep"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Theme    string `json:"theme"`
		Model    string `json:"model"`
		Provider map[string]struct {
			NPM    string                    `json:"npm"`
			Models map[string]map[string]any `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "dark" || cfg.Model != "magpie/think/deep" {
		t.Fatalf("config: %s", b)
	}
	p := cfg.Provider["magpie"]
	if p.NPM != "@ai-sdk/openai-compatible" {
		t.Fatalf("npm %q", p.NPM)
	}
	levels := func(id string) []string {
		t.Helper()
		m, ok := p.Models[id]
		if !ok {
			t.Fatalf("%s missing: %s", id, b)
		}
		vs, ok := m["variants"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no variants: %v", id, m)
		}
		var out []string
		for _, e := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
			if v, ok := vs[e]; ok {
				if !reflect.DeepEqual(v, map[string]any{"reasoningEffort": e}) {
					t.Fatalf("%s %s: %v", id, e, v)
				}
				out = append(out, e)
			}
		}
		if len(out) != len(vs) {
			t.Fatalf("%s: unexpected variants %v", id, vs)
		}
		return out
	}
	if got := levels("think/deep"); !reflect.DeepEqual(got, []string{"none", "high", "max"}) {
		t.Fatalf("deep: %v", got)
	}
	if got := levels("think/wide"); !reflect.DeepEqual(got, []string{"low", "medium", "high", "xhigh", "max"}) {
		t.Fatalf("wide: %v", got)
	}
	if got := levels("think/plain"); got != nil {
		t.Fatalf("plain: %v", got)
	}
	if lim, _ := p.Models["think/deep"]["limit"].(map[string]any); lim["context"] != float64(1000000) {
		t.Fatalf("deep limit: %v", p.Models["think/deep"])
	}
}

// TestOpenCodeZenModelReasons: an OpenCode Zen model that thinks with no
// levels to pick (mimo-v2.6-flash-free, models.dev: reasoning true, no
// reasoning_options) is marked reasoning in opencode.json, which OpenCode's
// model tooltip showed as 不支持推理 through magpie (#725); so is one whose
// levels hold OpenCode's own low, medium and high. One with fewer levels
// isn't (OpenCode would add the levels it lacks), nor one that doesn't think.
func TestOpenCodeZenModelReasons(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{
	  "opencode": {"models": {
	    "mimo-v2.6-flash-free": {"id":"mimo-v2.6-flash-free","reasoning":true,"reasoning_options":[]},
	    "deepseek-v4-flash-free": {"id":"deepseek-v4-flash-free","reasoning":true,"reasoning_options":[{"type":"effort","values":["low","high","max"]}]},
	    "laguna-s-2.1-free": {"id":"laguna-s-2.1-free","reasoning":true,"reasoning_options":[{"type":"effort","values":["low","medium","high"]}]},
	    "ling-2.6-flash-free": {"id":"ling-2.6-flash-free","reasoning":false}}}
	}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	zen, err := provider.FromPreset("opencode-zen")
	if err != nil {
		t.Fatal(err)
	}
	zen.Models = []string{"mimo-v2.6-flash-free", "deepseek-v4-flash-free", "laguna-s-2.1-free", "ling-2.6-flash-free"}
	if err := provider.Save(zen); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeFile(t, path, `{}`)
	if err := opencode(home, filepath.Join(home, ".config")).Field("model").Set("magpie/opencode-zen/mimo-v2.6-flash-free"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Provider map[string]struct {
			Models map[string]struct {
				Reasoning *bool          `json:"reasoning"`
				Variants  map[string]any `json:"variants"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	ms := cfg.Provider["magpie"].Models
	for id, want := range map[string]bool{"mimo-v2.6-flash-free": true, "laguna-s-2.1-free": true,
		"deepseek-v4-flash-free": false, "ling-2.6-flash-free": false} {
		m, ok := ms["opencode-zen/"+id]
		if !ok {
			t.Errorf("%s missing: %s", id, b)
			continue
		}
		if got := m.Reasoning != nil && *m.Reasoning; got != want {
			t.Errorf("%s: reasoning %v, want %v (variants %v)", id, got, want, m.Variants)
		}
	}
}
