package agent

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestZedXDGPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux config path")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	if a := zed(home, cfg); a.Path != filepath.Join(cfg, "zed", "settings.json") {
		t.Fatalf("XDG: %s", a.Path)
	}
}

func TestZedCustomPaths(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "zedg-config")
	t.Setenv("MAGPIE_ZED_BIN", "/opt/zedg/bin/zedg")
	t.Setenv("MAGPIE_ZED_CONFIG_DIR", config)
	t.Setenv("MAGPIE_ZED_PROCESS_NAMES", "zedg,ZedG")
	a := zed(home, filepath.Join(home, ".config"))
	want := filepath.Join(config, "settings.json")
	if a.Bin != "/opt/zedg/bin/zedg" {
		t.Fatalf("custom binary: %q", a.Bin)
	}
	if a.Path != want || a.Dir != filepath.Dir(want) {
		t.Fatalf("custom config path: path=%q dir=%q", a.Path, a.Dir)
	}
}

func TestZedDefaultsStayStable(t *testing.T) {
	t.Setenv("MAGPIE_ZED_BIN", "")
	t.Setenv("MAGPIE_ZED_CONFIG_DIR", "")
	t.Setenv("MAGPIE_ZED_PROCESS_NAMES", "")
	a := zed(t.TempDir(), filepath.Join(t.TempDir(), ".config"))
	if a.Bin != "zed" {
		t.Fatalf("default binary: %q", a.Bin)
	}
	want := `(^|/)(zed|zeditor|zed-editor)( |$)`
	if got := zedProcessNames(); len(got) != 1 || got[0] != want {
		t.Fatalf("default process names: %#v", got)
	}
}

func TestZedCustomProcessNames(t *testing.T) {
	t.Setenv("MAGPIE_ZED_PROCESS_NAMES", "zedg,ZedG")
	want := []string{`(^|/)zedg( |$)`, `(^|/)ZedG( |$)`}
	if got := zedProcessNames(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("custom process names: %#v", got)
	}
}

func TestZedRelativeConfigDirUsesDefault(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, ".config")
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("MAGPIE_ZED_CONFIG_DIR", "zedg-config")
	a := zed(home, cfg)
	want := filepath.Join(cfg, "zed", "settings.json")
	if runtime.GOOS == "windows" {
		// Zed's default there is Roaming AppData, so the relative
		// custom directory falls back to it, not to the XDG shape.
		want = filepath.Join(home, "AppData", "Roaming", "Zed", "settings.json")
	}
	if a.Path != want {
		t.Fatalf("relative config directory escaped default: %q, want %q", a.Path, want)
	}
}
