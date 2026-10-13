package provider

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/catalog"
	"github.com/fogway/fogway/internal/settings"
	"github.com/fogway/fogway/internal/testenv"
)

// prefsHome is a sandbox home where two providers serve the same model,
// which models.dev names and gives four reasoning levels.
func prefsHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	// a `security` that finds nothing: no Keychain of the user's is read
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		testenv.Program(t, filepath.Join(bin, "security"), "#!/bin/sh\nexit 44\n")
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	data := `{"a":{"models":{"sol":{"id":"sol","name":"Sol","reasoning_options":[{"type":"effort","values":["low","medium","high","max"]}]}}},
		"b":{"models":{"sol":{"id":"sol","name":"Sol","reasoning_options":[{"type":"effort","values":["low","medium","high","max"]}]}}}}`
	if err := os.WriteFile(catalog.CachePath(), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	for _, id := range []string{"a", "b"} {
		if err := Save(Provider{ID: id, Name: strings.ToUpper(id), Catalog: id, Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"sol"}}); err != nil {
			t.Fatal(err)
		}
	}
}

func entry(t *testing.T, id string) Entry {
	t.Helper()
	for _, e := range Catalog() {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no %s in the catalog", id)
	return Entry{}
}

// A model's name of the user's is kept by provider/model in settings, is
// what the catalog and the lists agents get call it, leaves the same model
// from another provider (and the group they make) alone, and goes when
// reset. The agents are told each time.
func TestModelName(t *testing.T) {
	prefsHome(t)
	touched := 0
	catalog.Changed = func() { touched++ }
	t.Cleanup(func() { catalog.Changed = nil })

	if err := SetModelName("a/sol", "  My   Sol "); err != nil {
		t.Fatal(err)
	}
	if touched != 1 {
		t.Fatalf("agents told %d times", touched)
	}
	if got := settings.Load().ModelNames["a/sol"]; got != "My Sol" {
		t.Fatalf("settings hold %q", got)
	}
	if n, ok := ModelName("a", "sol"); !ok || n != "My Sol" {
		t.Fatal(n, ok)
	}
	a, b := entry(t, "a/sol"), entry(t, "b/sol")
	// a custom name still carries its provider, the same as the vendor's
	// own name does, so the picker can still be told apart by vendor
	if a.Name != "My Sol" || a.Default != "Sol" || a.Label() != "My Sol · A" || a.Model != "sol" {
		t.Fatalf("%+v %q", a, a.Label())
	}
	if b.Name != "Sol" || b.Default != "" || b.Label() != "Sol · B" {
		t.Fatalf("%+v %q", b, b.Label())
	}
	if g := entry(t, "group/auto-sol"); g.Name != "Sol" {
		t.Fatalf("the group took a provider's name: %+v", g)
	}
	if p, m, ok := Resolve("a/sol"); !ok || p.ID != "a" || m != "sol" {
		t.Fatal("a named model no longer resolves", p.ID, m, ok)
	}
	var listed []string
	for _, m := range CodexListed() {
		listed = append(listed, m.ID+"="+m.Name)
	}
	if !slices.Contains(listed, "a/sol=My Sol · A") || !slices.Contains(listed, "b/sol=Sol · B") {
		t.Fatal(listed)
	}

	// unless the custom name already says the provider: it isn't repeated
	if err := SetModelName("a/sol", "Sol on A"); err != nil {
		t.Fatal(err)
	}
	if a := entry(t, "a/sol"); a.Label() != "Sol on A" {
		t.Fatalf("provider repeated: %+v %q", a, a.Label())
	}
	if got := (Provider{ID: "a"}).ModelNames(); got["sol"] != "Sol on A" || len(got) != 1 {
		t.Fatal(got)
	}

	// a provider's name, or a Find by it, is the same provider
	if err := SetModelName("B/sol", "Sol from B"); err != nil {
		t.Fatal(err)
	}
	if b := entry(t, "b/sol"); b.Name != "Sol from B" {
		t.Fatalf("%+v", b)
	}
	// a rename takes its names with it
	if err := Rename("b", "c"); err != nil {
		t.Fatal(err)
	}
	if c := entry(t, "c/sol"); c.Name != "Sol from B" {
		t.Fatalf("%+v", c)
	}
	if _, ok := settings.Load().ModelNames["b/sol"]; ok {
		t.Fatal("the old id's name stayed")
	}

	if err := SetModelName("a/sol", ""); err != nil {
		t.Fatal(err)
	}
	if a := entry(t, "a/sol"); a.Name != "Sol" || a.Default != "" || a.Label() != "Sol · A" {
		t.Fatalf("%+v", a)
	}
	if _, ok := settings.Load().ModelNames["a/sol"]; ok {
		t.Fatal("a reset name is still kept")
	}
	for _, bad := range []string{"sol", "nobody/sol", "group/auto-sol", "a/", "a/no-such-model"} {
		if err := SetModelName(bad, "x"); err == nil {
			t.Errorf("%s was named", bad)
		}
	}
}

// Keeping some of a model's reasoning levels offers only those, in the
// vendor's order, where fogway lists it (and so in the groups it is in);
// asking for one it hasn't fails, and all of them, or none, is a reset.
func TestModelEfforts(t *testing.T) {
	prefsHome(t)
	if err := SetModelEfforts("a/sol", []string{"high", "low", "high"}); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelEfforts["a/sol"]; !slices.Equal(got, []string{"low", "high"}) {
		t.Fatal(got)
	}
	if got := entry(t, "a/sol").Efforts; !slices.Equal(got, []string{"low", "high"}) {
		t.Fatal(got)
	}
	if got := entry(t, "b/sol").Efforts; !slices.Equal(got, []string{"low", "medium", "high", "max"}) {
		t.Fatal(got)
	}
	if got := entry(t, "group/auto-sol").Efforts; !slices.Equal(got, []string{"low", "high"}) {
		t.Fatal(got)
	}
	// the vendor is still asked at any level it has
	if got := (Provider{ID: "a", Catalog: "a"}).Efforts("sol"); len(got) != 4 {
		t.Fatal(got)
	}
	if err := SetModelEfforts("a/sol", []string{"ultra"}); err == nil {
		t.Fatal("kept a level the model hasn't")
	}
	if err := SetModelEfforts("a/sol", []string{"low", "medium", "high", "max"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings.Load().ModelEfforts["a/sol"]; ok {
		t.Fatal("every level kept is still a narrowing")
	}
	SetModelEfforts("a/sol", []string{"max"})
	if err := SetModelEfforts("a/sol", nil); err != nil {
		t.Fatal(err)
	}
	if got := entry(t, "a/sol").Efforts; len(got) != 4 {
		t.Fatal(got)
	}
	// levels kept that the vendor no longer has offer them all
	if got := effortsKept([]string{"low", "high"}, []string{"max"}); !slices.Equal(got, []string{"low", "high"}) {
		t.Fatal(got)
	}
}

// A model whose levels aren't known can be given some, which the catalog,
// the lists agents get and the gateway then take it to have; none takes
// them away, and a model with levels of its own takes no others.
func TestModelEffortsGiven(t *testing.T) {
	prefsHome(t)
	if err := Save(Provider{ID: "c", Name: "C", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"mystery-7"}}); err != nil {
		t.Fatal(err)
	}
	p, _, _ := Resolve("c/mystery-7")
	if e := entry(t, "c/mystery-7"); len(e.Efforts) != 0 || len(p.Efforts("mystery-7")) != 0 {
		t.Fatalf("unknown model has levels %v", e.Efforts)
	}
	if err := SetModelEfforts("c/mystery-7", []string{"max", "low", "high"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"low", "high", "max"}
	if e := entry(t, "c/mystery-7"); !slices.Equal(e.Efforts, want) || !slices.Equal(p.Efforts("mystery-7"), want) {
		t.Fatalf("given: %v %v", e.Efforts, p.Efforts("mystery-7"))
	}
	if err := SetModelEfforts("c/mystery-7", []string{"turbo"}); err == nil {
		t.Fatal("turbo taken")
	}
	if err := SetModelEfforts("c/mystery-7", nil); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, "c/mystery-7"); len(e.Efforts) != 0 || settings.Load().ModelEfforts["c/mystery-7"] != nil {
		t.Fatalf("taken away: %v", e.Efforts)
	}
	if err := SetModelEfforts("a/sol", []string{"xhigh"}); err == nil {
		t.Fatal("sol took xhigh")
	}
}

// Whether a model takes images is the vendor's answer until the user says
// otherwise. That answer is what agents and a group's image check see.
func TestModelImage(t *testing.T) {
	prefsHome(t)
	yes := true
	if err := SetModelImage("a/sol", &yes); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, "a/sol"); !e.Images || e.ImageInput == nil || !*e.ImageInput {
		t.Fatalf("a sees %+v", e.ImageInput)
	}
	if e := entry(t, "b/sol"); e.Images || (e.ImageInput != nil && *e.ImageInput) {
		t.Fatalf("b changed %+v images %v", e.ImageInput, e.Images)
	}
	no := false
	if err := SetModelImage("a/sol", &no); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, "a/sol"); e.Images || e.ImageInput == nil || *e.ImageInput {
		t.Fatalf("a text %+v", e.ImageInput)
	}
	if err := SetModelImage("a/sol", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings.Load().ModelImages["a/sol"]; ok {
		t.Fatal("override kept")
	}
	if err := Save(Provider{ID: "c", Name: "C", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"mystery-7"}}); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, "c/mystery-7"); e.ImageInput != nil {
		t.Fatalf("unknown said %v", *e.ImageInput)
	}
	if err := SetModelImage("c/mystery-7", &yes); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, "c/mystery-7"); !e.Images || e.ImageInput == nil || !*e.ImageInput {
		t.Fatalf("given %+v", e.ImageInput)
	}
}

