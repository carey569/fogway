package gateway

import (
	"github.com/yetone/magpie/internal/codexcat"
	"github.com/yetone/magpie/internal/provider"
)

// CodexCatalogPath is Codex's model list in its own shape, for a Codex that
// names magpie its model_provider (base_url <gateway>/v1) on another
// computer, with a gateway key (#1281): its model_catalog_url. /v1/models
// stays OpenAI's list, which Codex can't read, and CodexPath's list is the
// ChatGPT backend's, for a Codex signed in to ChatGPT here.
const CodexCatalogPath = "/v1/codex/models"

// codexCatalogMost is the most of a catalog Codex reads from a
// model_catalog_url (MAX_MODEL_CATALOG_BYTES in codex-rs's models
// endpoint): past it the whole list fails to load and Codex's picker has
// none of magpie's models.
const codexCatalogMost = 1 << 20

// codexTiers are the service tiers Codex here is offered on each of shown,
// by id: Fast on a ChatGPT account's GPT model, the tiers of Codex's own
// entry on a relay's. Another magpie that has this one as its provider
// offers its Codex the same, and sends the tier asked on (#1234).
func codexTiers(shown []provider.Entry) map[string][]any {
	out := map[string][]any{}
	for id, ts := range codexcat.ServiceTiers(provider.CodexCatalog(shown)) {
		for _, t := range ts {
			out[id] = append(out[id], t)
		}
	}
	return out
}
