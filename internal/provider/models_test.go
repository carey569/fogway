package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/settings"
)

func TestEntryForUsesImageSettingsSnapshot(t *testing.T) {
	prefsHome(t)
	if err := settings.Save(settings.Settings{ModelImages: map[string]bool{"a/sol": true}}); err != nil {
		t.Fatal(err)
	}
	snapshot := settings.Settings{ModelImages: map[string]bool{"a/sol": false, "b/sol": true, "c/*": true}}
	yes, no := true, false
	for _, tc := range []struct {
		provider string
		known    *bool
		images   bool
		want     *bool
	}{
		{"a", &yes, false, &no},
		{"b", nil, true, &yes},
		{"c", nil, false, nil},
		{"d", &yes, true, &yes},
		{"e", &no, false, &no},
	} {
		e := entryFor(Provider{ID: tc.provider}, catalog.Model{ID: "sol", ImageInput: tc.known}, snapshot)
		if e.Images != tc.images || (e.ImageInput == nil) != (tc.want == nil) ||
			(e.ImageInput != nil && *e.ImageInput != *tc.want) {
			t.Fatalf("%s image snapshot = %v, %v; want %v, %v", tc.provider, e.Images, e.ImageInput, tc.images, tc.want)
		}
	}
}

func TestExplicitTextOnlyBeatsCrossProviderImageGuess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{
		"a":{"models":{"shared":{"id":"shared","modalities":{"input":["text","image"],"output":["text"]}}}},
		"b":{"models":{"shared":{"id":"shared","modalities":{"input":["text","image"],"output":["text"]}}}}
	}`
	if err := os.WriteFile(catalog.CachePath(), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if !catalog.SeesImages("shared") {
		t.Fatal("test catalog did not classify shared model as vision")
	}
	if err := Save(Provider{ID: "probe", Chat: "https://example.test/v1", Key: "key", Models: []string{"shared"}}); err != nil {
		t.Fatal(err)
	}
	no := false
	if err := catalog.SaveLive("probe", "https://example.test/v1", []catalog.Model{{ID: "shared", ImageInput: &no}}); err != nil {
		t.Fatal(err)
	}
	for _, e := range Catalog() {
		if e.ID == "probe/shared" {
			if e.Images || e.ImageInput == nil || *e.ImageInput {
				t.Fatalf("explicit text-only model advertised images: %+v", e)
			}
			return
		}
	}
	t.Fatal("probe/shared missing from catalog")
}

func TestFetchedKeysShareImageCapability(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := `["text","image"]`
		if r.Header.Get("Authorization") == "Bearer text-key" {
			input = `["text"]`
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"shared","modalities":{"input":` + input + `}}]}`))
	}))
	defer server.Close()
	p := Provider{ID: "relay", Chat: server.URL, Key: "vision-key", Keys: []KeyAccount{{Key: "text-key"}}}
	models, err := p.Fetch(context.Background())
	if err != nil || len(models) != 1 || models[0].ImageInput == nil || *models[0].ImageInput || models[0].Images {
		t.Fatalf("shared image capability: %+v, %v", models, err)
	}
}

func TestFetchedKeysKeepUnknownImageCapability(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input := `,"modalities":{"input":["text","image"]}`
		if r.Header.Get("Authorization") == "Bearer unknown-key" {
			input = ""
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"shared"` + input + `}]}`))
	}))
	defer server.Close()
	p := Provider{ID: "relay", Chat: server.URL, Key: "vision-key", Keys: []KeyAccount{{Key: "unknown-key"}}}
	models, err := p.Fetch(context.Background())
	if err != nil || len(models) != 1 || models[0].ImageInput != nil {
		t.Fatalf("confirmed and unknown image capability: %+v, %v", models, err)
	}
}

func TestFetchedKeysKeepUnknownImageCapabilityFromOldCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer failed-key" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"shared","modalities":{"input":["text","image"]}}]}`))
	}))
	defer server.Close()
	if err := catalog.SaveLive("relay", server.URL, []catalog.Model{{ID: "shared", Keys: []string{keyID("failed-key")}}}); err != nil {
		t.Fatal(err)
	}
	p := Provider{ID: "relay", Chat: server.URL, Key: "vision-key", Keys: []KeyAccount{{Key: "failed-key"}}}
	models, err := p.Fetch(context.Background())
	if err != nil || len(models) != 1 || models[0].ImageInput != nil {
		t.Fatalf("fresh capability and old cache without ImageInput: %+v, %v", models, err)
	}
}

func modelNamed(ms []catalog.Model, id string) (catalog.Model, bool) {
	for _, m := range ms {
		if m.ID == id {
			return m, true
		}
	}
	return catalog.Model{}, false
}

func TestRejectsTemperatureFromFetchedList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", home)

	no, yes := false, true
	if err := catalog.SaveLive("p", "http://x", []catalog.Model{
		{ID: "strict", Name: "Strict", Temperature: &no},
		{ID: "plain", Name: "Plain", Temperature: &yes},
		{ID: "silent", Name: "Silent"},
	}); err != nil {
		t.Fatal(err)
	}
	p := Provider{ID: "p"}
	for model, want := range map[string]bool{"strict": true, "plain": false, "silent": false, "unknown": false} {
		if got := p.RejectsTemperature(model); got != want {
			t.Errorf("RejectsTemperature(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestIsOpenCode(t *testing.T) {
	for base, want := range map[string]bool{
		"https://opencode.ai/zen/go/v1": true,
		"https://api.opencode.ai/v1":    true,
		"https://notopencode.ai/v1":     false,
		"https://api.deepseek.com/v1":   false,
	} {
		if got := (Provider{Chat: base}).IsOpenCode(); got != want {
			t.Errorf("%s: %v", base, got)
		}
	}
}

// A model its vendor lists with no name is called by its id, in the
// catalog and so in every agent's list (#955: an unnamed one showed as
// fogway/antigravity/gemini-3.6-flash-tiered); a name the user gave it
// still wins, its vendor's then being the id.
func TestEntryForUnnamedModel(t *testing.T) {
	p := Provider{ID: "acme", Name: "Acme"}
	e := entryFor(p, catalog.Model{ID: "m-1"}, settings.Settings{})
	if e.Name != "m-1" || e.Label() != "m-1 · Acme" {
		t.Fatalf("name %q, label %q", e.Name, e.Label())
	}
	e = entryFor(p, catalog.Model{ID: "m-1", Name: "Model One"}, settings.Settings{})
	if e.Name != "Model One" {
		t.Fatalf("name %q", e.Name)
	}
	e = entryFor(p, catalog.Model{ID: "m-1"}, settings.Settings{ModelNames: map[string]string{"acme/m-1": "Mine"}})
	if e.Name != "Mine" || e.Default != "m-1" {
		t.Fatalf("name %q, default %q", e.Name, e.Default)
	}
}