// Renaming a provider moves what the user gave its models in every
// per-model map. The settings walk them by the convention themselves — each
// map[string]X of Settings named Model*, the ones there are now and any
// added later — and this holds the provider's own call to it: a map that
// walk leaves out, or skips as soon as an earlier one has moved something,
// is caught here instead of by a user whose overrides went on answering for
// the id the provider had. A map that isn't per-model — Visible, which is by
// agent id — is left as it was.
//
// The maps come from that walk and not from the convention written out
// again here: a copy keeps passing while the walk it copies goes on
// changing, so a map added to the settings later would be held to the copy
// and to nothing else.
func TestRenameMovesEveryPerModelMap(t *testing.T) {
	const atLeast = 3 // only a floor against a walk that found nothing at all
	s := settings.Settings{Visible: map[string][]string{"code": {"old/model", "old"}}}
	tables := map[string]reflect.Value{}
	settings.PerModelKeys(&s, func(name string, m reflect.Value) {
		m.Set(reflect.MakeMap(m.Type()))
		m.SetMapIndex(reflect.ValueOf("old/model").Convert(m.Type().Key()), reflect.New(m.Type().Elem()).Elem())
		tables[name] = m
	})
	if len(tables) < atLeast {
		t.Fatalf("%d per-model maps in the settings, expected at least %d", len(tables), atLeast)
	}
	if !renameModelPrefs(&s, "old", "new") {
		t.Fatal("renaming moved nothing")
	}
	keys := func(m reflect.Value) []string {
		var out []string
		for _, k := range m.MapKeys() {
			out = append(out, k.String())
		}
		slices.Sort(out)
		return out
	}
	for name, m := range tables {
		old, now := reflect.ValueOf("old/model").Convert(m.Type().Key()), reflect.ValueOf("new/model").Convert(m.Type().Key())
		if !m.MapIndex(now).IsValid() || m.MapIndex(old).IsValid() {
			t.Errorf("%s holds %v", name, keys(m))
		}
	}
	if got := s.Visible["code"]; !slices.Equal(got, []string{"old/model", "old"}) {
		t.Errorf("Visible, which is by agent id, was moved to %v", got)
	}
}

