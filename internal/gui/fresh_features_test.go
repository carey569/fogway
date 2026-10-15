package gui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A new install was written plugins.yml's Essentials preset, which left
// every built-in plugin not marked Essential off: Sessions and routing
// groups, two of what Fogway keeps. With the Plugins page gone there was
// nowhere to turn them on, and the window left Sessions out of its sidebar
// for anyone not recording gateway conversations (the owner's own, on
// 2026-10-15). The window leaves nothing out: boot.js names no feature off,
// and neither do the settings.
func TestNewInstallLeavesNothingOut(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	// what a new install was written, and the owner's file says
	dir := filepath.Join(h, ".config", "fogway")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins.yml"), []byte("preset: essentials\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := Handler(webHost{}, nil)
	get := func(path string) string {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	js := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(get("/boot.js")), "window.bootPrefs = "), ";")
	var boot map[string]any
	if err := json.Unmarshal([]byte(js), &boot); err != nil {
		t.Fatalf("boot.js: %v: %s", err, js)
	}
	if off, ok := boot["featuresOff"].([]any); ok && len(off) > 0 {
		t.Errorf("boot.js leaves %v out of the window", off)
	}
	var s struct {
		Builtins []struct {
			ID string `json:"id"`
			On bool   `json:"on"`
		} `json:"builtins"`
	}
	if err := json.Unmarshal([]byte(get("/api/settings")), &s); err != nil {
		t.Fatal(err)
	}
	for _, b := range s.Builtins {
		if !b.On {
			t.Errorf("the settings say %s is off", b.ID)
		}
	}
}
