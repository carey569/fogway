package provider

import (
	"fmt"
	"strings"

	"github.com/carey569/fogway/internal/settings"
)

// Codex's lead picks the model of each subagent it spawns by itself, in
// spawn_agent's model argument (willz on Discord: gpt-6-astra), and its
// own config can't overrule that pick: [agents] default_subagent_model is
// only the default for a spawn that names none. settings.CodexSubagentModel
// is the model fogway puts Codex's subagents on, whatever the lead asked
// for: the gateway rewrites a subagent's request (x-openai-subagent
// collab_spawn) to it. Any model fogway serves may be set (#1633: a lead
// on a chat provider's model, its subagents wanted on that provider too).
// A subagent's task the ChatGPT backend sealed, its lead having run there,
// opens only on a ChatGPT account again: the gateway leaves such a
// subagent on the model asked for when the one set can't open it, and the
// route trace says so. "" leaves every subagent on the model its lead
// asked for.

// CodexSubagentModel is the model Codex's subagents are put on, "" for
// the one the lead asks for.
func CodexSubagentModel() string {
	return strings.TrimSpace(heldSettings().CodexSubagentModel)
}

// CodexSubagentModels are the models and routing groups Codex's subagents
// may be put on, the ones Codex's own lists show (a model taken out of
// them, or not picked, isn't offered here either), in the catalog's order.
func CodexSubagentModels() []Entry {
	shown, _ := CatalogFor("codex")
	return shown
}

// CodexSubagentUnserved says why Codex's subagents can't be put on id now,
// "" when fogway serves it.
func CodexSubagentUnserved(id string) string {
	served := Served()
	for _, e := range served {
		if e.ID == id {
			return ""
		}
	}
	return "fogway doesn't serve it now"
}

// SetCodexSubagentModel puts Codex's subagents on id, a model or routing
// group of fogway's (provider/model, group/<id>); "" leaves them on the
// model their lead asks for. One fogway doesn't serve now is kept all the
// same — a backup or profile put back on a computer whose providers
// differ, or an account signed in again later — and the gateway leaves
// subagents on the model asked for until it is served, saying why in the
// route trace. One of Codex's own bare names is refused: it names no
// model of fogway's to put the request on.
func SetCodexSubagentModel(id string) error {
	id = strings.TrimSpace(id)
	if pid, model, ok := strings.Cut(id, "/"); id != "" && (!ok || pid == "" || model == "") {
		return fmt.Errorf("Codex's subagents can't be put on %q: pick one of fogway's models (provider/model)", id)
	}
	s := settings.Load()
	if s.CodexSubagentModel == id {
		return nil
	}
	s.CodexSubagentModel = id
	if err := settings.Save(s); err != nil {
		return err
	}
	Changed() // a held settings read sees it
	return nil
}
