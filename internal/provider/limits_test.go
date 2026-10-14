package provider

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/settings"
)

// limitsHome keeps a test off the machine's own settings, providers and
// catalogue, and forgets what an earlier test cached.
func limitsHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	catalog.Reset()
	t.Cleanup(catalog.Reset)
}

// A limit keyed by a model the provider does not serve would never be looked
// up again, so it is refused the same way a name for one is: quietly keeping
// it is a value that reads as set and is not.
func TestLimitsRefuseAModelTheProviderHasNot(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "relay", Name: "relay", Chat: "https://relay.example/v1", Key: "k", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetModelOutput("relay/typo", 4096); err == nil {
		t.Error("a reply limit for a model relay does not serve was accepted")
	}
	if err := SetContext(*p, "typo", 262144); err == nil {
		t.Error("a window for a model relay does not serve was accepted")
	}
	if n := settings.Load().ModelOutputs["relay/typo"]; n != 0 {
		t.Errorf("a refused reply limit was kept anyway: %d", n)
	}
	stored, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if n := stored.Contexts["typo"]; n != 0 {
		t.Errorf("a refused window was kept anyway: %d", n)
	}

	// "*" names every model of the provider, not one of them, and a key
	// that is not a model's at all is refused before either
	if err := SetModelOutput("relay/*", 4096); err != nil {
		t.Errorf("the provider's own every-model limit was refused: %v", err)
	}
	if err := SetModelOutput("not a model", 4096); err == nil {
		t.Error("a key that names no provider and model was accepted")
	}
}

// A model taken off a provider — fogway provider set relay models=sol —
// leaves a window and a reply limit keyed by an id the provider no longer
// serves, and both have to be clearable: refusing a --reset for it is a
// number the user can only take off by hand, with nothing to say why.
func TestLimitsComeOffAModelThatHasGone(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "relay", Name: "relay", Chat: "https://relay.example/v1", Key: "k",
		Models: []string{"sol", "mystery-x"}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetContext(*p, "mystery-x", 262144); err != nil {
		t.Fatal(err)
	}
	if err := SetModelOutput("relay/mystery-x", 131072); err != nil {
		t.Fatal(err)
	}

	// fogway provider set relay models=sol: the model is off its list
	p, err = Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	p.Models = []string{"sol"}
	if err := Save(*p); err != nil {
		t.Fatal(err)
	}
	if err := SetModelOutput("relay/mystery-x", 0); err != nil {
		t.Fatalf("--reset of a reply limit: %v", err)
	}
	if n := settings.Load().ModelOutputs["relay/mystery-x"]; n != 0 {
		t.Errorf("the reply limit of a model that has gone is kept: %d", n)
	}
	if err := SetContext(*p, "mystery-x", 0); err != nil {
		t.Fatalf("--reset of a window: %v", err)
	}
	stored, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if n := stored.Contexts["mystery-x"]; n != 0 {
		t.Errorf("the window of a model that has gone is kept: %d", n)
	}

	// setting one is still refused: it would answer no request, and
	// nothing would ever say the number is not in force
	if err := SetModelOutput("relay/mystery-x", 4096); err == nil {
		t.Error("a reply limit for a model that has gone was accepted")
	}
	if err := SetContext(*stored, "mystery-x", 262144); err == nil {
		t.Error("a window for a model that has gone was accepted")
	}
}

