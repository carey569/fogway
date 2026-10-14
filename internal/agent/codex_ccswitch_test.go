package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/edit"
)

// CC Switch's Codex config, as it writes it: its "custom" table, one of
// its older versions' named by a profile, and a table of the user's own.
const ccSwitchCodex = "model_provider = \"custom\"\nmodel = \"gpt-5.4\"\nmodel_reasoning_effort = \"high\"\n\n" +
	"[model_providers.custom]\nname = \"custom\"\nbase_url = \"https://relay.example/v1\"\nwire_api = \"responses\"\nrequires_openai_auth = true\nexperimental_bearer_token = \"sk-relay\"\n\n" +
	"[model_providers.cc-switch-2]\nname = \"old\"\nbase_url = \"https://old.example/v1\"\n\n" +
	"[model_providers.mine]\nname = \"mine\"\nbase_url = \"https://mine.example/v1\"\n\n" +
	"[profiles.work]\nmodel_provider = \"cc-switch-2\"\n"

// A Codex thread keeps the provider it was started on, and CC Switch puts
// every third-party one on its "custom" table: reopened after fogway took
// Codex over, it still went to the relay, a fogway model picked in it
// too, and the relay said the model wasn't found. While a fogway model is
// on, CC Switch's tables go through fogway too, and get their own base URL
// back when Codex steps off fogway; a profile's table and the user's own
// stay as they are.
func TestCodexTakesCCSwitchTables(t *testing.T) {
	for _, auth := range []string{`{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, ""} {
		home, read := codexHome(t, auth, ccSwitchCodex)
		path := filepath.Join(home, ".codex", "config.toml")
		cx := codex(home)
		table := func(id string) map[string]string {
			tb, err := edit.GetTOMLTable(path, "model_providers."+id)
			if err != nil {
				t.Fatal(err)
			}
			return tb
		}
		v1 := here(home).v1()
		if err := cx.Fields[0].Set("fake/m1"); err != nil {
			t.Fatal(err)
		}
		if c := table("custom"); c["base_url"] != v1 || c["experimental_bearer_token"] != "sk-relay" || c["requires_openai_auth"] != "true" {
			t.Fatalf("auth %q: custom not through fogway:\n%s", auth, read())
		}
		if table("cc-switch-2")["base_url"] != "https://old.example/v1" || table("mine")["base_url"] != "https://mine.example/v1" {
			t.Fatalf("auth %q: other tables changed:\n%s", auth, read())
		}
		if d := cx.Check(); d != "" {
			t.Fatalf("auth %q: check: %s", auth, d)
		}
		// CC Switch writes its table again: the row says so, and fogway's
		// next sync takes it over again
		if err := edit.SetTOMLKey(path, "model_providers.custom", "base_url", "https://relay2.example/v1"); err != nil {
			t.Fatal(err)
		}
		if d := cx.Check(); !strings.Contains(d, "[model_providers.custom]") || !strings.Contains(d, "relay2.example") {
			t.Fatalf("auth %q: check: %q", auth, d)
		}
		if err := cx.Sync(); err != nil {
			t.Fatal(err)
		}
		if table("custom")["base_url"] != v1 || cx.Check() != "" {
			t.Fatalf("auth %q: sync:\n%s", auth, read())
		}
		// one of Codex's own models: the table's own base URL is back
		if err := cx.Fields[0].Set("gpt-5.4"); err != nil {
			t.Fatal(err)
		}
		if table("custom")["base_url"] != "https://relay2.example/v1" {
			t.Fatalf("auth %q: own model:\n%s", auth, read())
		}
		// and a reset gives it back too
		if err := cx.Fields[0].Set("fake/m1"); err != nil {
			t.Fatal(err)
		}
		if err := cx.Fields[0].Set(""); err != nil {
			t.Fatal(err)
		}
		if table("custom")["base_url"] != "https://relay2.example/v1" {
			t.Fatalf("auth %q: reset:\n%s", auth, read())
		}
		if _, ok := stashLoad()[here(home).key("codex.tables")]; ok {
			t.Errorf("auth %q: stash left", auth)
		}
	}
}

// A CC Switch table the user pointed at fogway by hand stays so when Codex
// steps off fogway: fogway gives back only what it took.
func TestCodexKeepsHandPointedCCSwitchTable(t *testing.T) {
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "")
	v1 := here(home).v1()
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"),
		[]byte("[model_providers.custom]\nname = \"custom\"\nbase_url = \""+v1+"\"\nwire_api = \"responses\"\n"), 0o644)
	cx := codex(home)
	if err := cx.Fields[0].Set("fake/m1"); err != nil {
		t.Fatal(err)
	}
	if err := cx.Fields[0].Set(""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), `base_url = "`+v1+`"`) {
		t.Fatalf("\n%s", read())
	}
}

// CC Switch's OpenAI mirror ("custom": name OpenAI, the ChatGPT sign-in, no
// base_url) left in config.toml after the user moved Codex onto the
// built-in provider and a fogway model. Codex's openai_base_url, which
// takes the built-in provider to fogway, doesn't reach a table of another
// id, so a thread started on "custom" and reopened went straight to
// ChatGPT, which refused the fogway model picked in it, and fogway never
// saw the request (#1796, WardLu). While Codex goes to fogway by the base
// URL the mirror does too; a profile's mirror is the user's to pick, and
// stepping off fogway takes the base URL off again.
func TestCodexTakesCCSwitchMirrorForItsThreads(t *testing.T) {
	const cfg = "model = \"gpt-5.4\"\n\n" +
		"[model_providers.custom]\nname = \"OpenAI\"\nrequires_openai_auth = true\nsupports_websockets = true\nwire_api = \"responses\"\n\n" +
		"[model_providers.cc-switch-3]\nname = \"OpenAI\"\nrequires_openai_auth = true\nwire_api = \"responses\"\n\n" +
		"[profiles.p]\nmodel_provider = \"cc-switch-3\"\n"
	home, read := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, cfg)
	path := filepath.Join(home, ".codex", "config.toml")
	cx := codex(home)
	table := func(id string) map[string]string {
		tb, err := edit.GetTOMLTable(path, "model_providers."+id)
		if err != nil {
			t.Fatal(err)
		}
		return tb
	}
	at := here(home)
	for round, off := range []string{"gpt-5.4", ""} {
		if err := cx.Fields[0].Set("fake/m1"); err != nil {
			t.Fatal(err)
		}
		if c := table("custom"); c["base_url"] != at.codexURL() || c["requires_openai_auth"] != "true" || c["name"] != "OpenAI" {
			t.Fatalf("round %d: the mirror isn't through fogway:\n%s", round, read())
		}
		if _, ok := table("cc-switch-3")["base_url"]; ok {
			t.Fatalf("round %d: the profile's mirror changed:\n%s", round, read())
		}
		if d := cx.Check(); d != "" {
			t.Fatalf("round %d: check: %s\n%s", round, d, read())
		}
		if err := cx.Sync(); err != nil {
			t.Fatal(err)
		}
		if table("custom")["base_url"] != at.codexURL() || cx.Check() != "" {
			t.Fatalf("round %d: sync:\n%s", round, read())
		}
		if err := cx.Fields[0].Set(off); err != nil {
			t.Fatal(err)
		}
		if _, ok := table("custom")["base_url"]; ok {
			t.Fatalf("round %d: off fogway (%q), the mirror keeps fogway's URL:\n%s", round, off, read())
		}
		if _, ok := stashLoad()[at.key("codex.mirrors")]; ok {
			t.Errorf("round %d: stash left", round)
		}
	}
	// one the user changed while fogway had it stays as they left it
	if err := cx.Fields[0].Set("fake/m1"); err != nil {
		t.Fatal(err)
	}
	if err := edit.SetTOMLKey(path, "model_providers.custom", "base_url", "https://mine.example/v1"); err != nil {
		t.Fatal(err)
	}
	if err := cx.Fields[0].Set(""); err != nil {
		t.Fatal(err)
	}
	if table("custom")["base_url"] != "https://mine.example/v1" {
		t.Fatalf("hand-set base_url lost:\n%s", read())
	}
}