// A wire name is given only for a model the provider really serves: a relay
// lists the model under the id fogway knows and asks for it under the
// vendor's own, but one the provider does not serve is a request fogway would
// never send, so naming it is refused where it is given, as a price and a
// limit are for the same id. "*" stands for every model of the provider
// rather than for one and is allowed; a removal is exempt, so an entry left
// for a model that has since gone can still be taken away.
func TestSetUpstreamNameRefusesAModelNotServed(t *testing.T) {
	prefsHome(t)
	// "a" serves "sol" (prefsHome)
	if err := SetUpstreamName("a/nope", "vendor-c/sol"); err == nil {
		t.Error("a name for a model the provider does not serve was accepted")
	}
	if _, ok := settings.Load().ModelWires["a/nope"]; ok {
		t.Error("a refused name was stored anyway")
	}
	// the whole-provider name stands on no one model, so it is allowed
	if err := SetUpstreamName("a/*", "vendor-c/sol"); err != nil {
		t.Errorf("a name for every model of the provider was refused: %v", err)
	}
	// a model that has since gone off the provider's list can still have the
	// name it was given taken away: the removal is what the user is after
	if err := SetUpstreamName("a/sol", "vendor-c/sol-2"); err != nil {
		t.Fatal(err)
	}
	if err := SetUpstreamName("a/sol", ""); err != nil {
		t.Errorf("taking away the name of a model now gone was refused: %v", err)
	}
	if _, ok := settings.Load().ModelWires["a/sol"]; ok {
		t.Error("the name of a model now gone was not taken away")
	}
}