// A reply limit outlives the provider it was set for, and that provider can
// be deleted: the entry is then left in the settings under an id nothing
// serves any more. A removal that resolved the provider first would leave it
// with no way off but the file, so a removal is done under the key the entry
// is stored at — which is all that is left of the id to go by. With no
// provider to resolve, the two checks that keep a --reset off a key that was
// never one are all that stand between it and the wrong entry.
func TestOutputComesOffAProviderThatIsGone(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "relay", Name: "relay", Chat: "https://relay.example/v1", Key: "k",
		Models: []string{"sol", "mystery-x"}}); err != nil {
		t.Fatal(err)
	}
	// the spellings a caller has: the id itself, the provider's own
	// every-model one, and the "fogway/" the agents put in front of the
	// gateway's ids. Each is a key of the one map.
	ids := []string{"relay/mystery-x", "relay/*", "fogway/relay/sol"}
	for _, id := range ids {
		if err := SetModelOutput(id, 131072); err != nil {
			t.Fatal(err)
		}
	}
	// the provider is deleted, not switched off: nothing is left of it to
	// resolve a ref against, and the entries stay in the settings
	if err := Delete("relay"); err != nil {
		t.Fatal(err)
	}
	if got := len(settings.Load().ModelOutputs); got != len(ids) {
		t.Fatalf("%d limits are left in the settings; want %d", got, len(ids))
	}
	for _, id := range ids[:2] {
		key, dropped, err := DropModelOutput(id)
		if err != nil {
			t.Errorf("%s --reset: %v; a limit of a provider that is gone cannot be taken off", id, err)
			continue
		}
		if !dropped {
			t.Errorf("%s --reset: took a limit off that was not there", id)
		}
		if want := strings.TrimPrefix(id, "fogway/"); key != want {
			t.Errorf("%s --reset: cleared %q; want %q, the key it is stored at", id, key, want)
		}
	}
	// and the same removal through the setter every other caller reaches for
	if err := SetModelOutput(ids[2], 0); err != nil {
		t.Errorf("%s --reset through SetModelOutput: %v", ids[2], err)
	}
	if left := settings.Load().ModelOutputs; len(left) != 0 {
		t.Errorf("the settings still hold %v; want none", left)
	}
	// taking away what is not there is not a failure: it is the state the
	// entry is left in either way, and the caller is told there was none to
	// take so it does not report one that was
	if _, dropped, err := DropModelOutput("relay/mystery-x"); err != nil {
		t.Errorf("--reset of a limit that is not there: %v", err)
	} else if dropped {
		t.Error("--reset of a limit that is not there took one off")
	}
	if _, _, err := DropModelOutput("group/auto"); err == nil {
		t.Error("a routing group's was taken as a model's limit")
	}
	if _, _, err := DropModelOutput("not a model"); err == nil {
		t.Error("a key that names no provider and model was accepted")
	}
	// setting one is still refused: there is no provider whose model it
	// would be, so it would answer no request and nothing would say so
	if err := SetModelOutput("relay/mystery-x", 4096); err == nil {
		t.Error("a reply limit for a provider that is gone was accepted")
	}
	if n := settings.Load().ModelOutputs["relay/mystery-x"]; n != 0 {
		t.Errorf("a refused reply limit was kept anyway: %d", n)
	}
}

