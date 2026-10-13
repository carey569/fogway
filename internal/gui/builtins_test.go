package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/pluginrt"
	_ "github.com/fogway/fogway/plugins/all"
)

func builtinHome(t *testing.T) http.Handler {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	return Handler(nil, nil)
}

func postBuiltin(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/plugins/builtin", strings.NewReader(body)))
	return rec
}

// A switch is kept in plugins.yml as the user's own choice and the
// runtime follows it; an unknown one is refused.
func TestBuiltinSwitch(t *testing.T) {
	h := builtinHome(t)
	if _, err := pluginrt.Settle(true); err != nil {
		t.Fatal(err)
	}
	if rec := postBuiltin(h, `{"id":"sessions","on":false}`); rec.Code != http.StatusOK {
		t.Fatalf("sessions off: %d %s", rec.Code, rec.Body)
	} else {
		var out []builtinJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 {
			t.Fatalf("the built-ins: %+v", out)
		}
		for _, b := range out {
			// groups is off until a group is in use (Essentials)
			if b.On || b.State == pluginrt.Active {
				t.Fatalf("after sessions off: %+v", out)
			}
		}
	}
	if pluginrt.Running().Active("sessions") {
		t.Fatalf("the runtime: %+v", pluginrt.Running().Statuses())
	}
	if rec := postBuiltin(h, `{"id":"nope","on":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown plugin: %d", rec.Code)
	}
}

// A plugins.yml that doesn't parse leaves every plugin on, says so, and
// no switch writes over it.
func TestBuiltinBrokenFile(t *testing.T) {
	h := builtinHome(t)
	if err := os.MkdirAll(filepath.Dir(pluginrt.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginrt.Path(), []byte("plugins: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, b := range builtinsState() {
		if b.Broken == "" {
			t.Fatalf("not told broken: %+v", b)
		}
	}
	if rec := postBuiltin(h, `{"id":"sessions","on":false}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("switched over a broken file: %d", rec.Code)
	}
	if b, _ := os.ReadFile(pluginrt.Path()); string(b) != "plugins: [\n" {
		t.Fatalf("written over: %q", b)
	}
}

// Every string of every built-in plugin is said in every language.
func TestBuiltinsTranslated(t *testing.T) {
	src, err := os.ReadFile("assets/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh: {", `"zh-TW": {`, "ja: {", "de: {"} {
		_, block, ok := strings.Cut(string(src), "\n  "+lang+"\n")
		if !ok {
			t.Fatalf("no %s block", lang)
		}
		block, _, _ = strings.Cut(block, "\n  },")
		for _, m := range pluginrt.Registered() {
			for _, s := range []string{m.Name, m.Sub, m.One, m.Many} {
				k, _ := json.Marshal(s)
				if !strings.Contains(block, "\n    "+string(k)+": ") {
					t.Errorf("%s doesn't say %s", lang, k)
				}
			}
		}
	}
}
