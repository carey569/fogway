package desktopdir

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// ran makes dir a userData Desktop has run in, at t.
func ran(tb testing.TB, dir string, t time.Time) {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	p := filepath.Join(dir, "Local State")
	if err := os.WriteFile(p, []byte(`{}`), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.Chtimes(p, t, t); err != nil {
		tb.Fatal(err)
	}
}
