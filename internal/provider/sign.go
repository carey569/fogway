package provider

// How a request to a provider is signed, and which of its APIs a model is
// asked on. Fogway's providers are keys and URLs the user added (and
// Vertex AI, signed with the user's Google credentials); it reads no
// agent's sign-in.

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/carey569/fogway/internal/catalog"
)

// Accounts are the subscriptions an agent is signed in to. Fogway serves
// keys alone: there are none.
func Accounts() []Provider { return nil }

// APIs lists the APIs model is served on, as the provider's last model
// list said: Copilot serves its GPT models on Responses alone and its
// Claude models on Chat and Anthropic's. nil is not known, and every API
// the provider speaks may be tried. One the user set for the model
// (SetModelAPI) is the only one.
func (p Provider) APIs(model string) []Protocol {
	// the one the user said it is asked on, whatever the list says
	if proto, ok := p.ModelAPI(model); ok {
		return []Protocol{proto}
	}
	return p.ListedAPIs(model)
}

// ListedAPIs are the APIs model is served on as its vendor says, whatever
// the user set: what APIs gives with nothing set (the editor's Auto).
func (p Provider) ListedAPIs(model string) []Protocol {
	// asked for every model of every agent: read once while a request holds
	ms, _, _ := p.live()
	for _, m := range ms {
		if m.ID == model && len(m.APIs) > 0 {
			out := make([]Protocol, len(m.APIs))
			for i, a := range m.APIs {
				out[i] = Protocol(a)
			}
			return out
		}
	}
	// Bedrock has no list to say it: Claude is served on Anthropic's
	// messages alone, OpenAI's GPT models on Responses and chat
	// completions, every other model (gpt-oss too) on chat completions
	// alone
	if p.IsBedrock() {
		if bedrockClaude(model) {
			return []Protocol{Anthropic}
		}
		if bedrockGPT(model) && p.Responses != "" {
			return []Protocol{Responses, Chat}
		}
		return []Protocol{Chat}
	}
	// OpenCode serves some models on OpenAI's Responses API only (Grok,
	// GPT) or Anthropic's (Claude, MiniMax), and turns the others away:
	// models.dev says which, in the list of the gateway its URL is at,
	// whatever catalog the provider was saved with (#1215)
	if p.IsOpenCode() {
		for _, c := range p.openCodeCatalogs() {
			if a := catalog.APIOf(c, model); a != "" {
				return []Protocol{Protocol(a)}
			}
		}
	}
	return nil
}

// Sign authenticates a request to the provider: its key, or Vertex AI's
// Google token, then the user's own headers.
func (p Provider) Sign(ctx context.Context, req *http.Request, proto Protocol, body []byte) error {
	ctx = p.Via(ctx) // a sign-in refreshed on the way goes through its proxy

	if p.IsVertex() {
		// a Google token, which goes nowhere but the project's own address
		if base := p.vertexBase(); base == "" || !strings.HasPrefix(req.URL.String(), base+"/") {
			return fmt.Errorf("%s is asked only at Vertex AI, at its project's address", p.Name)
		}
		tok, err := p.vertexAccessToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for k, v := range AuthHeaders(p, proto) {
		req.Header.Set(k, v)
	}
	// The user's own headers ride on plain key+URL providers, after auth so
	// they can override a default when a gateway insists on a private scheme.
	// Written with the name as the user typed it — some gateways match
	// header names case-sensitively — in place of the same header in any
	// other case (PutUserHeader), so it is sent once, theirs.
	// A list the request has its own of (anthropic-beta) is added to, not
	// replaced: the user's betas go after those the agent asked.
	for k, v := range p.Headers {
		if ListHeader(k) {
			MergeList(req.Header, k, v)
			continue
		}
		catalog.PutUserHeader(req.Header, k, v)
	}
	return nil
}

// ListHeader is a header whose value is a comma-separated list the request
// has its own of, which a provider's header of the same name adds to:
// anthropic-beta, the betas an agent asks on each request.
func ListHeader(name string) bool { return strings.EqualFold(name, "anthropic-beta") }

// MergeList writes name in h as the items h has under it, in any case,
// followed by those of add: comma-separated, each once, empty ones left
// out, under name as written.
func MergeList(h http.Header, name, add string) {
	var vals []string
	for _, k := range slices.Sorted(maps.Keys(h)) {
		if strings.EqualFold(k, name) {
			vals = append(vals, h[k]...)
			delete(h, k)
		}
	}
	var out []string
	for _, v := range append(vals, add) {
		for _, it := range strings.Split(v, ",") {
			if it = strings.TrimSpace(it); it != "" && !slices.Contains(out, it) {
				out = append(out, it)
			}
		}
	}
	if len(out) > 0 {
		h[name] = []string{strings.Join(out, ",")}
	}
}

// Prepare adjusts a request body the way the backend wants it: a key's
// backend takes it as it is.
func (p Provider) Prepare(body []byte) []byte { return body }

// Explain is the error a refusal is passed on as: msg, as a key's backend
// has nothing to add.
func (p Provider) Explain(msg string, status int, body []byte) string { return msg }
