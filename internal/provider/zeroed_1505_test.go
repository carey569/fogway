package provider

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/steady"
)

// writePrivate, which writes logins.json, migrations.json and
// providers.json, flushes the new file before the rename; one that fails
// to flush leaves the old file.
func TestWritePrivateSyncsBeforeRename(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logins.json")
	os.WriteFile(p, []byte(`["old"]`), 0o600)
	defer func() { steady.Sync = (*os.File).Sync }()
	steady.Sync = func(*os.File) error { return errors.New("the disk went away") }
	if err := writePrivate(p, []byte(`["new"]`)); err == nil {
		t.Fatal("written without a flush")
	}
	if b, _ := os.ReadFile(p); string(b) != `["old"]` {
		t.Fatalf("a failed flush left %q", b)
	}
}
