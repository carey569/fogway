package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// The one key, all but used up, keeps the conversation it answered: there
// is nobody it could move to, and the routing story says it was kept,
// not that it moved.
func TestLoneSpentKeyKeepsItsConversation(t *testing.T) {
	now := time.Now()
	keyWindows(t, map[string]provider.Allowance{"sk-kimi-a": {{Used: 99, Resets: now.Add(2 * time.Hour), Span: 5 * time.Hour}}})
	p := provider.Provider{ID: "kimi-code-cn", Key: "sk-kimi-a", Chat: "https://api.kimi.com/coding/v1"}
	c := candidate{p: p, model: "kimi-for-coding", rest: "kimi-code-cn"}
	cs, wg := weigh(p, []candidate{c}, "kimi-for-coding", provider.Chat)
	pl := planned{order: []Weighed{weighed(cs[0], p, wg, false, provider.Chat)}}
	if !pl.order[0].Known || pl.order[0].Used != 99 {
		t.Fatalf("the trace: %+v", pl.order[0])
	}
	h := http.Header{}
	h.Set("x-claude-code-session-id", "lone-spent-1016")
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	_, _, _, key := affine("t", provider.AffinitySession, false, h, provider.Chat, body, cs, pl)
	answered(key, c, 1, 0)
	t.Cleanup(func() { sticks.Lock(); delete(sticks.m, key); sticks.Unlock() })
	got, _, aff, _ := affine("t", provider.AffinitySession, false, h, provider.Chat, body, cs, pl)
	if got[0].rest != "kimi-code-cn" || aff.Why == "spent" || !aff.Kept {
		t.Fatalf("kept %v, why %s", aff.Kept, aff.Why)
	}
}
