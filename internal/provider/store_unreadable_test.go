package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProviderFile(t *testing.T, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// What lists the providers learns that providers.json can't be read
// (FileError), rather than taking it for none; start-up and the gateway
// still go on with it as empty (#415's review).
func TestFileErrorSaysUnreadable(t *testing.T) {
	providerFileHome(t)
	if err := FileError(); err != nil {
		t.Fatalf("no file is a first use: %v", err)
	}
	writeProviderFile(t, originalProviderFile)
	if err := FileError(); err != nil {
		t.Fatalf("a file that reads: %v", err)
	}
	writeProviderFile(t, originalProviderFile[:40])
	err := FileError()
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a file cut short: %v", err)
	}
	for _, want := range []string{Path(), "can't be read", "left it unchanged", "move it aside"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q doesn't say %q", err, want)
		}
	}
	if ps := load().Providers; len(ps) != 0 {
		t.Fatalf("load of a broken file: %+v", ps)
	}
	_ = All()
	_ = Catalog()
}
