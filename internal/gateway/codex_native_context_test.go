package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/codexcat"
)

// A ChatGPT account's model in a catalog magpie writes (model_catalog_json)
// takes its entry from Codex's cache under magpie's id, with the window
// magpie resolved for it, not the one the cache kept (#674).
func TestCodexCatalogOwnEntryTakesResolvedWindow(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), []byte(`{"etag":"\"v1\"","models":[
		{"slug":"gpt-6.1-sol","context_window":600000,"max_context_window":872000,"base_instructions":"sol prompt"}]}`), 0o644)
	window := func(n int) (int, int, any) {
		es := codexcat.Entries([]catalog.Model{{ID: "codex/gpt-6.1-sol", Name: "Sol", Context: n}}, 0)
		// as Codex reads it
		b, _ := json.Marshal(es[0])
		var e struct {
			Context int    `json:"context_window"`
			Max     int    `json:"max_context_window"`
			Prompt  string `json:"base_instructions"`
		}
		json.Unmarshal(b, &e)
		return e.Context, e.Max, e.Prompt
	}
	if c, x, p := window(272000); c != 272000 || x != 872000 || p != "sol prompt" {
		t.Errorf("%d/%d %v, want 272000/872000 with the entry's prompt", c, x, p)
	}
	if c, x, _ := window(900000); c != 900000 || x != 900000 {
		t.Errorf("%d/%d, want 900000 with the max raised to it", c, x)
	}
}
