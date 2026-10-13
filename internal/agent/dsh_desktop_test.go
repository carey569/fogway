package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dshAppPlist is DeepSeek Harness Desktop's Info.plist as its 0.2.0-rc.2
// macOS build ships it (download.deepseek.com's dsh-latest-macos-arm64.dmg),
// cut to the keys around the ones read, in their order and indentation.
const dshAppPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>CFBundleDisplayName</key>
    <string>DeepSeek Harness</string>
    <key>CFBundleExecutable</key>
    <string>DeepSeek Harness</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.deepseek.dsh</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>DeepSeek Harness</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>0.2.0-rc.2</string>
    <key>CFBundleVersion</key>
    <string>0.2.0-rc.2</string>
  </dict>
</plist>
`

// dshDesktopPatch is the patch list dsh writes for a new profile
// (PROFILE_PATCH_TEMPLATE in the app's dsh-app-boot), which is what the
// desktop app's profiles/desktop/cordis.patch.yml holds after its first start.
const dshDesktopPatch = "# Your patch layer for this dsh profile, applied after every bundle layer:\n" +
	"# a top-level YAML array of loader patch entries (id-targeted config\n" +
	"# overrides, disables, and insert lists; `!!js` expressions allowed).\n" +
	"[]\n"

// The desktop app alone, its profile as its first start leaves it: magpie's
// model set goes into that profile and dsh's key store, and nothing reads it
// as no longer going through magpie.
func TestDshDesktopOnlyWired(t *testing.T) {
	home, dir, _ := dshRouteHome(t)
	t.Setenv("PATH", t.TempDir())
	desktop := filepath.Join(dir, "profiles", "desktop", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(desktop), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte(dshDesktopPatch), 0o644); err != nil {
		t.Fatal(err)
	}
	a := dsh(home)
	if a.Path != desktop {
		t.Fatalf("edits %s, not the desktop app's profile", a.Path)
	}
	if err := a.Field("model").Set("magpie/deepseek/flash"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(desktop)
	if s := string(b); !strings.Contains(s, "apiKeyEnv: "+dshKeyRef) || !strings.Contains(s, "provider: "+dshRoute) {
		t.Fatalf("the desktop profile has no route or start:\n%s", s)
	}
	if v, _ := os.ReadFile(filepath.Join(dir, ".credentials.yaml")); !strings.Contains(string(v), dshKeyRef+": ") {
		t.Fatalf("no key in dsh's store:\n%s", v)
	}
	if got := a.Field("model").Get(); got != "magpie/deepseek/flash" {
		t.Fatalf("model reads %q", got)
	}
	if d := a.Check(); d != "" {
		t.Fatalf("check: %s", d)
	}
	if d := a.Drift(); d != nil {
		t.Fatalf("drift: %+v", d)
	}
}

// The desktop app first opened while magpie runs, after the CLI's web
// profile was wired: its new profile, dsh's empty template, listed none of
// magpie's models, and the Agents page said DeepSeek Harness no longer goes
// through magpie until Reapply or magpie's next start (only the catalog sync
// filled new profiles). The serving round now fills a profile dsh has just
// made; one with entries of its own, the user's, is still left to Reapply.
func TestDshDesktopProfileMadeWhileServing(t *testing.T) {
	home, dir, web := dshRouteHome(t)
	if err := os.WriteFile(web, []byte(dshDesktopPatch), 0o644); err != nil {
		t.Fatal(err)
	}
	a := dsh(home)
	if err := a.Field("model").Set("magpie/deepseek/flash"); err != nil {
		t.Fatal(err)
	}
	if d := a.Check(); d != "" {
		t.Fatalf("web wired: %s", d)
	}
	desktop := filepath.Join(dir, "profiles", "desktop", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(desktop), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, []byte(dshDesktopPatch), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := a.Check(); !strings.Contains(d, "desktop profile") {
		t.Fatalf("the new desktop profile not said: %q", d)
	}
	if trouble := dshWiredOnce(); trouble != "" {
		t.Fatal(trouble)
	}
	b, _ := os.ReadFile(desktop)
	if s := string(b); !strings.Contains(s, "apiKeyEnv: "+dshKeyRef) || !strings.Contains(s, "provider: "+dshRoute) {
		t.Fatalf("the round left the desktop profile bare:\n%s", s)
	}
	if d := a.Check(); d != "" {
		t.Fatalf("after the round: %s", d)
	}

	// a profile with the user's own entries and no route is theirs: the
	// round doesn't put magpie in it
	own := "- id: agent-default-model\n  config:\n    provider: deepseek-official\n    model: deepseek-v4-pro\n"
	if err := os.WriteFile(desktop, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if trouble := dshWiredOnce(); trouble != "" {
		t.Fatal(trouble)
	}
	if b, _ := os.ReadFile(desktop); string(b) != own {
		t.Fatalf("the user's profile written:\n%s", b)
	}
}
