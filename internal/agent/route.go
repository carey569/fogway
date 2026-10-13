package agent

import (
	"slices"
	"strings"

	"github.com/fogway/fogway/internal/catalog"
	"github.com/fogway/fogway/internal/gateway"
	"github.com/fogway/fogway/internal/provider"
	"github.com/fogway/fogway/internal/settings"
)

// Every agent's model picker has two halves: the models the agent reaches
// on its own (its sign-in, its own keys), and fogway's catalog — every model
// of every provider the user added, reached through the local gateway.
// A catalog model is spelled "provider/model"; an agent that sends that to
// the gateway gets the vendor's reply in whichever API the agent speaks.

// fogwayID is the provider id agents know the gateway by.
const fogwayID = "fogway"

// RoutingGroups is the picker's group of routing groups.
const RoutingGroups = "Routing groups"

// viaFogway lists the catalog as agent is shown it, for a picker: the
// routing groups first, in one group of their own, then one group per
// provider.
func viaFogway(agent, prefix string) []Option {
	var out, groups []Option
	shown, _ := provider.CatalogFor(agent)
	own := map[string]bool{} // the agent's own sign-in among the providers: none
	fast := provider.FastPicks(agent)
	for _, e := range shown {
		if e.Group != "" {
			groups = append(groups, Option{Value: prefix + e.ID, Label: e.Name, Note: "routing group · via fogway",
				Icon: e.Provider.Icon, Icons: e.Icons, Group: RoutingGroups, Ref: e.ID})
			continue
		}
		note := e.Provider.Name + " · via fogway"
		o := Option{Value: prefix + e.ID, Label: e.Name, Note: note,
			Icon: e.Provider.Icon, Group: e.Provider.Name, Ref: e.ID, Free: e.Free, Rate: e.Rate, RateWas: e.RateWas, Context: e.Context, own: own[e.Provider.ID],
			sub: false}
		// a model with a fast mode is switched fast in the picker (#954)
		if provider.CanFast(e.Provider, e.Model) {
			o.FastFor, o.Fast = agent, slices.Contains(fast, e.ID)
			o.OwnFast = provider.OwnFast(e.Provider, e.Model)
		}
		out = append(out, o)
	}
	return append(groups, out...)
}

// listLabels are the names the agent's own list calls the catalog's models
// by, by id: provider.Labels, which follows the Provider in model names
// setting, where an Option's Label is the name alone for fogway's picker.
func listLabels(agent string) map[string]string {
	shown, _ := provider.CatalogFor(agent)
	labels := provider.Labels(shown)
	out := make(map[string]string, len(shown))
	for i, e := range shown {
		out[e.ID] = labels[i]
	}
	return out
}

// viaFogwayFor is viaFogway without the agent's own account: Codex CLI going
// through fogway to its own ChatGPT login would only add a hop.
func viaFogwayFor(agentID, prefix string) []Option {
	var out []Option
	for _, o := range viaFogway(agentID, prefix) {
		if !strings.HasPrefix(o.Value, prefix+agentID+"/") {
			out = append(out, o)
		}
	}
	return out
}

// isFogway reports whether a model value is a catalog reference.
func isFogway(v string) bool {
	if pid, _, ok := strings.Cut(v, "/"); ok && dryProviders[pid] {
		return true
	}
	_, _, ok := provider.Resolve(v)
	return ok && strings.Contains(v, "/")
}

func firstOf(xs []string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return ""
}

// fogwayModels is the catalog as agent is shown it, as catalog.Models, for
// agents that keep their own model files.
func fogwayModels(agent string) []catalog.Model {
	// the providers built once for all the models' prices, not for each:
	// dsh's round at every start built them for each of hundreds
	defer provider.Hold()()
	var out []catalog.Model
	shown, _ := provider.CatalogFor(agent)
	labels := provider.Labels(shown)
	// a model fogway describes images to takes them (provider.Described)
	seen := provider.Described != nil && provider.Described()
	st, find := settings.Load(), provider.GroupFinder()
	members := func(id string) []provider.Member {
		_, ms, _ := find(id)
		return ms
	}
	for i, e := range shown {
		m := catalog.Model{ID: e.ID, Name: labels[i], Provider: firstOf(e.Provider.Catalogs()), Efforts: e.Efforts, Images: e.Images || seen, ImageInput: e.ImageInput, Context: e.Context, Output: e.Output, AgentsV2: e.AgentsV2, Reasoning: e.Reasoning}
		// where Codex and Claude Code compact it (#876): a threshold the
		// user set on the model or on its provider, else a window they
		// typed whole (#1809), else the one for every model, which codexcat
		// answers from the settings it is given
		m.Compact = provider.CompactFor(st, e, find)
		// what a call costs the user (Pi's cost, #781)
		if pr, ok := provider.EntryPriceIn(st, find, e); ok {
			m.Price = &pr
		}
		if seen && !e.Images {
			yes := true
			m.ImageInput = &yes
		}
		// APIs is the one to ask it on for the gateway to relay the request
		// as it is. A group's members may each want another, so a group has
		// one only when every member is served best on Responses (#1594,
		// ObsidianArch02): Pi, omp, droid and dsh then ask it there, and a
		// GPT group's tool-call arguments stream as the member sends them
		// instead of being held for a Chat answer
		if e.Group == "" {
			if n := e.Provider.Native(e.Model); n != "" {
				m.APIs = []string{string(n)}
			}
		} else if allResponses(members(e.ID)) {
			m.APIs = []string{string(provider.Responses)}
		}
		// the service tiers Codex is offered on it, as the gateway's own
		// lists offer them: Fast in the Codex CLI fogway writes
		// fogway-models.json for (#1530)
		provider.CodexTiers(&m, e, members)
		out = append(out, m)
	}
	return out
}

// allResponses reports whether a group's members are all served best on
// Responses, so the gateway relays a Responses request to whichever it
// picks. A group with none, or one member on another API, is false: a
// Messages-only group stays on Chat too, as the thinking a Claude member
// takes is known by its own model, not by the group's id.
func allResponses(ms []provider.Member) bool {
	for _, m := range ms {
		if m.Provider.Native(m.Model) != provider.Responses {
			return false
		}
	}
	return len(ms) > 0
}

// maxTokens is the output limit an agent is handed for m, kept within the
// context window it is handed with it: models.dev lists some models' output
// above their window (deepseek-chat's 384000 against 128000). An unknown
// window leaves the output as it is.
func maxTokens(m catalog.Model) int { return provider.OutputWithin(m.Context, m.Output) }

// group tags every option with a group name.
func group(name string, opts []Option) []Option {
	for i := range opts {
		opts[i].Group = name
	}
	return opts
}

func gatewayV1() string { return gateway.URL() + "/v1" }
