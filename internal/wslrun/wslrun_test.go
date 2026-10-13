package wslrun

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseProbe(t *testing.T) {
	out := "bin:/home/me/.local/bin/claude\n" +
		"path:/home/me/.nvm/bin:/usr/bin\n" +
		"lpath:/usr/bin:/bin\n" +
		"mount:/mnt/c/\n" +
		"route:default via 172.20.0.1 dev eth0 proto kernel\n" +
		"net:nat\n"
	tool, ok := parseProbe("Ubuntu", out)
	if !ok {
		t.Fatal("not found")
	}
	if tool.Distro != "Ubuntu" || tool.Path != "/home/me/.local/bin/claude" || tool.Mount != "/mnt/" || tool.Host != "172.20.0.1" {
		t.Fatalf("tool = %+v", tool)
	}
	if tool.Root {
		t.Fatal("no uid printed, yet Root")
	}
	// the interactive shell's PATH wins, and the bin's folder joins it
	if tool.PATH != "/home/me/.local/bin:/home/me/.nvm/bin:/usr/bin" {
		t.Fatalf("PATH = %q", tool.PATH)
	}

	// mirrored networking: the distro reaches Windows' loopback itself
	tool, _ = parseProbe("d", "bin:/usr/bin/claude\nlpath:/usr/bin\nmount:/win/c/\nroute:default via 10.0.0.1 dev eth0\nnet:mirrored\n")
	if tool.Host != "" || tool.Mount != "/win/" || tool.PATH != "/usr/bin" {
		t.Fatalf("mirrored = %+v", tool)
	}

	// consomme networking (#1230), and virtioproxy, its name before WSL
	// 2.9: 127.0.0.1 is relayed to Windows' own, and the default route is
	// Windows' next hop (a TUN proxy's 198.18.0.2), not Windows. A mode
	// wslinfo didn't name, or one that isn't these, keeps the route.
	for out, host := range map[string]string{
		"net:consomme\n":    "",
		"net:Consomme\r\n":  "",
		"net:virtioproxy\n": "",
		"net:nat\n":         "198.18.0.2",
		"net:bridged\n":     "198.18.0.2",
		"net:\n":            "198.18.0.2",
		"":                  "198.18.0.2",
	} {
		tool, _ = parseProbe("d", "bin:/usr/bin/claude\nlpath:/usr/bin\nroute:default via 198.18.0.2 dev eth0 proto kernel\n"+out)
		if tool.Host != host {
			t.Errorf("%q: Host = %q, want %q", out, tool.Host, host)
		}
	}

	// a distro whose default user is root: its Claude Code is told so
	// (the gateway's asRoot)
	for out, root := range map[string]bool{"uid:0\n": true, "uid:0\r\n": true, "uid:1000\n": false, "uid:\n": false} {
		tool, _ = parseProbe("d", "bin:/usr/bin/claude\n"+out)
		if tool.Root != root {
			t.Errorf("%q: Root = %v, want %v", out, tool.Root, root)
		}
	}
	if s := probeScript("claude"); !strings.Contains(s, `echo "uid:$(id -u)"`) {
		t.Fatalf("probe doesn't print the user: %s", s)
	}

	// no claude in the distro (the probe skips Windows' own under /mnt)
	if _, ok := parseProbe("d", "mount:/mnt/c/\n"); ok {
		t.Fatal("found nothing, yet ok")
	}
}

func TestProbeSkipsWindowsOwn(t *testing.T) {
	if s := probeScript("claude"); !strings.Contains(s, `/mnt/*|"") continue`) {
		t.Fatalf("probe takes a Windows claude under /mnt: %s", s)
	}
}

func utf16le(s string) []byte {
	b := []byte{0xff, 0xfe}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func TestArgv(t *testing.T) {
	tool := Tool{Distro: "Ubuntu", Path: "/usr/bin/claude", PATH: "/usr/bin"}
	a := tool.argv([]string{"-p"})
	if a[0] != "-d" || a[1] != "Ubuntu" || a[2] != "--exec" || a[4] != "PATH=/usr/bin" || !strings.HasPrefix(a[5], runVar+"=") || a[6] != "/usr/bin/claude" || a[7] != "-p" {
		t.Fatalf("argv = %v", a)
	}
	if b := tool.argv(nil); b[5] == a[5] {
		t.Fatal("two runs share a mark")
	}
}