// A limit set through a display name is stored under the provider's own id,
// which is the only one outputOf and the gateway read it by, so a --reset
// spelled the same way has to resolve the name to reach it — and then clear
// the entry the provider is really answering with, not leave it standing
// under a key nothing is read by.
func TestOutputResetByADisplayName(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "c", Name: "b", Chat: "https://relay.example/v1", Key: "k",
		Models: []string{"sol", "other"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b/sol", "b/*"} {
		if err := SetModelOutput(id, 4096); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		// nothing is stored under the name: the entry is under the id the
		// provider is read by
		if n, ok := settings.Load().ModelOutputs[id]; ok {
			t.Errorf("%q is stored at %d; want the provider's own id, which is what answers", id, n)
		}
		key, dropped, err := DropModelOutput(id)
		if err != nil {
			t.Errorf("%s --reset: %v", id, err)
			continue
		}
		if !dropped {
			t.Errorf("%s --reset: took a limit off that was not there", id)
		}
		if want := "c" + strings.TrimPrefix(id, "b"); key != want {
			t.Errorf("%s --reset: cleared %q; want %q, the key it is stored at", id, key, want)
		}
		if left := settings.Load().ModelOutputs; len(left) != 0 {
			t.Errorf("%s --reset: the settings still hold %v; want none", id, left)
		}
	}
}

// A --reset of an id no provider answers to is not a failure: there is
// nothing stored under it to take away, which is the state the entry is
// left in either way. What it must not do is clear a limit of a provider it
// does resolve to, and report the user's own as gone.
func TestOutputResetOfAnIdNoProviderAnswersTo(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "c", Name: "b", Chat: "https://relay.example/v1", Key: "k",
		Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	if err := SetModelOutput("c/sol", 8192); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, key string }{
		{"typo/sol", "typo/sol"}, // no provider answers to any part of it
		{"b/typo", "c/typo"},     // a name that resolves, for a model nothing is stored under
	} {
		key, dropped, err := DropModelOutput(c.id)
		if err != nil {
			t.Errorf("%s --reset: %v; a limit that is not there is nothing to take away", c.id, err)
			continue
		}
		if dropped {
			t.Errorf("%s --reset: took a limit off that was not there", c.id)
		}
		if key != c.key {
			t.Errorf("%s --reset: cleared %q; want %q", c.id, key, c.key)
		}
		if n := settings.Load().ModelOutputs["c/sol"]; n != 8192 {
			t.Fatalf("%s --reset: c/sol answers with at most %d; want its own 8192", c.id, n)
		}
	}
	// and a limit cannot be set on one either: with no provider to resolve,
	// the checks that keep a key from being a model stand on the key itself
	if err := SetModelOutput("typo/sol", 4096); err == nil {
		t.Error("a reply limit for a model no provider serves was accepted")
	}
	if n, ok := settings.Load().ModelOutputs["typo/sol"]; ok {
		t.Errorf("a refused reply limit was kept anyway: %d", n)
	}
}

// A removal that took a limit off is a change of the catalog the agents have
// to hear of: the number they keep in files of their own goes back to the
// vendor's list only if they are told. One over a limit that was not there
// changes nothing, and is told to no one — a save and a redraw of every
// agent's model lists over a limit that did not move.
func TestLimitsTellTheAgentsOnlyOfALimitThatMoved(t *testing.T) {
	limitsHome(t)
	if err := Save(Provider{ID: "relay", Name: "relay", Chat: "https://relay.example/v1", Key: "k",
		Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetContext(*p, "sol", 262144); err != nil {
		t.Fatal(err)
	}
	if err := SetModelOutput("relay/sol", 131072); err != nil {
		t.Fatal(err)
	}
	told := 0
	catalog.Changed = func() { told++ }
	t.Cleanup(func() { catalog.Changed = nil })

	key, dropped, err := DropModelOutput("relay/sol")
	if err != nil {
		t.Fatal(err)
	}
	if !dropped {
		t.Fatal("a reply limit that was set was not taken off")
	}
	if key != "relay/sol" {
		t.Errorf("cleared %q; want the key the limit was stored at", key)
	}
	if _, ok := settings.Load().ModelOutputs[key]; ok {
		t.Error("the key the limit was stored at is still in the settings")
	}
	if told != 1 {
		t.Errorf("a reply limit taken off told the agents %d times; want once", told)
	}

	p, err = Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	told = 0
	dropped, err = DropContext(*p, "sol")
	if err != nil {
		t.Fatal(err)
	}
	if !dropped {
		t.Error("a window that was set was not taken off")
	}
	if told != 1 {
		t.Errorf("a window taken off told the agents %d times; want once", told)
	}
	stored, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if n := stored.Contexts["sol"]; n != 0 {
		t.Errorf("relay/sol still takes %d tokens; want none", n)
	}

	// and a second removal of either is over a limit that is not there:
	// nothing is written, and no agent's files are rewritten
	told = 0
	if _, dropped, err := DropModelOutput("relay/sol"); err != nil {
		t.Errorf("--reset of a reply limit that is not there: %v", err)
	} else if dropped {
		t.Error("--reset of a reply limit that is not there took one off")
	}
	if dropped, err := DropContext(*stored, "sol"); err != nil {
		t.Errorf("--reset of a window that is not there: %v", err)
	} else if dropped {
		t.Error("--reset of a window that is not there took one off")
	}
	// and through the setter every other caller goes by, which is a save
	// away from a provider rewritten for nothing
	if err := SetContext(*stored, "sol", 0); err != nil {
		t.Errorf("--reset of a window that is not there through SetContext: %v", err)
	}
	if told != 0 {
		t.Errorf("a removal over a limit that was not there told the agents %d times; want none", told)
	}
}

// #1438: a reply limit is handed out within the window beside it, and an
// unknown window or output, or one already within, is left as it is.
func TestOutputWithin(t *testing.T) {
	for _, c := range []struct{ window, output, want int }{
		{256000, 500000, 256000}, // Grok's window, models.dev's output
		{128000, 64000, 64000},   // within: unchanged
		{128000, 128000, 128000}, // at the window: unchanged
		{0, 500000, 500000},      // unknown window
		{256000, 0, 0},           // unknown output
		{0, 0, 0},
	} {
		if got := OutputWithin(c.window, c.output); got != c.want {
			t.Errorf("OutputWithin(%d, %d) = %d, want %d", c.window, c.output, got, c.want)
		}
		if got := (Entry{Context: c.window, Output: c.output}).PublishedOutput(); got != c.want {
			t.Errorf("PublishedOutput of %d/%d = %d, want %d", c.window, c.output, got, c.want)
		}
	}
}
