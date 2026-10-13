package gateway

import (
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/provider"
	"github.com/fogway/fogway/internal/settings"
)

// No API means no injected search function; a relay's native search
// declaration still reaches it unchanged.
func TestSearchProviderOffForwarding(t *testing.T) {
	fresh(t)
	relay, seen := relayOfEvery(t)
	st := settings.Load()
	st.Searcher = "off"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	for _, native := range []bool{false, true} {
		p := provider.Provider{ID: "relay", Name: "Relay", Key: "k", Models: []string{"custom-model"}, Searches: native}
		if native {
			p.Anthropic = relay.URL
		} else {
			p.Chat = relay.URL + "/v1"
		}
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
		body := `{"model":"relay/custom-model","max_tokens":100,"messages":[{"role":"user","content":"news?"}],"tools":[{"name":"Read","input_schema":{"type":"object"}},{"type":"web_search_20250305","name":"web_search"}]}`
		if code, reply := post(t, "/v1/messages", body); code != 200 {
			t.Fatalf("native %v: %d %s", native, code, reply)
		}
		paths, bodies := seen()
		if len(bodies) != 1 {
			t.Fatalf("native %v: upstream calls %v", native, paths)
		}
		if strings.Contains(bodies[0], "web_search") != native || !strings.Contains(bodies[0], "Read") {
			t.Errorf("native %v: tools changed incorrectly: %s", native, bodies[0])
		}
	}
}
