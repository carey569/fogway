package shortcut

import "testing"

func TestTarget(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Tools\fogway-windows-amd64.exe`:          `C:\Tools\fogway-windows-amd64.exe`,
		`C:\Tools\fogway.exe.old`:                    `C:\Tools\fogway.exe`,
		`C:\Tools\fogway.exe.old-3`:                  `C:\Tools\fogway.exe`,
		`C:\Tools\fogway.exe.new`:                    `C:\Tools\fogway.exe`,
		`C:\Tools\FOGWAY.EXE.OLD`:                    `C:\Tools\FOGWAY.EXE`,
		`C:\old\fogway.exe`:                          `C:\old\fogway.exe`,
		`C:\Tools\fogway.exe.older`:                  `C:\Tools\fogway.exe.older`,
		`/usr/local/bin/fogway`:                      `/usr/local/bin/fogway`,
		`C:\Users\me\Downloads\fogway (1).exe.old-2`: `C:\Users\me\Downloads\fogway (1).exe`,
	} {
		if got := Target(in); got != want {
			t.Errorf("Target(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStale(t *testing.T) {
	exe := "/tools/fogway.exe"
	for _, c := range []struct {
		target, dir string
		stale       bool
	}{
		{"", "", true},                            // no shortcut yet
		{"/tools/fogway.exe", "/tools", false},    // already this one
		{"/TOOLS/Fogway.EXE", "/Tools/", false},   // the same, in another case
		{"/downloads/fogway.exe", "/tools", true}, // the exe was moved
		{"/tools/fogway.exe", "/downloads", true}, // starts in another folder
		{"/tools/fogway.exe", "", true},           // in no folder
	} {
		if got := Stale(c.target, c.dir, exe); got != c.stale {
			t.Errorf("Stale(%q, %q) = %v, want %v", c.target, c.dir, got, c.stale)
		}
	}
}