// A wire name is the name of a model of a provider, not of a spelling of
// that provider's id: it is kept, looked up and taken away under the id the
// provider has now, so a ref spelled with the id it had before a rename —
// which an agent still running on an older config sends — names that same
// model, and leaves no second key behind that no lookup will ever match.
func TestSetUpstreamNameFollowsARenamedProvider(t *testing.T) {
	prefsHome(t)
	if err := SetUpstreamName("a/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if err := Rename("a", "c"); err != nil {
		t.Fatal(err)
	}
	p, err := Find("c")
	if err != nil {
		t.Fatal(err)
	}
	// the rename took the name with it
	if got := settings.Load().ModelWires["c/sol"]; got != "vendor-c/sol" {
		t.Fatalf("the settings hold %q", got)
	}
	// giving one by the id the provider had is giving one for that model,
	// not a name the provider is never asked for
	if err := SetUpstreamName("a/sol", "vendor-c/sol-2"); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelWires; len(got) != 1 || got["c/sol"] != "vendor-c/sol-2" {
		t.Fatalf("the settings hold %v", got)
	}
	if got := UpstreamName(*p, "sol"); got != "vendor-c/sol-2" {
		t.Fatalf("the provider is asked for %q", got)
	}
	// and taking it away by that id takes away the one in force
	if err := SetUpstreamName("a/sol", ""); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelWires; len(got) != 0 {
		t.Fatalf("the settings still hold %v", got)
	}
	if got := UpstreamName(*p, "sol"); got != "sol" {
		t.Fatalf("the provider is asked for %q", got)
	}
}

