package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/testenv"
)

// syncHome is a sandbox home with a models.dev catalog that knows glm-4.6's
// window and a provider serving it.
func syncHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("HERMES_HOME", "")
	t.Setenv("MIMOCODE_HOME", "")
	t.Setenv("DEVECO_CONFIG_DIR", "")
	t.Setenv("HANA_HOME", "")
	t.Setenv("DSH_HOME", "")
	t.Setenv("OMO_CODING_AGENT_DIR", "")
	t.Setenv("SENPI_CODING_AGENT_DIR", "")
	t.Setenv("PRIME_AGENT_CODING_AGENT_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "")
	noKeychain(t)
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"zai":{"models":{"glm-4.6":{"id":"glm-4.6","name":"GLM-4.6","limit":{"context":204800}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"glm-4.6"}}); err != nil {
		t.Fatal(err)
	}
	return home
}

// noKeychain puts a `security` that finds nothing first on PATH: the
// catalog looks for a Claude Code sign-in, on a Mac in the Keychain, which
// a test has no business reading.
func noKeychain(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	bin := t.TempDir()
	testenv.Program(t, filepath.Join(bin, "security"), "#!/bin/sh\nexit 44\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// A provider added after a magpie model was picked reaches the lists agents
// keep of magpie's models; a file magpie wrote nothing into stays as it is.
func TestSyncCatalogRewritesAgentLists(t *testing.T) {
	home := syncHome(t)
	piModels := filepath.Join(home, ".pi", "agent", "models.json")
	writeFile(t, piModels, `{"providers":{"mine":{"baseUrl":"http://x"},"magpie":{"name":"magpie","models":[]}}}`)
	crushCfg := filepath.Join(home, ".config", "crush", "crush.json")
	crushBody := `{"providers":{"mine":{"base_url":"http://x"}}}`
	writeFile(t, crushCfg, crushBody)
	codexDir := filepath.Join(home, ".codex")
	codexCat := filepath.Join(codexDir, "magpie-models.json")
	writeFile(t, filepath.Join(codexDir, "config.toml"), "model = \"relay/glm-4.6\"\nmodel_provider = \"magpie\"\nmodel_catalog_json = '"+codexCat+"'\n")
	writeFile(t, codexCat, `{"models":[]}`)

	if err := provider.Save(provider.Provider{ID: "added", Name: "Added", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	SyncCatalog()

	pi := strings.ReplaceAll(readFile(piModels), " ", "")
	if !strings.Contains(pi, `"relay/glm-4.6"`) || !strings.Contains(pi, `"added/m2"`) ||
		!strings.Contains(pi, `"contextWindow":204800`) || !strings.Contains(pi, `"mine"`) {
		t.Errorf("pi models.json:\n%s", pi)
	}
	if got := readFile(crushCfg); got != crushBody {
		t.Errorf("crush config without magpie changed:\n%s", got)
	}
	cx := readFile(codexCat)
	if !strings.Contains(cx, `"slug": "added/m2"`) || !strings.Contains(cx, `"context_window": 204800`) {
		t.Errorf("codex catalog:\n%s", cx)
	}

	// nothing changed since: nothing is written
	st, _ := os.Stat(piModels)
	os.Chtimes(piModels, st.ModTime().Add(-1e12), st.ModTime().Add(-1e12))
	was, _ := os.Stat(piModels)
	SyncCatalog()
	if now, _ := os.Stat(piModels); !now.ModTime().Equal(was.ModTime()) {
		t.Error("pi models.json rewritten though nothing changed")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// A model of magpie's chosen for OpenCode that a provider of the file's own
// already sends to magpie is named there, and magpie's flat provider isn't
// added beside it with the same models again.
func TestOpenCodeKeepsItsOwnGatewayProviders(t *testing.T) {
	home := syncHome(t)
	cfg := filepath.Join(home, ".config", "opencode", "opencode.json")
	writeFile(t, cfg, `{
  "provider": {
    "magpie-relay": {"npm": "@ai-sdk/openai-compatible", "options": {"baseURL": "http://localhost:3425/v1/"}, "models": {"relay/glm-4.6": {}}},
    "elsewhere": {"options": {"baseURL": "https://example.com/v1"}, "models": {"relay/glm-4.6": {}}}
  },
  "model": "magpie-relay/relay/glm-4.6"
}`)
	oc := opencode(home, filepath.Join(home, ".config"))
	if err := oc.Fields[0].Set("magpie/relay/glm-4.6"); err != nil {
		t.Fatal(err)
	}
	if v, _ := edit.GetJSON(cfg, "model"); v != "magpie-relay/relay/glm-4.6" {
		t.Fatalf("model = %q", v)
	}
	if _, ok := edit.GetJSON(cfg, "provider.magpie"); ok {
		t.Fatal("magpie's provider added beside the file's own")
	}
	// a model none of them lists still gets magpie's provider
	os.WriteFile(cfg, []byte(`{"provider": {"magpie-relay": {"options": {"baseURL": "http://127.0.0.1:3425/v1"}, "models": {}}}}`), 0o644)
	if err := oc.Fields[0].Set("magpie/relay/glm-4.6"); err != nil {
		t.Fatal(err)
	}
	if v, _ := edit.GetJSON(cfg, "model"); v != "magpie/relay/glm-4.6" {
		t.Fatalf("model = %q", v)
	}
	if _, ok := edit.GetJSON(cfg, "provider.magpie"); !ok {
		t.Fatal("magpie's provider not added")
	}
}
