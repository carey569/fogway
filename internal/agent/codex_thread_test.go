package agent

import (
	"strings"
	"testing"
)

// A base URL of the user's own — a proxy of theirs in front of OpenAI — is
// left as it is when magpie becomes the provider, and after.
func TestCodexProviderKeepsOwnBaseURL(t *testing.T) {
	const own = `openai_base_url = "https://proxy.example/v1"`
	home, read := codexHome(t, "", "model = \"gpt-5.5\"\n"+own+"\n")
	cx := codex(home)
	if err := cx.Fields[0].Set("fake/m1"); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); !strings.Contains(cfg, own) || strings.Count(cfg, "openai_base_url") != 1 ||
		!strings.Contains(cfg, `model_provider = "magpie"`) {
		t.Fatalf("\n%s", cfg)
	}
	if err := cx.Fields[0].Set(""); err != nil {
		t.Fatal(err)
	}
	if cfg := read(); !strings.Contains(cfg, own) || strings.Contains(cfg, "model_provider =") {
		t.Fatalf("reset:\n%s", cfg)
	}
}
