package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

// newModelCatalog is models.dev as it listed gpt-6.1-sol the day it was
// added: OpenAI's entry says it thinks, with no levels yet, while its
// family's gpt-6-sol and gpt-6-astra have theirs; Xiaomi's mimo line takes
// a thinking switch alone.
const newModelCatalog = `{
  "openai": {"models": {
    "gpt-6.1-sol": {"id":"gpt-6.1-sol","family":"gpt-sol","reasoning":true,"release_date":"2026-09-29"},
    "gpt-6-sol": {"id":"gpt-6-sol","family":"gpt-sol","reasoning":true,"release_date":"2026-09-22","reasoning_options":[{"type":"effort","values":["none","low","medium","high","xhigh","max"]}]},
    "gpt-5.6-sol": {"id":"gpt-5.6-sol","family":"gpt-sol","reasoning":true,"release_date":"2026-07-09","reasoning_options":[{"type":"effort","values":["none","low","medium","high","xhigh"]}]},
    "gpt-6-astra": {"id":"gpt-6-astra","family":"gpt-astra","reasoning":true,"release_date":"2026-09-04","reasoning_options":[{"type":"effort","values":["low","medium","high","xhigh","max"]}]},
    "gpt-4.1": {"id":"gpt-4.1","family":"gpt","reasoning":false,"release_date":"2025-04-14"}}},
  "xiaomi": {"models": {
    "mimo-v2.6-flash": {"id":"mimo-v2.6-flash","family":"mimo","reasoning":true,"release_date":"2026-09-22","reasoning_options":[{"type":"toggle"}]},
    "mimo-v2.5": {"id":"mimo-v2.5","family":"mimo","reasoning":true,"release_date":"2026-04-22"}}},
  "requesty": {"models": {
    "gpt-6.1-sol": {"id":"gpt-6.1-sol","reasoning":true}}}
}`

func newModelHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalog.CachePath(), []byte(newModelCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(catalog.Reset)
}
