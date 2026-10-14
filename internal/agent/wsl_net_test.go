package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/carey569/fogway/internal/gateway"
)

// A .wslconfig Notepad saved as "Unicode" (UTF-16LE with a BOM), as
// "Unicode big endian", or UTF-16LE with no BOM, is read as WSL reads it
// (lgtm on Discord: mirrored all along, and told it wasn't).
func TestWSLConfigUTF16(t *testing.T) {
	cfg := "[wsl2] # networking\r\nmemory=8GB\r\nnetworkingMode=mirrored\r\n"
	be := []byte{0xfe, 0xff}
	for _, u := range utf16.Encode([]rune(cfg)) {
		be = append(be, byte(u>>8), byte(u))
	}
	for name, b := range map[string][]byte{
		"utf-8":            []byte(cfg),
		"utf-8 bom":        append([]byte{0xef, 0xbb, 0xbf}, cfg...),
		"utf-16le bom":     utf16le(cfg, true),
		"utf-16le, no bom": utf16le(cfg, false),
		"utf-16be bom":     be,
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		os.WriteFile(filepath.Join(home, ".wslconfig"), b, 0o644)
		if !wslMirrored(wslConfig()) {
			t.Errorf("%s: not mirrored: %q", name, wslConfig())
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if wslConfig() != "" || wslMirrored(wslConfig()) {
		t.Error("no .wslconfig")
	}
}

// The probe takes WSL's networking mode from wslinfo; an empty answer or
// an old wslinfo's usage text says nothing.
func TestParseProbeNet(t *testing.T) {
	for out, want := range map[string]string{
		"home:/home/me\nnet:mirrored\n":                    "mirrored",
		"home:/home/me\nnet:NAT\r\n":                       "nat",
		"home:/home/me\nnet:\n":                            "",
		"home:/home/me\nnet:Usage: wslinfo [Options...]\n": "",
		"home:/home/me\n":                                  "",
	} {
		if d := parseProbe("U", out); d == nil || d.Net != want {
			t.Errorf("%q: %+v", out, d)
		}
	}
	if !strings.Contains(wslProbeScript, "wslinfo --networking-mode") {
		t.Error("probe doesn't ask wslinfo")
	}
}

// A distro that stopped is probed again once it runs, so WSL restarted in
// another networking mode is seen without restarting fogway.
func TestWSLReprobedAfterStop(t *testing.T) {
	home, _ := codexHome(t, "", "model = \"gpt-5.5\"\n")
	fakeWSL(t, "Ubuntu\r\n", "Ubuntu\r\n", nil, map[string]string{"Ubuntu": filepath.Dir(home)})
	running, net, probes := true, "nat", 0
	wslRun = func(_ time.Duration, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "-l -q":
			return utf16le("Ubuntu\r\n", true), nil
		case "-l --running -q":
			if running {
				return utf16le("Ubuntu\r\n", true), nil
			}
			return nil, nil
		}
		if len(args) > 2 && args[0] == "-d" {
			probes++
			return []byte("home:/" + filepath.Base(home) + "\ndir:.codex\nnet:" + net + "\n"), nil
		}
		return nil, errors.New("unexpected wsl.exe " + strings.Join(args, " "))
	}
	look := func() distro {
		wsl.Lock()
		wsl.at, wsl.runAt = time.Time{}, time.Time{}
		wsl.Unlock()
		ds := wslDistros()
		if len(ds) != 1 {
			t.Fatalf("%+v", ds)
		}
		return ds[0]
	}
	if d := look(); d.Mirrored || probes != 1 {
		t.Fatalf("%+v, %d probes", d, probes)
	}
	if d := look(); probes != 1 {
		t.Fatalf("probed again while running: %+v", d)
	}
	running = false
	if d := look(); d.Running || probes != 1 {
		t.Fatalf("stopped: %+v, %d probes", d, probes)
	}
	running, net = true, "mirrored"
	if d := look(); !d.Mirrored || probes != 2 {
		t.Fatalf("restarted mirrored: %+v, %d probes", d, probes)
	}
}

// An agent an older fogway pointed at a consomme distro's default route
// (#1230's 198.18.0.2) is offered 127.0.0.1 once the distro is probed again.
func TestWSLConsommeMovesOffRoute(t *testing.T) {
	home, _ := codexHome(t, "", "model = \"gpt-5.5\"\n")
	probe := "home:/" + filepath.Base(home) + "\ndir:.codex\nroute:default via 198.18.0.2 dev eth0\nnet:consomme\n"
	fakeWSL(t, "Ubuntu\r\n", "Ubuntu\r\n", map[string]string{"Ubuntu": probe},
		map[string]string{"Ubuntu": filepath.Dir(home)})
	// wsl.json as a fogway that took the route for Windows left it
	os.MkdirAll(filepath.Dir(wslStatePath()), 0o755)
	if err := os.WriteFile(wslStatePath(), []byte(`{"Ubuntu":{"name":"Ubuntu","home":"/`+filepath.Base(home)+
		`","root":"","has":{"dir:.codex":true},"probe":2,"gateway":"198.18.0.2","net":"consomme"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if ds := wslDistros(); len(ds) != 1 || ds[0].Gateway != "" {
		t.Fatalf("%+v", ds)
	}
	old := "http://198.18.0.2:" + gateway.Port()
	if to, name := wslMovedFrom(old + "/v1"); to != gateway.URL() || name != "Ubuntu" {
		t.Fatalf("moved from %s: %q %q", old, to, name)
	}
	if to, _ := wslMovedFrom("http://10.1.2.3:" + gateway.Port()); to != "" {
		t.Fatalf("a host the distro never had moved to %q", to)
	}
}
