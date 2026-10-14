package agent

import (
	"testing"

	"github.com/carey569/fogway/internal/usage"
)

// Every agent's requests are told by what its own description says.
func TestAgentOf(t *testing.T) {
	cases := map[string]string{
		"claude-cli/2.1.0 (external, cli)":         "claude",
		"codex_cli_rs/0.40.0 (Mac OS 26.0; arm64)": "codex",
		"GeminiCLI/0.9.0 (darwin; arm64)":          "gemini",
		"opencode/1.2.3":                           "opencode",
		"curl/8.4.0":                               "curl",
		"oh-my-pi/1.0":                             "omp",
		"deepseek-harness/0.3.1":                   "dsh",
		"dsh":                                      "dsh", // a record kept by an id already
		"pi":                                       "pi",
		"cline ai-sdk/openai-compatible/3.0.37":    "cline",
		"":                                         "other",
	}
	for ua, want := range cases {
		if got := usage.AgentOf(ua); got != want {
			t.Errorf("%q: got %q want %q", ua, got, want)
		}
	}
}

// Every agent's own gateway key (gateway.TokenFor) names it in usage: one
// that sends no User-Agent of its own is told by that key alone.
func TestEveryAgentNamedByItsKey(t *testing.T) {
	for _, a := range Clients() {
		if got := usage.AgentOf(a.ID); got != a.ID {
			t.Errorf("%s's key names %q", a.ID, got)
		}
	}
}
