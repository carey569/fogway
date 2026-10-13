package provider

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func providerFileHome(t *testing.T) {
	t.Helper()
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("PATH", home)
}

const originalProviderFile = `{"providers":[{"id":"original","name":"Original","chat":"https://original.example.invalid","key":"synthetic-original","models":["model"]}],"groups":[{"id":"original-group","members":["original/model"]}]}`

// The automatic /v1 correction has no error result: leave both the URL
// and the saved file alone when its read fails.
func TestFixV1KeepsInvalidFile(t *testing.T) {
	providerFileHome(t)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte(strings.Replace(originalProviderFile, `[{"id":"original-group","members":["original/model"]}]`, `"not a list"`, 1))
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	base := "https://original.example.invalid"
	p := Provider{ID: "original", Chat: base}
	if got := p.fixV1(base, base+"/v1/models"); got != base {
		t.Errorf("failed correction returned %q, want %q", got, base)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("invalid providers.json was overwritten: %v", err)
	}
}
