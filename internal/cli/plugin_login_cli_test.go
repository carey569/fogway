package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/testenv"
)

// fakeCoHome is a HOME of the test's own with the plugin host's test
// plugin (FakeCo) added, as TestAccountsOfAPlugin sets one up.
func fakeCoHome(t *testing.T) context.Context {
	t.Helper()
	bun := testenv.Bun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("MAGPIE_BUN", bun)
	t.Cleanup(plugin.Settle)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	abs, _ := filepath.Abs("../plugin/testdata/fake/index.js")
	if _, err := plugin.Add(ctx, abs); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Providers(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// noBrowser has the sign-in pages magpie opens go nowhere. It swaps the
// opener rather than putting a do-nothing open first on PATH: a script
// written now is a program macOS checks on its first run, which took 25 to
// 55 seconds while other test binaries ran, and ate the minute the plugin
// calls have (#1284).
func noBrowser(t *testing.T) {
	t.Helper()
	old := openInBrowser
	openInBrowser = func(string) {}
	t.Cleanup(func() { openInBrowser = old })
}
