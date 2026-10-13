package provider

import (
	"testing"
)

func TestPresetKeepsHeaders(t *testing.T) {
	p, _ := FromPreset("openrouter")
	p.Headers = map[string]string{" HTTP-Referer ": " https://example.com ", "": "x"}
	p = normalize(p)
	if p.Preset != "openrouter" || len(p.Headers) != 1 || p.Headers["HTTP-Referer"] != "https://example.com" {
		t.Fatalf("preset headers: %+v", p.Headers)
	}
}

func TestPresetHeaderHints(t *testing.T) {
	for id, want := range map[string]string{"anthropic": "anthropic-workspace-id", "openrouter": "X-OpenRouter-Title"} {
		pr := Preset(id)
		if pr == nil {
			t.Fatalf("no preset %q", id)
		}
		found := false
		for _, h := range pr.HeaderHints {
			found = found || h == want
		}
		if !found {
			t.Fatalf("%s hints %v, want %s among them", id, pr.HeaderHints, want)
		}
	}
}