// The same for the one name given for every model of a provider: it is kept
// under the id the provider has now, and a removal by the id it had takes
// it away rather than leaving it in force under a key nothing matches.
func TestSetUpstreamNameWildcardFollowsARenamedProvider(t *testing.T) {
	prefsHome(t)
	if err := SetUpstreamName("a/*", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if err := Rename("a", "c"); err != nil {
		t.Fatal(err)
	}
	p, err := Find("c")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetUpstreamName("a/*", "vendor-c/sol-2"); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelWires; len(got) != 1 || got["c/*"] != "vendor-c/sol-2" {
		t.Fatalf("the settings hold %v", got)
	}
	if got := UpstreamName(*p, "sol"); got != "vendor-c/sol-2" {
		t.Fatalf("the provider is asked for %q", got)
	}
	if err := SetUpstreamName("a/*", ""); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelWires; len(got) != 0 {
		t.Fatalf("the settings still hold %v", got)
	}
	if got := UpstreamName(*p, "sol"); got != "sol" {
		t.Fatalf("the provider is asked for %q", got)
	}
}

// The provider of a ref is the provider it names, however it is spelled, as
// it already is for the setters beside this one: a name a user reaches for
// a provider by is that provider, and the wire name is kept under the id
// it has. What the key check is for is a provider id that is not spelled
// the way a provider's is — a file fogway did not write, whose keys no
// lookup goes by — and not the name the provider is shown by.
func TestSetUpstreamNameTakesTheNameAProviderIsCalledBy(t *testing.T) {
	prefsHome(t)
	// the same list "a" serves sol from in prefsHome
	if err := Save(Provider{ID: "relay", Name: "My Relay", Catalog: "a", Key: "k",
		Chat: "http://127.0.0.1:1/v1", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	const ref = "My Relay/sol"
	if err := SetUpstreamName(ref, "vendor-c/sol"); err != nil {
		t.Fatalf("a ref spelled with the name the provider is shown by was refused: %v", err)
	}
	// the setters beside this one take the very same ref
	if err := SetModelName(ref, "My Sol"); err != nil {
		t.Fatalf("%q names a model of that provider, as the others have it: %v", ref, err)
	}
	// and both names are kept under the id the provider has, which is what
	// every lookup goes by
	if got := settings.Load().ModelWires; len(got) != 1 || got["relay/sol"] != "vendor-c/sol" {
		t.Fatalf("the settings hold %v", got)
	}
	if got := settings.Load().ModelNames["relay/sol"]; got != "My Sol" {
		t.Fatalf("the settings hold the name %q", got)
	}
	p, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if got := UpstreamName(*p, "sol"); got != "vendor-c/sol" {
		t.Fatalf("the provider is asked for %q", got)
	}
	if n, ok := ModelName("relay", "sol"); !ok || n != "My Sol" {
		t.Fatalf("the model is called %q (%t)", n, ok)
	}
}

// The key is checked where the name is given, and what it says names the
// setting being checked: the wire name a user cannot give, and not the
// price or the limit of the same model.
func TestSetUpstreamNameSaysWhichSettingItChecks(t *testing.T) {
	prefsHome(t)
	// a providers file fogway did not write: "a" under an id that is not
	// spelled the way a provider's is, which is what the check refuses
	f := load()
	for i := range f.Providers {
		if f.Providers[i].ID == "a" {
			f.Providers[i].ID = "my_relay"
		}
	}
	if err := store(f); err != nil {
		t.Fatal(err)
	}
	err := SetUpstreamName("my_relay/sol", "vendor-c/sol")
	if err == nil {
		t.Fatal("a wire name was given for a key the settings do not keep models under")
	}
	if !strings.Contains(err.Error(), "wire name") {
		t.Fatalf("the error does not say which setting it is about: %v", err)
	}
	if got := settings.Load().ModelWires; len(got) != 0 {
		t.Fatalf("a refused name was stored anyway: %v", got)
	}
}

// A row of the ledger is judged by the name in force when the ledger is
// read: a record keeps the name the vendor's reply gave and not the name
// the request went out under, so this is the only name there is to judge by,
// and giving a name after a call re-judges that call — which is what
// usage.Ledger's comment sets out as the trade. What makes it so is that the
// lookup takes the names as the settings stand at the time of reading them,
// with nothing in it that remembers what they held when the call went out,
// and this is that side of it.
func TestUpstreamNameInFollowsTheNamesAsTheyAreNow(t *testing.T) {
	prefsHome(t)
	// "a" serves "sol" (prefsHome)
	if got := UpstreamNameIn(settings.Load().ModelWires, "a", "sol"); got != "sol" {
		t.Fatalf("with no name given, a/sol is judged as %q; want sol", got)
	}
	// a call made and answered under the name fogway knows the model by,
	// before any name is given for it
	if err := SetUpstreamName("a/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	// the same call read afterwards is judged by the name given since, so
	// the vendor's answer naming the very model that was asked for reads
	// as another one
	if got := UpstreamNameIn(settings.Load().ModelWires, "a", "sol"); got != "vendor-c/sol" {
		t.Errorf("after a name is given, a/sol is judged as %q; want vendor-c/sol", got)
	}
	// and a model with no name of its own is judged by the provider's, the
	// same way round: whatever was in force when the call went out is not
	// what a later reading of it is measured against
	if err := SetUpstreamName("a/*", "vendor-c/one"); err != nil {
		t.Fatal(err)
	}
	if got := UpstreamNameIn(settings.Load().ModelWires, "a", "other"); got != "vendor-c/one" {
		t.Errorf("a model with no name of its own is judged as %q; want the provider's", got)
	}
	if got := UpstreamNameIn(settings.Load().ModelWires, "a", "sol"); got != "vendor-c/sol" {
		t.Errorf("with both names in force, a/sol is judged as %q; want its own over the provider's", got)
	}
}

// A ref is taken at the key its name is really stored at, and the spellings
// are tried in one order: the ref as the user spelled it, the id the provider
// has or had, and last the name it is shown by — a spelling another provider
// may be shown by as well, so it is only trusted where nothing is kept under
// either of the other two. A name outlives the provider it was given for, so
// the first is where it is when that provider is gone and its id is free
// again, and it is the one a removal has to reach.
func TestDropUpstreamNameTakesTheKeyTheNameIsStoredAt(t *testing.T) {
	prefsHome(t)
	// the name a provider is shown by, under the id it has
	if err := Save(Provider{ID: "relay", Name: "My Relay", Catalog: "a", Key: "k",
		Chat: "http://127.0.0.1:1/v1", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	if err := SetUpstreamName("My Relay/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if dropped, err := DropUpstreamName("My Relay/sol"); err != nil || !dropped {
		t.Fatalf("DropUpstreamName = %v, %v; want the name taken off by the name its provider is shown by", dropped, err)
	}
	// an id a provider had before a rename is the id every lookup still
	// goes by, and so is the one a name is taken off at
	if err := SetUpstreamName("a/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if err := Rename("a", "renamed"); err != nil {
		t.Fatal(err)
	}
	if dropped, err := DropUpstreamName("a/sol"); err != nil || !dropped {
		t.Fatalf("DropUpstreamName = %v, %v; want the name taken off by the id its provider had", dropped, err)
	}
	// an id no provider is called by and no name is kept under is a mistyped
	// one: there is nothing to take off, which is not an error, and it
	// takes nothing else away on the way to saying so
	if dropped, err := DropUpstreamName("nope/sol"); dropped || err != nil {
		t.Fatalf("DropUpstreamName on an id no provider is called by = %v, %v; want no name taken and no error", dropped, err)
	}
	if got := settings.Load().ModelWires; len(got) != 0 {
		t.Errorf("the settings hold %v; a ref naming no provider took nothing off, and nothing else either", got)
	}
}

// The id a wire name was given under is free again once its provider is
// deleted, and another provider can be shown by it — the name that provider
// is shown by is a spelling any other provider may answer to, so `b/sol`
// names a provider that is not the one a name under `b/sol` is for. Taking
// the name off through it would take away that provider's own name instead,
// a name that is really in force, and leave the one that renames a
// provider's requests for a vendor that never heard of them in the file.
func TestDropUpstreamNameWorksOffTheKeyRatherThanTheProvidersName(t *testing.T) {
	prefsHome(t)
	if err := SetUpstreamName("b/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if err := Delete("b"); err != nil {
		t.Fatal(err)
	}
	// a provider of an id of its own, c, shown by the id b that is now
	// nobody's, and with a name of its own to be left alone
	if err := Save(Provider{ID: "c", Name: "B", Catalog: "b", Key: "k",
		Chat: "http://127.0.0.1:1/v1", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	if err := SetUpstreamName("c/sol", "vendor-d/sol"); err != nil {
		t.Fatal(err)
	}
	if dropped, err := DropUpstreamName("b/sol"); err != nil || !dropped {
		t.Fatalf("DropUpstreamName = %v, %v; want the name under b/sol taken off by that key", dropped, err)
	}
	wires := settings.Load().ModelWires
	if _, ok := wires["b/sol"]; ok {
		t.Errorf("b/sol still has a name, %v; it is in force for whichever provider takes that id next", wires)
	}
	if got := wires["c/sol"]; got != "vendor-d/sol" {
		t.Errorf("c's own name is %q; the removal went through the name c is shown by", got)
	}
	// which is the whole of what taking the name off buys: a provider
	// taking the id b is asked for its model by the name fogway knows it by
	if err := Save(Provider{ID: "b", Name: "B", Catalog: "b", Key: "k",
		Chat: "http://127.0.0.1:1/v1", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("b")
	if err != nil {
		t.Fatal(err)
	}
	if got := UpstreamName(*p, "sol"); got != "sol" {
		t.Errorf("the provider taking the id b is asked for %q; want sol", got)
	}
}

// HasUpstreamName answers for the key a name is really kept under, however
// it is spelled: a name that is there is not "not kept" because the ref
// that named it carried a fogway/ in front of it, which is the spelling
// every agent uses and the one the removal standing in front of it is asked
// for by.
func TestHasUpstreamNameReadsTheKeyHoweverItIsSpelled(t *testing.T) {
	prefsHome(t)
	if err := SetUpstreamName("b/sol", "vendor-c/sol"); err != nil {
		t.Fatal(err)
	}
	if err := SetUpstreamName("b/*", "vendor-c/*"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"b/sol", "fogway/b/sol", "  b/sol  ", "fogway/b/*"} {
		if !HasUpstreamName(key) {
			t.Errorf("a name is kept under %q; HasUpstreamName says none is", key)
		}
	}
	for _, key := range []string{"b/other", "fogway/b/other", "fogway/a/sol", ""} {
		if HasUpstreamName(key) {
			t.Errorf("no name is kept under %q; HasUpstreamName says one is", key)
		}
	}
}

// A name that is only whitespace is no name at either level of the lookup,
// as it is in UpstreamNameIn: the model's own falls through to the
// provider's, and both falling through leaves the request under the id it
// was built with. A blank name read as a name would send a relay an id of
// spaces.
func TestUpstreamNameAsInIgnoresANameOfOnlyWhitespace(t *testing.T) {
	for _, tc := range []struct {
		what       string
		wires      map[string]string
		sent, want string
	}{
		{"the model's own name is blank, the provider's is not",
			map[string]string{"relay/sol": "   ", "relay/*": "vendor-c/*"}, "sol-high", "vendor-c/sol-high"},
		{"both are blank",
			map[string]string{"relay/sol": "  ", "relay/*": "\t"}, "sol-high", "sol-high"},
		{"the provider's is blank and no other model has one",
			map[string]string{"relay/sol": "   ", "relay/*": " "}, "sol-high", "sol-high"},
		{"the request goes out under the model itself",
			map[string]string{"relay/sol": "   ", "relay/*": "vendor-c/*"}, "sol", "vendor-c/sol"},
	} {
		if got := UpstreamNameAsIn(tc.wires, "relay", "sol", tc.sent); got != tc.want {
			t.Errorf("%s: asking for %q gives %q; want %q", tc.what, tc.sent, got, tc.want)
		}
	}
}

// dispatch reads an account dispatch out of the sources it is written in:
// which package a name is a constant of, and which constants are on their
// way to themselves. A module's own name is the prefix every import path in
// it has, which is how one package's constant is looked up in another.
type dispatch struct {
	t    *testing.T
	root string // the tree the module is checked out in
	mod  string // what its go.mod calls the module
	seen map[string]bool
}

// accountBackends are the agents the gateway serves a request itself for,
// where the account belongs to the agent whose backend it is: a branch that
// asks whether the provider's account is one of them and then reaches that
// backend's own serve, as against one that sends the request to a vendor.
// A test file is not read: what a test says the dispatch does is no part of
// what it does.
func (d *dispatch) accountBackends(dir string) []string {
	d.t.Helper()
	var out []string
	for _, name := range d.files(dir) {
		ast.Inspect(d.parse(name), func(n ast.Node) bool {
			ifs, ok := n.(*ast.IfStmt)
			if !ok || !d.servesItself(ifs) {
				return true
			}
			for _, agent := range d.agents(ifs.Cond, dir) {
				if !slices.Contains(out, agent) {
					out = append(out, agent)
				}
			}
			return true
		})
	}
	slices.Sort(out)
	return out
}

// servesItself says whether a branch is one where the request is answered for
// the agent that owns the account instead of sent to a vendor: it ends in
// one of the serves, each named for the backend it answers for and reached
// by every account of that agent. A branch that only reads the account — what
// a request needs from it, which model it counts tokens for — is not one,
// and neither is a call that begins with serve and serves nothing:
// servesElsewhere answers whether a provider serves something.
func (d *dispatch) servesItself(ifs *ast.IfStmt) bool {
	d.t.Helper()
	for _, stmt := range ifs.Body.List {
		own := false
		ast.Inspect(stmt, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if name := sel.Sel.Name; strings.HasPrefix(name, "serve") && len(name) > len("serve") && name[len("serve")] >= 'A' && name[len("serve")] <= 'Z' {
					own = true
				}
			}
			return !own
		})
		if own {
			return true
		}
	}
	return false
}

// agents are the ones a condition compares the provider's account against,
// and nothing at all unless it asks the account is there: the same
// comparison is made elsewhere in the gateway to tell what a request needs
// rather than where it goes.
func (d *dispatch) agents(cond ast.Expr, dir string) []string {
	d.t.Helper()
	var out []string
	account := false
	ast.Inspect(cond, func(n ast.Node) bool {
		b, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch b.Op {
		case token.NEQ:
			if d.isField(b.X, "Account") && d.isIdent(b.Y, "nil") {
				account = true
			}
		case token.EQL:
			sel, ok := b.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Agent" || !d.isField(sel.X, "Account") {
				return true
			}
			agent := d.id(dir, b.Y)
			if agent == "" {
				d.t.Errorf("the dispatch compares an account's agent against %s, which is not an id this test can read: a wire name for that account would be accepted and never sent",
					types.ExprString(b.Y))
				return true
			}
			out = append(out, agent)
		}
		return true
	})
	if !account {
		return nil
	}
	return out
}

// isField says whether the expression is a field of that name, and isIdent
// whether it is a name standing for itself.
func (d *dispatch) isField(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

func (d *dispatch) isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// id is the agent id a comparison is against, read as far as it is written:
// a string as it stands, or the constant it names — of the package being
// read, as the gateway writes its own, or of one that package imports, as
// Qoder's CN subscription is named. A constant written in terms of itself
// is no id, and says so rather than standing in for one.
func (d *dispatch) id(dir string, e ast.Expr) string {
	d.t.Helper()
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				d.t.Errorf("%s is not a string this test can read: %v", x.Value, err)
				return ""
			}
			return s
		}
	case *ast.Ident:
		return d.constant(dir, x.Name)
	case *ast.SelectorExpr:
		if pkg, ok := x.X.(*ast.Ident); ok {
			return d.constant(d.pkgDir(dir, pkg.Name), x.Sel.Name)
		}
	}
	return ""
}

// constant is what a constant of the package in dir is written as, in the
// first file that declares it, or the empty string where none does.
func (d *dispatch) constant(dir, name string) string {
	d.t.Helper()
	key := filepath.Join(dir, name)
	if d.seen[key] {
		return ""
	}
	d.seen[key] = true
	for _, file := range d.files(dir) {
		var id string
		ast.Inspect(d.parse(file), func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok || id != "" || len(spec.Names) != 1 || spec.Names[0].Name != name || len(spec.Values) != 1 {
				return true
			}
			id = d.id(dir, spec.Values[0])
			return true
		})
		if id != "" {
			return id
		}
	}
	return ""
}

// pkgDir is where the package in dir imports `pkg` from keeps its sources:
// an import path is the module's own name and a path under it, so the path
// under it is where those sources are in the same tree.
func (d *dispatch) pkgDir(dir, pkg string) string {
	d.t.Helper()
	if d.root == "" {
		d.root, d.mod = d.module()
	}
	mod := d.mod + "/"
	for _, name := range d.files(dir) {
		for _, im := range d.parse(name).Imports {
			path, err := strconv.Unquote(im.Path.Value)
			if err != nil || !strings.HasPrefix(path, mod) {
				continue
			}
			as := filepath.Base(path)
			if im.Name != nil {
				as = im.Name.Name
			}
			if as == pkg {
				return filepath.Join(d.root, filepath.FromSlash(strings.TrimPrefix(path, mod)))
			}
		}
	}
	d.t.Fatalf("no package is imported as %q from %s; the dispatch cannot be read", pkg, dir)
	return ""
}

// module is the tree the sources are in and what its go.mod calls the
// module, found by walking up to the go.mod that names it.
func (d *dispatch) module() (string, string) {
	d.t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		d.t.Fatalf("the tree the sources are in: %v", err)
	}
	for {
		if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if m, ok := strings.CutPrefix(l, "module "); ok {
					return root, strings.TrimSpace(m)
				}
			}
		}
		if up := filepath.Dir(root); up != root {
			root = up
			continue
		}
		break
	}
	d.t.Fatal("no go.mod above the sources says what the module is called; the dispatch cannot be read")
	return "", ""
}

// files are the sources of a package, its tests left out.
func (d *dispatch) files(dir string) []string {
	d.t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		d.t.Fatalf("the sources in %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			out = append(out, filepath.Join(dir, n))
		}
	}
	if len(out) == 0 {
		d.t.Fatalf("no sources in %s", dir)
	}
	slices.Sort(out)
	return out
}

func (d *dispatch) parse(name string) *ast.File {
	d.t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		d.t.Fatalf("%s: %v", name, err)
	}
	return f
}
