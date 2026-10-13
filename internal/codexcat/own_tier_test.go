package codexcat

import (
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

// What this magpie tells another that has it as its provider: the tiers
// its Codex is offered on each model — a ChatGPT account's own model with
// those of Codex's own entry, Ultrafast among them, a relay's GPT model
// Fast, none on any other — and that one's Codex is offered the same on
// the model, with Codex's own names (#1234).
func TestServiceTiersGoOnToAnotherMagpie(t *testing.T) {
	v1Home(t, `{"etag":"W/\"a\"","models":[{"slug":"gpt-6.1-sol","base_instructions":"x","service_tiers":[{"id":"priority","name":"Fast","description":"f"},{"id":"ultrafast","name":"Ultrafast","description":"u"}]}]}`)
	here := ServiceTiers([]catalog.Model{
		{ID: "codex/gpt-6.1-sol", Name: "a"},
		{ID: "relay/gpt-5.5", Name: "b", OwnTier: true},
		{ID: "relay/glm-5", Name: "c", OwnTier: true},
	})
	ids := func(ts []tier) string {
		s := ""
		for _, x := range ts {
			s += x.ID + ":" + x.Name + " "
		}
		return s
	}
	want := map[string]string{"codex/gpt-6.1-sol": "priority:Fast ultrafast:Ultrafast ", "relay/gpt-5.5": "priority:Fast ", "relay/glm-5": ""}
	for k, v := range want {
		if got := ids(here[k]); got != v {
			t.Errorf("%s tells %q, want %q", k, got, v)
		}
	}
	var there []catalog.Model
	for _, id := range []string{"codex/gpt-6.1-sol", "relay/glm-5"} {
		var list []string
		for _, x := range here[id] {
			list = append(list, x.ID)
		}
		there = append(there, catalog.Model{ID: "office/" + id, Name: id, Tiers: list})
	}
	got := ServiceTiers(there)
	if s := ids(got["office/codex/gpt-6.1-sol"]); s != "priority:Fast ultrafast:Ultrafast " {
		t.Errorf("the other magpie's Codex offered %q", s)
	}
	if s := ids(got["office/relay/glm-5"]); s != "" {
		t.Errorf("a model offered no tier there is offered %q", s)
	}
}
