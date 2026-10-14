package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/carey569/fogway/internal/catalog"
)

// iFlytek Spark's plans serve no model list fogway can ask (the Token
// Plan's /v2/models is 404; Spark X2's host answers 401 on any path, so its
// list can't be told from none), so each carries the models its doc gives
// and is never asked; a key at one plan gets that plan's models. A new
// provider is at the Token Plan, and the models the plan's doc calls
// discontinued aren't offered.
func TestIFlytekPlans(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write([]byte(`{"data":[{"id":"not-a-plan-model"}]}`))
	}))
	t.Cleanup(srv.Close)
	pr := Preset("iflytek")
	if pr == nil || pr.Kind != KindVendor || len(pr.Regions) != 3 {
		t.Fatalf("preset %v", pr)
	}
	at := func(u string) string {
		if u == "" {
			return ""
		}
		return srv.URL + basePath(u)
	}
	for _, r := range pr.Regions {
		t.Run(r.ID, func(t *testing.T) {
			p, err := FromPreset("iflytek")
			if err != nil {
				t.Fatal(err)
			}
			p.ID, p.Key = "iflytek-"+r.ID, "test-key"
			p.Chat, p.Responses, p.Anthropic = at(r.Chat), at(r.Responses), at(r.Anthropic)
			if err := catalog.SaveLive(p.ID, p.Chat, []catalog.Model{{ID: "not-a-plan-model"}}); err != nil {
				t.Fatal(err)
			}
			ids := func(ms []catalog.Model) []string {
				var out []string
				for _, m := range ms {
					out = append(out, m.ID)
				}
				return out
			}
			if got := ids(p.Available()); !slices.Equal(got, r.Models) || len(got) == 0 {
				t.Errorf("available: %v, want %v", got, r.Models)
			}
			ms, err := p.Fetch(context.Background())
			if got := ids(ms); err != nil || !slices.Equal(got, r.Models) {
				t.Errorf("fetch: %v %v, want %v", got, err, r.Models)
			}
			if n := asked.Load(); n != 0 {
				t.Errorf("asked for models %d times", n)
			}
		})
	}
	p, _ := FromPreset("iflytek")
	if r := p.regionOf(pr); r == nil || r.ID != "token" || p.KeysURL != r.KeysURL {
		t.Errorf("a new provider is at %+v, keys from %s", r, p.KeysURL)
	}
	for _, gone := range []string{"xsparkx2", "xsparkx2agent"} {
		if slices.Contains(pr.Regions[0].Models, gone) {
			t.Errorf("offers %s, discontinued", gone)
		}
	}
}
