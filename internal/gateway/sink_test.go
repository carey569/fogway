package gateway

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
)

func forgetSinks(t *testing.T) {
	t.Helper()
	clear := func() {
		sunk.Lock()
		sunk.m = map[string]sinking{}
		sunk.Unlock()
		restingUntil.Lock()
		restingUntil.m = map[string]time.Time{}
		restingUntil.note = map[string]Rest{}
		restingUntil.Unlock()
		routed.Lock()
		routed.failures = map[string]int{}
		routed.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// rested ends what rests by key's rest, as if its minute were up.
func rested(key string) {
	restingUntil.Lock()
	restingUntil.m[key] = time.Now().Add(-time.Second)
	restingUntil.Unlock()
}

func whoOf(cs []candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.rest)
	}
	return out
}

// A routing group with Sink: a member's key rate limited with quota left
// goes behind the other members, in order or not, and the trace's order
// follows it.
func TestGroupRateLimitedSinks(t *testing.T) {
	forgetSinks(t)
	s := &Server{}
	mem := func(id string) provider.Member {
		return provider.Member{ID: id + "/m", Path: []string{id + "/m"}, Provider: provider.Provider{ID: id, Chat: "http://127.0.0.1:1/v1", Key: "k-" + id}, Model: "m"}
	}
	ms := []provider.Member{mem("a"), mem("b"), mem("c")}
	ids := func(cs []candidate) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.p.ID)
		}
		return out
	}
	rate := []byte(`{"error":{"message":"Too many requests"}}`)
	cs, _ := s.planGroup(provider.Group{ID: "g", Routing: provider.Ordered}, ms, provider.Chat)
	s.restAfter(cs[0], 429, http.Header{}, rate)
	rested(cs[0].restKey())
	for _, routing := range []string{provider.Ordered, ""} {
		g := provider.Group{ID: "g", Routing: routing, Sink: true}
		out, pl := s.planGroup(g, ms, provider.Chat)
		if got := ids(out); !slices.Equal(got, []string{"b", "c", "a"}) {
			t.Fatalf("%q: order %v", routing, got)
		}
		if pl.order[2].Provider != "a" || pl.order[2].Sunk == nil || pl.order[0].Sunk != nil {
			t.Fatalf("%q: trace %+v", routing, pl.order)
		}
		g.Sink = false
		if out, _ := s.planGroup(g, ms, provider.Chat); ids(out)[0] != "a" {
			t.Fatalf("%q off: order %v", routing, ids(out))
		}
	}
}
