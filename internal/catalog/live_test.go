package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpointAPIs(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"/messages"}, "anthropic"},
		{[]string{"/v1/messages", "/chat/completions", "ws:/responses", "/responses"}, "anthropic,chat,responses"},
		{[]string{"/v1/chat/completions/", "/chat/completions"}, "chat"},
		{[]string{"/embeddings"}, ""},
		{nil, ""},
	} {
		if got := strings.Join(EndpointAPIs(c.in), ","); got != c.want {
			t.Errorf("%v: %q", c.in, got)
		}
	}
}

// Another fogway's list tells each model's price (fogway_price), tiers
// and all; one no vendor could charge is not taken, and a list without
// one leaves the model unpriced.
func TestLiveFogwayPrice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
			{"id":"a/priced","fogway_price":{"input":3,"output":12,"cache_read":0.3,"cache_write":3.75,"tiers":[{"above":200000,"input":6,"output":18,"cache_read":0.6,"cache_write":7.5}]}},
			{"id":"a/free","fogway_price":{"input":0,"output":0,"cache_read":0,"cache_write":0}},
			{"id":"a/negative","fogway_price":{"input":-1,"output":12,"cache_read":0,"cache_write":0}},
			{"id":"a/sizeless","fogway_price":{"input":1,"output":2,"cache_read":0,"cache_write":0,"tiers":[{"above":0,"input":2,"output":4}]}},
			{"id":"a/odd","fogway_price":"cheap"},
			{"id":"a/none"}]}`)
	}))
	defer srv.Close()
	ms, err := FetchURL(context.Background(), srv.URL+"/v1/models", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*Price{}
	for _, m := range ms {
		got[m.ID] = m.Price
	}
	if p := got["a/priced"]; p == nil || p.Input != 3 || p.CacheWrite != 3.75 || len(p.Tiers) != 1 || p.Tiers[0].Above != 200000 || p.Tiers[0].Output != 18 {
		t.Errorf("priced: %+v", p)
	}
	if p := got["a/free"]; p == nil || p.Input != 0 || p.Output != 0 {
		t.Errorf("a price of nothing, set, is a price: %+v", p)
	}
	for _, id := range []string{"a/negative", "a/none"} {
		if p := got[id]; p != nil {
			t.Errorf("%s: %+v", id, p)
		}
	}
	if len(ms) != 6 {
		t.Errorf("an odd price lost a model: %d of 6", len(ms))
	}
}
