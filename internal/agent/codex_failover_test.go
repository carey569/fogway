package agent

import (
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/gateway"
)

// A base URL the user wrote in config.toml by hand, to have Codex's own
// models go through fogway, stays when fogway starts (Sync) with no other
// ChatGPT account on to fail over to: failover takes away only the one it
// wrote (#856: it was gone again after every fogway update).
func TestCodexHandWrittenBaseURLStays(t *testing.T) {
	base := gateway.URL() + gateway.CodexPath
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`,
		"model = \"gpt-5.5\"\nopenai_base_url = \""+base+"\"\n")
	cx := codex(home)
	for i := 0; i < 2; i++ {
		if err := cx.Sync(); err != nil {
			t.Fatal(err)
		}
		if cfg := read(); !strings.Contains(cfg, `openai_base_url = "`+base+`"`) || !strings.Contains(cfg, `model = "gpt-5.5"`) {
			t.Fatalf("sync %d took the user's base URL away:\n%s", i, cfg)
		}
	}
}
