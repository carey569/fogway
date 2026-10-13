package codexcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fogway/fogway/internal/catalog"
	"github.com/fogway/fogway/internal/settings"
)

func v1Home(t *testing.T, cache string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), []byte(cache), 0o644)
	return home
}

func setV1(t *testing.T, on bool) {
	t.Helper()
	s := settings.Load()
	s.CodexAgentsV1 = on
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
}

func versions(t *testing.T, ms []catalog.Model) map[string]any {
	t.Helper()
	var got struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(Catalog(ms), &got); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, m := range got.Models {
		out[m["slug"].(string)] = m["multi_agent_version"]
	}
	return out
}

// Codex keeps the stamped list in models_cache.json. Read back, it says what
// the backend did, so turning the setting off undoes it: a version the
// backend gave comes back, one it didn't give goes.
func TestCodexAgentsV1CacheRestores(t *testing.T) {
	home := v1Home(t, `{"etag":"W/\"a+fogway-0011\"","models":[
		{"slug":"gpt-6","multi_agent_version":"v2"},
		{"slug":"gpt-5.5"},
		{"slug":"gpt-luna","multi_agent_version":"v1"}]}`)
	// read unstamped: what the backend said is kept
	if got := CacheEntries(); got["gpt-6"]["multi_agent_version"] != "v2" {
		t.Fatalf("%v", got)
	}
	// Codex has since cached a stamped list
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), []byte(`{"etag":"W/\"b+fogway-v1.0011\"","models":[
		{"slug":"gpt-6","multi_agent_version":"v1"},
		{"slug":"gpt-5.5","multi_agent_version":"v1"},
		{"slug":"gpt-luna","multi_agent_version":"v1"},
		{"slug":"gpt-new","multi_agent_version":"v1"}]}`), 0o644)
	got := CacheEntries()
	if got["gpt-6"]["multi_agent_version"] != "v2" || got["gpt-luna"]["multi_agent_version"] != "v1" {
		t.Fatalf("not restored: %v", got)
	}
	if _, ok := got["gpt-5.5"]["multi_agent_version"]; ok {
		t.Fatalf("a version the backend never gave stayed: %v", got["gpt-5.5"])
	}
	if got["gpt-new"] == nil {
		t.Fatal("a model with no record is dropped")
	}
	// and the stamped values aren't taken for the backend's
	if was := originals(); was["gpt-5.5"] != "" || was["gpt-6"] != "v2" {
		t.Fatalf("record: %v", was)
	}
}
