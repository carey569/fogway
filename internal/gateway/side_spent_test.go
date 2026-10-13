package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/catalog"
	"github.com/fogway/fogway/internal/provider"
)

// restAs rests ids (a provider, a key, or a provider's model) as routing
// does after the vendor said why, for an hour.
func restAs(t *testing.T, why string, ids ...string) {
	t.Helper()
	restingUntil.Lock()
	defer restingUntil.Unlock()
	for _, id := range ids {
		restingUntil.m[id] = time.Now().Add(time.Hour)
		restingUntil.note[id] = Rest{Why: why, Status: 429, By: "window"}
	}
	t.Cleanup(func() {
		restingUntil.Lock()
		defer restingUntil.Unlock()
		for _, id := range ids {
			delete(restingUntil.m, id)
			delete(restingUntil.note, id)
		}
	})
}

// Image recognition left to fogway picks a describer with allowance left
// over one out of it, whatever their order: the cheaper key's describer
// out of quota, resting after a refusal or read used up alike, gives way
// to the dearer one (#1801, hibaliqya-yonma: each image went on to an
// account at 100% and was refused 429). A rate limit isn't out of
// allowance. With every one out, one is still named, so agents are still
// told images are described.
func TestAutoVisionPassesOverOneOut(t *testing.T) {
	fresh(t)
	for id, price := range map[string]float64{"cheap": 1, "dear": 5} {
		if err := provider.Save(provider.Provider{ID: id, Name: id, Chat: "http://" + id + ".invalid/v1", Key: id + "-key", Models: []string{"eye"}}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.SaveLive(id, "http://"+id+".invalid/v1", []catalog.Model{
			{ID: "eye", Images: true, ImageInput: imageInputBool(true), Price: &catalog.Price{Input: price, Output: price}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	windows := map[string]provider.Allowance{}
	keyWindows(t, windows)
	if m := AutoVision(); m != "cheap/eye" {
		t.Fatalf("AutoVision = %q, want the cheaper", m)
	}
	restAs(t, failRate, "cheap")
	if m := AutoVision(); m != "cheap/eye" {
		t.Fatalf("AutoVision = %q with cheap only rate limited", m)
	}
	restAs(t, failQuota, "cheap")
	if m := AutoVision(); m != "dear/eye" {
		t.Fatalf("AutoVision = %q with cheap out of quota", m)
	}
	restAs(t, failCredit, "dear")
	if m := AutoVision(); m != "cheap/eye" || !provider.Described() {
		t.Fatalf("AutoVision = %q, described %v, with every one out", m, provider.Described())
	}
	if _, ready := autoVision(); ready {
		t.Fatal("a pick out of allowance said to be ready")
	}

	// used up as last read, before any refusal
	restingUntil.Lock()
	restingUntil.m = map[string]time.Time{}
	restingUntil.Unlock()
	windows["cheap-key"] = week(100, time.Now().Add(time.Hour))
	if m := AutoVision(); m != "dear/eye" {
		t.Fatalf("AutoVision = %q with cheap's week used up", m)
	}
	windows["cheap-key"] = week(100, time.Now().Add(-time.Hour))
	if m := AutoVision(); m != "cheap/eye" {
		t.Fatalf("AutoVision = %q after cheap's week renewed", m)
	}

	// out for one model only: the same provider's other one that sees
	p, err := provider.Find("cheap")
	if err != nil {
		t.Fatal(err)
	}
	p.Models = append(p.Models, "other-eye")
	if err := provider.Save(*p); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive(p.ID, p.Chat, []catalog.Model{
		{ID: "eye", Images: true, Price: &catalog.Price{Input: 1, Output: 1}},
		{ID: "other-eye", Images: true, Price: &catalog.Price{Input: 2, Output: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	restAs(t, failQuota, "cheap/eye")
	if m := AutoVision(); m != "cheap/other-eye" {
		t.Fatalf("AutoVision = %q with only cheap/eye out", m)
	}
}

// searchFake is an Anthropic API that searches, answering each key with
// what it found; the keys it was asked with are kept.
type searchFake struct {
	mu    sync.Mutex
	asked []string
}

func (f *searchFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	f.mu.Lock()
	f.asked = append(f.asked, r.Header.Get("X-Api-Key"))
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"Go 1.27.1 is out: https://go.dev/doc/devel/release"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
}

func (f *searchFake) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// The web search fogway runs for a model that can't is a side request of
// the same kind: the searcher it picks has allowance left, and one out of
// it isn't asked, the next searcher or the search APIs searching instead.
func TestSearcherPassesOverOneOut(t *testing.T) {
	fresh(t)
	f := &searchFake{}
	up := httptest.NewServer(f)
	t.Cleanup(up.Close)
	hosts := searchHosts[provider.Anthropic]
	searchHosts[provider.Anthropic] = append(slices.Clone(hosts), provider.HostOf(up.URL))
	t.Cleanup(func() { searchHosts[provider.Anthropic] = hosts })
	for _, id := range []string{"first", "second"} {
		if err := provider.Save(provider.Provider{ID: id, Name: id, Anthropic: up.URL, Key: id + "-key", Models: []string{"claude-haiku-5-5"}}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.SaveLive(id, up.URL, []catalog.Model{{ID: "claude-haiku-5-5"}}); err != nil {
			t.Fatal(err)
		}
	}
	windows := map[string]provider.Allowance{}
	keyWindows(t, windows)
	first, _, ok := autoSearcher()
	if !ok {
		t.Fatal("no searcher")
	}
	other := "second"
	if first.ID == "second" {
		other = "first"
	}
	windows[first.ID+"-key"] = week(100, time.Now().Add(time.Hour))
	if p, _, ok := autoSearcher(); !ok || p.ID != other {
		t.Fatalf("autoSearcher = %q with %s used up", p.ID, first.ID)
	}
	said, _, err := New().webSearch(context.Background(), "latest go")
	if err != nil || !strings.Contains(said, "Go 1.27.1") {
		t.Fatalf("search: %q %v", said, err)
	}
	if got := f.keys(); slices.Contains(got, first.ID+"-key") || !slices.Contains(got, other+"-key") {
		t.Fatalf("asked with %q", got)
	}

	// both out: fogway still offers its search, and asks neither
	windows[other+"-key"] = week(100, time.Now().Add(time.Hour))
	if !canSearch() {
		t.Fatal("search no longer offered with its searchers out for now")
	}
	before := len(f.keys())
	if _, _, err := New().webSearch(context.Background(), "latest go"); err == nil || !strings.Contains(err.Error(), "out of its allowance") {
		t.Fatalf("both out: %v", err)
	}
	if n := len(f.keys()); n != before {
		t.Fatalf("a searcher out of allowance was asked: %q", f.keys()[before:])
	}
}
