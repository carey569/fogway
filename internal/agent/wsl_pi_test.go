package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/edit"
)

// piDistroHome is a distro's / in a temp dir with a user whose ~/.pi holds
// settings (none when ""); magpie's own HOME is another temp dir, whose
// ~/.pi nothing may touch.
func piDistroHome(t *testing.T, settings string) (root, home string) {
	t.Helper()
	syncHome(t)
	root = t.TempDir()
	home = filepath.Join(root, "home", "me")
	os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755)
	if settings != "" {
		os.WriteFile(filepath.Join(home, ".pi", "agent", "settings.json"), []byte(settings), 0o644)
	}
	return root, home
}

// The probe asks after Pi as after Codex, and reads what it says.
func TestWSLProbeFindsPi(t *testing.T) {
	for _, want := range []string{`[ -d "$HOME/.pi" ] && echo dir:.pi`, `p=$(command -v pi 2>/dev/null || wslbin pi) && echo "bin:pi $p"`,
		`[ -d "$HOME/.codex" ] && echo dir:.codex`, `p=$(command -v codex 2>/dev/null || wslbin codex) && echo "bin:codex $p"`} {
		if !strings.Contains(wslProbeScript, want) {
			t.Errorf("probe lacks %q:\n%s", want, wslProbeScript)
		}
	}
	d := parseProbe("U", "home:/home/me\ndir:.pi\nbin:pi\n")
	if d == nil || !d.Has["dir:.pi"] || !d.Has["bin:pi"] || !wslFound(*d) {
		t.Fatalf("%+v", d)
	}
}

// A running distro with Pi — its ~/.pi, or pi on its PATH alone — is
// listed as Pi · WSL <distro>, with the model Pi starts on; one with Codex
// and Pi has both; one with neither has none.
func TestWSLPiDiscovered(t *testing.T) {
	root, home := piDistroHome(t, `{"defaultProvider": "xai", "defaultModel": "grok-4.6", "defaultThinkingLevel": "high"}`)
	codexRoot := t.TempDir()
	os.MkdirAll(filepath.Join(codexRoot, "root", ".codex"), 0o755)
	os.MkdirAll(filepath.Join(codexRoot, "root", ".pi", "agent"), 0o755)
	os.WriteFile(filepath.Join(codexRoot, "root", ".codex", "config.toml"), []byte("model = \"gpt-5.5\"\n"), 0o644)
	binRoot := t.TempDir()
	asked, opened := fakeWSL(t, "Ubuntu\r\nArch\r\nBoth\r\nNone\r\n", "Ubuntu\r\nArch\r\nBoth\r\nNone\r\n", map[string]string{
		"Ubuntu": "home:/home/me\ndir:.pi\nroute:default via 172.20.0.1 dev eth0\n",
		"Arch":   "home:/home/a\nbin:pi\n",
		"Both":   "home:/root\ndir:.codex\nbin:codex\ndir:.pi\n",
		"None":   "home:/home/n\n",
	}, map[string]string{"Ubuntu": root, "Arch": binRoot, "Both": codexRoot})

	ds := wslDistros()
	if len(ds) != 3 || strings.Join(*opened, ",") != "Ubuntu,Arch,Both" || len(*asked) != 4 {
		t.Fatalf("distros %+v, asked %v, opened %v", ds, *asked, *opened)
	}
	var ids []string
	byID := map[string]*Agent{}
	for _, a := range wslAgentsOf(ds) {
		ids = append(ids, a.ID)
		byID[a.ID] = a
	}
	if got := strings.Join(ids, ","); got != "pi@wsl:Ubuntu,pi@wsl:Arch,codex@wsl:Both,pi@wsl:Both" {
		t.Fatalf("agents %s", got)
	}
	a := byID["pi@wsl:Ubuntu"]
	if a.Name != "Pi · WSL Ubuntu" || a.Icon != "pi" || a.WSL != "Ubuntu" || a.Bin != "" || a.UA != nil ||
		a.Path != filepath.Join(home, ".pi", "agent", "settings.json") {
		t.Fatalf("%+v", a)
	}
	if v := a.Values(); v["model"] != "xai/grok-4.6" || v["effort"] != "high" {
		t.Fatalf("values %v", v)
	}
	// Pi's model is kept apart from Codex's in the same distro
	byID["codex@wsl:Both"].Values()
	byID["pi@wsl:Both"].Values()
	wslSave()
	var kept map[string]*distro
	json.Unmarshal([]byte(readFile(wslStatePath())), &kept)
	if kept["Ubuntu"].Values["pi.model"] != "xai/grok-4.6" || kept["Both"].Values["model"] != "gpt-5.5" ||
		kept["Both"].Values["pi.model"] != "" || kept["None"] != nil {
		t.Fatalf("kept %s", readFile(wslStatePath()))
	}
}

// A stopped distro's Pi is never asked anything nor opened: it is listed
// as magpie last saw it, its options come from magpie alone, and a pick
// starts the distro, then writes its files.
func TestWSLPiStopped(t *testing.T) {
	root, home := piDistroHome(t, `{"defaultProvider": "xai", "defaultModel": "on-disk"}`)
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	before, _ := os.Stat(settings)
	b, _ := json.Marshal(map[string]*distro{
		"Stopped": {Name: "Stopped", Home: "/home/me", Root: root, Has: map[string]bool{"dir:.pi": true},
			Values: map[string]string{"pi.model": "xai/grok-4.6", "pi.effort": "low"}},
	})
	os.MkdirAll(filepath.Dir(wslStatePath()), 0o755)
	os.WriteFile(wslStatePath(), b, 0o600)
	asked, opened := fakeWSL(t, "Stopped\r\n", "", nil, nil)

	ds := wslDistros()
	if len(ds) != 1 || ds[0].Running || len(*asked) != 0 || len(*opened) != 0 {
		t.Fatalf("%+v asked %v opened %v", ds, *asked, *opened)
	}
	as := wslAgentsOf(ds)
	if len(as) != 1 {
		t.Fatalf("%d agents", len(as))
	}
	a := as[0]
	if a.ID != "pi@wsl:Stopped" || a.Icon != "pi" || a.Path != "" || a.Dir != "" || a.Check != nil || a.Sync != nil {
		t.Fatalf("%+v", a)
	}
	if v := a.Values(); v["model"] != "xai/grok-4.6" || v["effort"] != "low" {
		t.Fatalf("values %v", v)
	}
	if len(a.Field("model").Options(a.Values())) == 0 || !strings.Contains(a.Notice(), "isn't running") {
		t.Fatal("options / notice")
	}
	if a.Drift() != nil {
		t.Fatal("drift")
	}
	if len(*asked) != 0 || len(*opened) != 0 {
		t.Fatalf("asked %v opened %v", *asked, *opened)
	}
	if after, _ := os.Stat(settings); !after.ModTime().Equal(before.ModTime()) || readFile(settings) != `{"defaultProvider": "xai", "defaultModel": "on-disk"}` {
		t.Fatal("a stopped distro's files were touched")
	}

	if err := a.Field("model").Set("magpie/relay/glm-4.6"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*asked, ",") != "Stopped" {
		t.Fatalf("not started first: %v", *asked)
	}
	if m, _ := edit.GetJSON(settings, "defaultModel"); m != "relay/glm-4.6" {
		t.Fatalf("set:\n%s", readFile(settings))
	}
	if !strings.Contains(readFile(filepath.Join(home, ".pi", "agent", "models.json")), `"baseUrl"`) {
		t.Fatal("no models.json")
	}
	if v := a.Field("model").Get(); v != "magpie/relay/glm-4.6" {
		t.Fatal(v)
	}
}
