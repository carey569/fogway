package provider

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/carey569/fogway/internal/lastgood"
	"github.com/carey569/fogway/internal/steady"
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

// providers.json zeroed: its providers are its last good generation's.
func TestZeroedProvidersReadFromBackup(t *testing.T) {
	providerFileHome(t)
	lastgood.Reset()
	t.Cleanup(lastgood.Reset)
	writeProviderFile(t, originalProviderFile)
	if err := Save(Provider{ID: "new", Name: "New", Chat: "https://new.example.invalid/v1", Key: "synthetic-new"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(Path(), make([]byte, 2048), 0o600)
	if err := FileError(); err != nil {
		t.Fatalf("a zeroed providers.json with a backup: %v", err)
	}
	ps := load().Providers
	if len(ps) != 1 || ps[0].ID != "original" {
		t.Fatalf("providers = %+v, want the backup's", ps)
	}
}
