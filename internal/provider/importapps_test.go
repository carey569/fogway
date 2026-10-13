package provider

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sqliteFixture(t *testing.T, path string, stmts ...string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func itemsOf(t *testing.T, id string) map[string]AppImport {
	t.Helper()
	for _, s := range ImportSources() {
		if s.ID == id {
			if !s.Found || s.Error != "" {
				t.Fatalf("%s: found %v, %s", id, s.Found, s.Error)
			}
			out := map[string]AppImport{}
			for _, it := range s.Items {
				out[it.Ref] = it
			}
			return out
		}
	}
	t.Fatalf("no source %s", id)
	return nil
}

func TestImportHeadersDistinguishExistingProvider(t *testing.T) {
	existing := Provider{ID: "relay", Key: "sk-shared", Responses: "https://relay.example.com/v1"}
	incoming := existing
	incoming.Headers = map[string]string{"X-Org": "abc"}
	items := settle([]AppImport{{Provider: incoming}}, []Provider{existing}, map[string]bool{})
	if items[0].Status != "taken" || items[0].KeyOf != "" {
		t.Fatalf("provider with new headers should be importable separately: %+v", items[0])
	}
	existing.Headers = incoming.Headers
	incoming.Key = "sk-other"
	items = settle([]AppImport{{Provider: incoming}}, []Provider{existing}, map[string]bool{})
	if items[0].KeyOf != "relay" {
		t.Fatalf("provider with matching headers should accept another key: %+v", items[0])
	}
}

func TestCodexImportModelsSameBasename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)                                  // Windows
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming")) // Windows: never the real Alma
	config := filepath.Join(home, "other", "config.toml")
	models := []byte(`{"models":[{"slug":"custom-model","visibility":"list"}]}`)
	for _, path := range []string{
		filepath.Join(home, ".codex", "magpie-models.json"),
		filepath.Join(home, "other", "magpie-models.json"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, models, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := codexImportModels(config, "../.codex/magpie-models.json"); len(got) != 0 {
		t.Fatalf("magpie's own catalog was imported: %v", got)
	}
	if got := codexImportModels(config, "magpie-models.json"); len(got) != 1 || got[0] != "custom-model" {
		t.Fatalf("user catalog with the same basename was skipped: %v", got)
	}
}

func FromPresetKey(t *testing.T, id, key string) Provider {
	p, err := FromPreset(id)
	if err != nil {
		t.Fatalf("no preset %s", id)
	}
	p.Key = key
	return p
}

func TestImportAppsByName(t *testing.T) {
	for i := 1; i < len(appReaders); i++ {
		if strings.ToLower(appReaders[i-1].name) > strings.ToLower(appReaders[i].name) {
			t.Fatalf("%s listed before %s", appReaders[i-1].name, appReaders[i].name)
		}
	}
}

// An app with nothing to bring over lists no items, not null: the window
// reads each source's items.
func TestImportSourcesNeverNull(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)                                  // Windows
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming")) // Windows: never the real Alma
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"model": "opus"}`), 0o644)
	for _, s := range ImportSources() {
		b, _ := json.Marshal(s)
		if s.Items == nil || strings.Contains(string(b), `"items":null`) {
			t.Fatalf("%s: items is null: %s", s.ID, b)
		}
	}
}

func TestSqlitePath(t *testing.T) {
	for in, want := range map[string]string{
		`C:/Users/me/.cc-switch/cc-switch.db`: "/C:/Users/me/.cc-switch/cc-switch.db",
		"/Users/me/.cc-switch/cc-switch.db":   "/Users/me/.cc-switch/cc-switch.db",
	} {
		if got := sqlitePath(in); got != want {
			t.Errorf("sqlitePath(%q) = %q, want %q", in, got, want)
		}
	}
}
