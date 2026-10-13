package gateway

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

// CodexPath is where Codex's built-in OpenAI provider reaches magpie, as its
// `openai_base_url`. Codex keeps its own sign-in and sends what it would send
// the ChatGPT backend: a request for one of its own models goes on there as
// it came, one for a magpie model (a catalog id, provider/model) is served
// like any other, and the model list is OpenAI's with magpie's added. Ending
// in /backend-api/codex keeps Codex treating it as that backend.
const CodexPath = "/backend-api/codex"

// codexAPIBase is where a Codex signed in with an API key sends its requests;
// its model list still comes from the ChatGPT backend. A var so tests can
// point it elsewhere.
var codexAPIBase = "https://api.openai.com/v1"

// magpieCompaction marks a compaction item magpie made: its summary, which
// only magpie reads back.
const magpieCompaction = "magpie1:"

var nativeSealedAgentPayload = regexp.MustCompile(`^gAAAAA[A-Za-z0-9_-]+={0,2}$`)

// codexCompactPrompt and codexSummaryPrefix are Codex's own (Apache-2.0,
// openai/codex, prompts/templates/compact).
const codexCompactPrompt = `You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

Include:
- Current progress and key decisions made
- Important context, constraints, or user preferences
- What remains to be done (clear next steps)
- Any critical data, examples, or references needed to continue

Be concise, structured, and focused on helping the next LLM seamlessly continue the work.`

const codexSummaryPrefix = `Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:`

// sealedTaskError is what a subagent is told whose task its lead sealed
// when nothing model names can read it. lead is the provider that
// answered the lead, "" when magpie didn't (the lead was one of Codex's
// own models) or has no record of it in the last sealerKeep. A task
// already sealed opens only where it was sealed, so the way on with it
// is said too (#1367). earlier is a provider that answered the lead's
// earlier turns but can't have sealed the task (canSeal): the lead moved
// on to Codex's own model, whose turns magpie keeps no record of, and the
// task was sealed there (plugins#70). Each says what is sealed: the task
// itself, which fork_turns="none" doesn't change (#1855).
func sealedTaskError(model, lead, earlier string) string {
	if earlier != "" {
		return fmt.Sprintf("This subagent's task was sealed by the ChatGPT backend, which answered the agent that sent it on one of Codex's own models (a subagent of this thread put there, or the lead after a switch), and only a ChatGPT account can open it. %s can't. %s answered the lead's earlier turns, but it neither seals a task nor opens one. "+sealedTaskItself+" Set Codex's subagent model on magpie's Agents page to a model of a ChatGPT account added in magpie, or to one that isn't served by the ChatGPT backend, so subagents get tasks they can read. To go on with this task, have it spawned again from an agent that isn't on Codex's own models.", model, earlier)
	}
	if lead != "" {
		return fmt.Sprintf("This subagent's task was sealed by the server that answered its lead (%s), and only that server can open it; %s can't. "+sealedTaskItself+" Give the subagent the lead's model, or a lead model that isn't served by the ChatGPT backend, so its subagents get a task they can read. To go on with this task, use a model on %s for the lead and the subagent, or have the lead spawn the subagent again.", lead, model, lead)
	}
	return fmt.Sprintf("This subagent's task was sealed by the ChatGPT backend that answered its lead, and only that backend can open it: a ChatGPT account, or the Responses API provider that answered the lead. %s is neither: magpie has no record of it answering the lead in the last 30 days. "+sealedTaskItself+" Give the subagent the lead's model, or a lead model that isn't served by the ChatGPT backend, so its subagents get a task they can read. To go on with this task, use the lead's model, or have the lead spawn the subagent again.", model)
}

// sealedTaskItself says what a sealed task is (#1855): spawn_agent's
// message, which the backend that answered the lead returns sealed (its
// function call's encrypted_function_args), and which Codex hands the
// subagent as it came, as its NEW_TASK (codex-rs multi_agents_v2.rs
// agent_message_from_tool). fork_turns="none" leaves only the lead's
// history out of the subagent's first request; the task is still sealed.
const sealedTaskItself = `What is sealed is the task itself: the message the agent that spawned this subagent wrote in spawn_agent, which Codex hands on as the backend sealed it. fork_turns="none" keeps the lead's earlier turns out of the subagent, but not the seal on its task, so it doesn't help here.`

// canSeal says whether the provider id names could have sealed a
// subagent's task, as sealedReader has who can open one: a ChatGPT
// account, or a provider of no sign-in serving the Responses API (a relay
// of the ChatGPT backend, #1109). One magpie can't find is taken as one
// that could.
func canSeal(id string) bool {
	p, err := provider.Find(id)
	if err != nil || p == nil {
		return true
	}
	if p.Account != nil {
		return p.Account.Agent == "codex"
	}
	return slices.Contains(p.Speaks(), provider.Responses)
}

// sealedReader is who can read a subagent's task its lead sealed: a
// ChatGPT account (#619). The ChatGPT backend seals spawn_agent's message
// for a lead it answers as it came (passthrough, a Codex account a group
// has as well), and only it opens it again. A provider that relays the
// ChatGPT backend's Responses API (Sub2API, #1109) seals its lead's tasks
// as well and opens them again: it reads the task when it answered the
// lead (lead, the provider id), and the task is relayed to it as it came,
// on Responses, never translated. sealers are the providers that answered
// the agents the sealed messages came from: the lead, for a subagent's
// task, and the conversation itself, for a lead its subagent's sealed reply
// comes back to (#1237) — the subagent, sent a task sealed there, had to be
// on that provider too.
func (s *Server) sealedReader(p provider.Provider, model string, sealers []string) bool {
	if p.Account != nil && p.Account.Agent == "codex" {
		return true
	}
	return p.Account == nil && slices.Contains(sealers, p.ID) && slices.Contains(s.usable(p, model), provider.Responses)
}

// sealedReaders keeps of cands those that can read a sealed subagent task,
// pl's order with them.
func (s *Server) sealedReaders(cands []candidate, pl planned, sealers []string) ([]candidate, planned) {
	var kept []candidate
	var order []Weighed
	for i, c := range cands {
		if s.sealedReader(c.p, c.model, sealers) {
			kept = append(kept, c)
			order = append(order, pl.order[i])
		}
	}
	cands, pl.order = kept, order
	pl.held = slices.DeleteFunc(slices.Clone(pl.held), func(c candidate) bool { return !s.sealedReader(c.p, c.model, sealers) })
	pl.capped = slices.DeleteFunc(slices.Clone(pl.capped), func(c candidate) bool { return !s.sealedReader(c.p, c.model, sealers) })
	return cands, pl
}

// leadProvider is the provider that answered a subagent's lead, the
// thread parent names, in any scope magpie routed it in: "" when it
// answered none of the lead's turns, or so long ago it no longer counts.
// Given a lead's own conversation, it is the provider that answered the
// lead.
func leadProvider(scope, parent string) string {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return ""
	}
	sticks.Lock()
	defer sticks.Unlock()
	st, had := stickOf(scope + "|" + parent) // reads what's kept on disk too
	if !had {
		for k, v := range sticks.m {
			if strings.HasSuffix(k, "|"+parent) && (!had || v.at.After(st.at)) {
				st, had = v, true
			}
		}
	}
	if !had || time.Since(st.at) > stickKeep {
		return ""
	}
	id, _, _ := strings.Cut(st.who, "@") // an account: provider@user
	id, _, _ = strings.Cut(id, "#")      // a key: provider#key
	return id
}

// sealerKeep is how far back the usage log is read for who answered a
// lead whose answerer affinity no longer remembers (#1367).
const sealerKeep = 30 * 24 * time.Hour

// loggedAnswer is who the usage log has answering a conversation: the
// provider, and the key or account as candidate.who names it ("" when the
// record doesn't tell).
type loggedAnswer struct {
	provider, who string
	at            time.Time
}

// loggedAnswers are the answers the usage log has to these conversations
// (a lead's thread, a request's own) in the last sealerKeep, the latest
// first: who could have sealed a message in them when affinity has let it
// go — a day and more since, a restart past the conversations
// affinity.json keeps, or the lead's last reply breaking off
// (unanswered), after which nothing could answer the lead again to be
// remembered (#1367). A call refused or failed answered nothing, a title
// isn't the conversation's own turn, and Codex's own sign-in (the ChatGPT
// backend, relayed as it came) is no provider here: a lead it answered is
// told of as one sealed by the ChatGPT backend.
func loggedAnswers(convs ...string) []loggedAnswer {
	convs = slices.DeleteFunc(slices.Clone(convs), func(c string) bool { return strings.TrimSpace(c) == "" })
	if len(convs) == 0 {
		return nil
	}
	var all []loggedAnswer
	usage.Visit(time.Now().Add(-sealerKeep), func(r usage.Record) {
		if r.Provider == "" || r.Provider == "magpie" || r.IsRejected() || r.IsReported() || r.Status <= 0 || r.Status >= 400 || r.Computer != "" || isTitleKind(r.Kind) {
			return
		}
		if r.Provider == "openai" && strings.HasPrefix(r.Endpoint, CodexPath) {
			return // Codex's own sign-in
		}
		if !slices.Contains(convs, r.NativeSession) && !slices.Contains(convs, r.Session) {
			return
		}
		a := loggedAnswer{provider: r.Provider, at: r.Time}
		switch {
		case r.ProviderAccount != "":
			a.who = r.Provider + "@" + strings.ToLower(r.ProviderAccount)
		case r.ProviderKeyID != "":
			a.who = r.Provider + "#" + r.ProviderKeyID
		}
		all = append(all, a)
	})
	slices.SortStableFunc(all, func(a, b loggedAnswer) int { return b.at.Compare(a.at) })
	return all
}

// loggedAnswerers are the providers of loggedAnswers, each once, the
// latest first.
func loggedAnswerers(convs ...string) []string {
	var ids []string
	for _, a := range loggedAnswers(convs...) {
		if !slices.Contains(ids, a.provider) {
			ids = append(ids, a.provider)
		}
	}
	return ids
}

// leadFirst puts first the account that answered the lead, the thread
// parent names, in scope: the one that sealed its subagent's task, which
// another account may not open, as it doesn't another's reasoning.
func leadFirst(scope, parent string, cands []candidate, pl planned) ([]candidate, planned, string) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return cands, pl, ""
	}
	sticks.Lock()
	st, had := stickOf(scope + "|" + parent)
	if !had {
		// a group with rules keeps the lead's conversation under its
		// first words too
		for k, v := range sticks.m {
			if strings.HasPrefix(k, scope+"|") && strings.HasSuffix(k, "|"+parent) {
				st, had = v, true
				break
			}
		}
	}
	sticks.Unlock()
	if !had || time.Since(st.at) > stickKeep {
		// affinity let the lead go: the usage log still has which key or
		// account answered it (#1367)
		st = stick{}
		for _, a := range loggedAnswers(parent) {
			if a.who != "" {
				st.who = a.who
				break
			}
		}
		if st.who == "" {
			return cands, pl, ""
		}
	}
	for i, c := range cands {
		if i > 0 && c.who() == st.who {
			cands = append(append([]candidate{c}, cands[:i]...), cands[i+1:]...)
			pl.order = append(append([]Weighed{pl.order[i]}, pl.order[:i]...), pl.order[i+1:]...)
			return cands, pl, c.rest
		}
	}
	if len(cands) > 0 && cands[0].who() == st.who {
		return cands, pl, cands[0].rest
	}
	return cands, pl, ""
}

// Only native sealed agent tasks need this guidance. Other encrypted_content
// fields (including ordinary model history) keep their existing handling.
func hasSealedAgentMessage(body []byte) bool {
	var request map[string]json.RawMessage
	if decodeJSON(body, &request) != nil {
		return false
	}
	var items []json.RawMessage
	if decodeJSON(request["input"], &items) != nil {
		return false
	}
	for _, raw := range items {
		var item struct {
			Type    string            `json:"type"`
			Content []json.RawMessage `json:"content"`
		}
		if decodeJSON(raw, &item) != nil || item.Type != "agent_message" {
			continue
		}
		for _, content := range item.Content {
			var part struct {
				Type             string `json:"type"`
				EncryptedContent string `json:"encrypted_content"`
			}
			if decodeJSON(content, &part) == nil && part.Type == "encrypted_content" && nativeSealedAgentPayload.MatchString(part.EncryptedContent) {
				return true
			}
		}
	}
	return false
}

// withoutSealedMessages is a request's input with each sealed agent message
// that is not a task left out, a note in its place; ok says one was. In
// MultiAgentV2 an agent on one of Codex's own models has its send_message
// sealed by the ChatGPT backend, and Codex hands it on as encrypted_content
// under a plain header (codex-rs protocol.rs to_model_input_item: "Message
// Type: MESSAGE" for a message, NEW_TASK for a task, spawn_agent's or
// followup_task's). A lead or subagent served by a model that can't open
// it was turned away whole, though all it lost was that message: the
// sender's final result reaches its lead as text all the same (#1613). A
// task is kept, and still turned away (serve): the subagent would have
// nothing to do. An item whose header says neither is taken as a task.
func withoutSealedMessages(body []byte) ([]byte, bool) {
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body, false
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return body, false
	}
	changed := false
	for i, raw := range items {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil || string(item["type"]) != `"agent_message"` {
			continue
		}
		var parts []map[string]any
		if json.Unmarshal(item["content"], &parts) != nil {
			continue
		}
		message, sealed := false, false
		for _, p := range parts {
			switch p["type"] {
			case "input_text":
				t, _ := p["text"].(string)
				message = message || strings.HasPrefix(t, "Message Type: MESSAGE\n")
			case "encrypted_content":
				c, _ := p["encrypted_content"].(string)
				sealed = sealed || nativeSealedAgentPayload.MatchString(c)
			}
		}
		if !message || !sealed {
			continue
		}
		var from string
		json.Unmarshal(item["author"], &from)
		if from == "" {
			from = "another agent"
		}
		note := "[magpie: this message from " + from + " was sealed by the ChatGPT backend, which answered that agent on one of Codex's own models, and only it can open it; this model can't, so it was left out. An agent's final result still reaches the agent that spawned it as text.]"
		for j, p := range parts {
			if c, _ := p["encrypted_content"].(string); p["type"] == "encrypted_content" && nativeSealedAgentPayload.MatchString(c) {
				parts[j] = map[string]any{"type": "input_text", "text": note}
			}
		}
		b, err := marshalPlain(parts)
		if err != nil {
			continue
		}
		item["content"] = b
		if b, err = marshalPlain(item); err != nil {
			continue
		}
		items[i], changed = b, true
	}
	if !changed {
		return body, false
	}
	var err error
	if q["input"], err = marshalPlain(items); err != nil {
		return body, false
	}
	nb, err := marshalPlain(q)
	if err != nil {
		return body, false
	}
	return nb, true
}

// withModel is a request body asking for another model.
func withModel(body []byte, model string) []byte {
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	q["model"], _ = json.Marshal(model)
	b, err := json.Marshal(q)
	if err != nil {
		return body
	}
	return b
}

// unreadableItem is the item OpenAI's refusal names: sealed content it
// can't verify, or an id it doesn't have ("Item with id 'rs_…' not found").
var unreadableItem = regexp.MustCompile(`(?i)(?:encrypted content for item|item with id) '?([A-Za-z]+_[A-Za-z0-9_-]+)'?`)

// codexHeader is one of the headers Codex tells the ChatGPT backend about
// a request by, which go on with it when a pool account signs it as they
// do when Codex's own sign-in does (codexUpstream): x-openai-subagent (a
// guardian review, a thread's title, memories, a compaction…), the turn's
// x-codex-turn-metadata, its installation, window and parent thread, and
// the session. Never a sign-in: the account's own goes in its place, and
// Codex's device attestation is its own sign-in's. Nor what holds for
// Codex's own account alone: x-openai-codex-luna-reserve, which says its
// plan's allowance is used up (another account's may not be), and
// x-codex-turn-state, the backend's routing of the turn for that account.
func codexHeader(k string) bool {
	k = strings.ToLower(k)
	if strings.Contains(k, "authorization") || k == "x-oai-attestation" ||
		k == "x-openai-codex-luna-reserve" || k == "x-codex-turn-state" {
		return false
	}
	switch k {
	case "session-id", "thread-id", "session_id", "conversation_id", "x-client-request-id", "version":
		return true
	}
	return strings.HasPrefix(k, "x-openai-") || strings.HasPrefix(k, "x-codex-")
}

// callKind is what an agent made a call for when it isn't a turn of the
// conversation, as Codex names it in x-openai-subagent: "guardian" (auto
// review of an approval), "review", "compact", "memory_consolidation",
// "thread_title", "collab_spawn"… A turn Codex sends on Luna Reserve, once
// the plan's own allowance is used up, is "luna_reserve". A call Codex
// makes on a hidden thread of its own goes without x-openai-subagent: its
// x-codex-turn-metadata names the thread's source instead — "thread_title"
// for the title of a new chat (#314), "guardian_review". A web search
// magpie runs for a model that can't search is "web_search".
func callKind(h http.Header) string {
	v := strings.TrimSpace(h.Get("x-openai-subagent"))
	if v == "" && h.Get("x-openai-memgen-request") != "" {
		v = "memgen"
	}
	if v == "" {
		v = threadSource(h.Get("x-codex-turn-metadata"))
	}
	if v == "" && h.Get("User-Agent") == SearchAgent {
		v = "web_search"
	}
	if v == "" && h.Get("User-Agent") == VisionAgent {
		v = "vision"
	}
	if v == "" && h.Get("User-Agent") == RouterAgent {
		v = "routing" // a group's classifier, asked before the turn (#1839)
	}
	if v == "" && h.Get("x-openai-codex-luna-reserve") != "" {
		v = "luna_reserve"
	}
	if len(v) > 40 {
		v = v[:40]
	}
	return v
}

// threadSource is the source Codex's turn metadata gives the thread a call
// was made on, when that isn't the user's conversation or a subagent's
// (which x-openai-subagent names): a feature's own thread, as Codex's
// ThreadSource has it — "thread_title", "guardian_review",
// "memory_consolidation".
func threadSource(meta string) string {
	if !strings.Contains(meta, "thread_source") {
		return ""
	}
	var m struct {
		Source string `json:"thread_source"`
	}
	if json.Unmarshal([]byte(meta), &m) != nil {
		return ""
	}
	switch s := strings.TrimSpace(m.Source); s {
	case "", "user", "subagent":
		return ""
	default:
		return s
	}
}

// codexModelsWait is how long the ChatGPT backend is given for its model
// list. Codex gives the whole request 5 s (MODELS_REFRESH_TIMEOUT in its
// models endpoint) and then keeps the list it was built with, magpie's
// models nowhere in it; a backend slow to answer, or not reachable at all
// on a network that drops chatgpt.com's packets rather than refusing
// them, held magpie's answer past that (#539). A var so tests can say.
var codexModelsWait = 3 * time.Second

// CodexOwnOnly reports whether the Codex asking for its model list at
// CodexPath reaches magpie for account failover alone, not connected to
// it (agent.CodexOwnOnly): it is then handed only its own models (#1385).
// Set by main; nil, every Codex gets magpie's models too.
var CodexOwnOnly func() bool

func codexOwnOnly() bool { return CodexOwnOnly != nil && CodexOwnOnly() }

// codexInput restores summaries magpie made. For a magpie model, it also
// replaces Codex's compaction trigger with a request to summarise the input.
// OpenAI's own models keep the trigger for their backend to handle.
func codexInput(body []byte, magpieModel bool) (_ []byte, compact bool) {
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body, false
	}
	var items []map[string]any
	if json.Unmarshal(q["input"], &items) != nil {
		return body, false
	}
	changed := false
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		switch it["type"] {
		case "compaction", "compaction_summary":
			enc, _ := it["encrypted_content"].(string)
			sum, ok := strings.CutPrefix(enc, magpieCompaction)
			if !ok {
				out = append(out, it) // OpenAI's own, for OpenAI
				continue
			}
			changed = true
			if b, err := base64.StdEncoding.DecodeString(sum); err == nil {
				out = append(out, userMessage(codexSummaryPrefix+"\n"+string(b)))
			}
		case "compaction_trigger":
			if !magpieModel {
				out = append(out, it)
				continue
			}
			changed, compact = true, true
			out = append(out, userMessage(codexCompactPrompt))
		case "reasoning":
			// OpenAI accepts no reasoning content parts in replayed input.
			// Drop the item so its encrypted_content cannot fail there too.
			if !magpieModel {
				if content, present := it["content"]; present && content != nil {
					parts, ok := content.([]any)
					if !ok || len(parts) > 0 {
						changed = true
						continue
					}
				}
				// Nor reasoning with nothing sealed in it — another vendor's,
				// only an id (rs_…): OpenAI, which keeps nothing (store is
				// false), looks the id up and answers 404 "Item … not found".
				if enc, _ := it["encrypted_content"].(string); enc == "" {
					changed = true
					continue
				}
			}
			out = append(out, it)
		default:
			out = append(out, it)
		}
	}
	if !changed {
		return body, false
	}
	b, _ := json.Marshal(out)
	q["input"] = b
	if compact {
		// the summary is text; a tool call would be no summary, so it
		// offers none. Otherwise it goes as Codex asks for its own summary
		// from a provider it reaches itself: "tools": [] (not left out,
		// #1511), streamed with its tool_choice — a relay in front of the
		// ChatGPT backend turns away one that isn't ("invalid codex
		// request", #292)
		q["tools"] = json.RawMessage("[]")
		q["tool_choice"] = json.RawMessage(`"auto"`)
		q["parallel_tool_calls"] = json.RawMessage("false")
	}
	nb, err := json.Marshal(q)
	if err != nil {
		return body, false
	}
	return nb, compact
}

// bareReasoningRefused is how unfit remembers a provider turning away, for
// model, reasoning items with nothing sealed in them (withoutBareReasoning).
func bareReasoningRefused(model string) string { return "bare reasoning\x00" + model }

// refusesInput is a 400 refusing a Responses request over its input: the
// error's param is the input, as OpenAI's own is for an item it can't find
// and a relay in front of it passes on with a message of its own ("bad
// response status code 400", #1044), or the error names an item by id.
func refusesInput(status int, b []byte) bool {
	if !badRequest(status) {
		return false
	}
	var e struct {
		Error struct {
			Param any `json:"param"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil {
		if p, _ := e.Error.Param.(string); p == "input" || strings.HasPrefix(p, "input[") || strings.HasPrefix(p, "input.") {
			return true
		}
	}
	return unreadableItem.Match(b)
}

// withoutBareReasoning is a Responses request without the reasoning items
// that have nothing sealed in them, where store isn't true: an id and a
// summary only, as magpie gives Codex in a translated reply (rs_ and
// newID). OpenAI's API and Azure OpenAI's look such an item up among the
// items they stored, and with store false they stored none, so the whole
// request goes back 400 "Item with id 'rs_…' not found" (#1008); its
// summary was the model's notes to itself, which no model reads back.
func withoutBareReasoning(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"reasoning"`)) {
		return body
	}
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	var store bool
	if json.Unmarshal(q["store"], &store) == nil && store {
		return body
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return body
	}
	kept := items[:0:0]
	for _, it := range items {
		var t struct {
			Type string `json:"type"`
			Enc  string `json:"encrypted_content"`
		}
		if json.Unmarshal(it, &t) == nil && t.Type == "reasoning" && t.Enc == "" {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == len(items) {
		return body
	}
	q["input"], _ = json.Marshal(kept)
	b, err := json.Marshal(q)
	if err != nil {
		return body
	}
	return b
}

// reasoningTextRefused is how unfit remembers a provider turning away, for
// model, reasoning items with their text in them (withoutReasoningText).
func reasoningTextRefused(model string) string { return "reasoning text\x00" + model }

// reasoningContentRefusal is OpenAI's refusal of a reasoning item with its
// text in it, naming the item: "Invalid 'input[1].content': array too
// long. Expected an array with maximum length 0, but got an array with
// length 1 instead." (the ChatGPT backend's; a relay's may word the rest
// otherwise, #1411)
var reasoningContentRefusal = regexp.MustCompile(`input\[(\d+)\]\.content'?:? array too long`)

// refusesReasoningText is a 400 refusing the reasoning text in a Responses
// request: the item the refusal names is reasoning with text in it.
func refusesReasoningText(status int, b, body []byte) bool {
	if !badRequest(status) {
		return false
	}
	m := reasoningContentRefusal.FindSubmatch(b)
	if m == nil {
		return false
	}
	it := gjson.GetBytes(body, "input."+string(m[1]))
	return it.Get("type").String() == "reasoning" && len(it.Get("content").Array()) > 0
}

// withoutReasoningText is a Responses request without the reasoning items
// that carry their text (content parts), as another vendor's model wrote
// them: DeepSeek's, which a routing group handed Codex with its own
// encrypted_content beside the text. OpenAI's API, the ChatGPT backend and
// a relay in front of them take a reasoning item's content only empty, and
// turn the whole request away over one (#1411); its seal is no more theirs
// than its text, so the item goes, as codexInput leaves it out for one of
// Codex's own models.
func withoutReasoningText(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"reasoning"`)) {
		return body
	}
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return body
	}
	kept := items[:0:0]
	for _, it := range items {
		var t struct {
			Type    string            `json:"type"`
			Content []json.RawMessage `json:"content"`
		}
		if json.Unmarshal(it, &t) == nil && t.Type == "reasoning" && len(t.Content) > 0 {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == len(items) {
		return body
	}
	q["input"], _ = json.Marshal(kept)
	b, err := json.Marshal(q)
	if err != nil {
		return body
	}
	return b
}

// openaiItemPrefix is the id prefix OpenAI takes for each kind of call item
// a vendor's reply may have handed Codex with another: magpie, or the
// vendor, gave a tool search's and a custom tool's call a function_call's
// id (fc_…), and Codex hands the item back on every later turn. OpenAI's
// own models and the ChatGPT backend turn the whole request away ("Invalid
// 'input[98].id': 'fc_…'. Expected an ID that begins with 'tsc'", "…with
// 'ctc'"), Codex's compaction with it.
var openaiItemPrefix = map[string]string{
	"tool_search_call": "tsc_",
	"custom_tool_call": "ctc_",
}

// callItemIDs is a Responses request whose call items have ids OpenAI
// takes (openaiItemPrefix): fc_X goes as tsc_X or ctc_X, the call_id the
// call's output names it by as it was, and the rest of the request byte
// for byte.
func callItemIDs(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"tool_search_call"`)) && !bytes.Contains(body, []byte(`"custom_tool_call"`)) {
		return body
	}
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return body
	}
	changed := false
	for i, raw := range items {
		var it map[string]json.RawMessage
		var typ, id string
		if json.Unmarshal(raw, &it) != nil || json.Unmarshal(it["type"], &typ) != nil {
			continue
		}
		prefix := openaiItemPrefix[typ]
		if prefix == "" || json.Unmarshal(it["id"], &id) != nil || id == "" || strings.HasPrefix(id, prefix) {
			continue
		}
		if _, rest, ok := strings.Cut(id, "_"); ok && rest != "" {
			id = prefix + rest
		} else {
			id = prefix + id
		}
		it["id"], _ = json.Marshal(id)
		if b, err := marshalPlain(it); err == nil {
			items[i], changed = b, true
		}
	}
	if !changed {
		return body
	}
	q["input"], _ = marshalPlain(items)
	nb, err := marshalPlain(q)
	if err != nil {
		return body
	}
	return nb
}

// itemIDRefusal is OpenAI's refusal of an input item whose id has a
// character other than [A-Za-z0-9_-], which Copilot passes on as it is
// (echo_ts, Codex moved from Muse Spark on OpenCode to GPT on Copilot):
// "Invalid 'input[1].id': 'rs_6aca2fc7533c979d5f0e4f7a:rs_01a125caa544754e9a3c70080100b009'.
// Expected an ID that contains letters, numbers, underscores, or dashes,
// but this value contained additional characters."
var itemIDRefusal = regexp.MustCompile(`(?s)Invalid 'input\[\d+\]\.id'.{0,1000}?Expected an ID that contains letters, numbers, underscores, or dashes`)

// itemIDsRefused is how unfit remembers a provider turning away, for
// model, an input item id with another character in it (plainItemIDs).
func itemIDsRefused(model string) string { return "item ids\x00" + model }

// validatesItemIDs reports whether p checks a Responses input item's id
// as OpenAI does, without having to be told by a refusal: OpenAI's API,
// Azure OpenAI's, the ChatGPT backend behind a Codex sign-in, and Copilot's
// /responses in front of them.
func validatesItemIDs(p provider.Provider) bool {
	return strings.HasSuffix(p.Host(), "openai.com") || p.IsAzure() ||
		p.Account != nil && (p.Account.Agent == "codex" || p.Account.Agent == "copilot")
}

// plainItemIDs is a Responses request whose input items' ids are of
// [A-Za-z0-9_-] only, as OpenAI takes them, and the rest of the request
// byte for byte. Another vendor's reply can hand the agent an item whose
// id has more in it — Muse Spark on OpenCode's reasoning id is two joined
// with a colon ("rs_6aca…:rs_01a1…") — and the agent sends it back on
// every later turn, after a switch too, where OpenAI and Copilot turn the
// whole request away over it (echo_ts). Such reasoning goes: an upstream
// that checks ids never made it, so its encrypted_content is another
// vendor's, which it couldn't read either. Any other item (a message, a
// call) keeps what it says, its id with each other character as "_"; a
// call's call_id, which its output names it by, stays as it was.
func plainItemIDs(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"id"`)) {
		return body
	}
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return body
	}
	changed := false
	kept := items[:0:0]
	for _, raw := range items {
		var it map[string]json.RawMessage
		var typ, id string
		if json.Unmarshal(raw, &it) != nil || json.Unmarshal(it["id"], &id) != nil || id == "" || safeIDChars.MatchString(id) {
			kept = append(kept, raw)
			continue
		}
		changed = true
		if json.Unmarshal(it["type"], &typ) == nil && typ == "reasoning" {
			continue
		}
		it["id"], _ = json.Marshal(unsafeToolID.ReplaceAllString(id, "_"))
		b, err := marshalPlain(it)
		if err != nil {
			return body
		}
		kept = append(kept, b)
	}
	if !changed {
		return body
	}
	q["input"], _ = marshalPlain(kept)
	nb, err := marshalPlain(q)
	if err != nil {
		return body
	}
	return nb
}

// longCallID finds a call_id longer than the ChatGPT backend takes.
var longCallID = regexp.MustCompile(`"call_id"\s*:\s*"[^"]{65,}"`)

// openaiOnly are the parts of a Responses request Codex sends only to a
// provider named "OpenAI": its built-in one (signed in, through
// openai_base_url), or CC Switch's table so named for remote compaction.
// Under any other name Codex leaves them out itself (client.rs, !is_openai).
var openaiOnly = [][]byte{[]byte(`"internal_chat_message_metadata_passthrough"`),
	[]byte(`"encrypted_function_args"`), []byte(`"stream_options"`), []byte(`"configuration_update"`)}

// forVendor is a Responses request as Codex sends it to a provider not
// OpenAI's: without its messages' internal metadata, a call's
// encrypted_function_args, stream_options' reasoning_summary_delivery and
// configuration_update items.
// A relay that checks it is Codex's turned the lot away ("invalid codex
// request", #292). OpenAI's API and the ChatGPT backend get it as it came;
// so does anything else, byte for byte, when none of them is in it.
func forVendor(p provider.Provider, body []byte) []byte {
	if p.Account != nil && p.Account.Agent == "codex" || strings.HasSuffix(p.Host(), "openai.com") {
		return body
	}
	if !slices.ContainsFunc(openaiOnly, func(k []byte) bool { return bytes.Contains(body, k) }) {
		return body
	}
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return body
	}
	changed := false
	// Codex's stream_options holds reasoning_summary_delivery alone; another
	// client's other options stay
	var so map[string]json.RawMessage
	if json.Unmarshal(q["stream_options"], &so) == nil {
		if _, ok := so["reasoning_summary_delivery"]; ok {
			changed = true
			delete(so, "reasoning_summary_delivery")
			if q["stream_options"], _ = marshalPlain(so); len(so) == 0 {
				delete(q, "stream_options")
			}
		}
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) == nil {
		out := items[:0]
		for _, raw := range items {
			var it map[string]json.RawMessage
			if json.Unmarshal(raw, &it) != nil {
				out = append(out, raw)
				continue
			}
			if string(it["type"]) == `"configuration_update"` {
				changed = true
				continue
			}
			_, meta := it["internal_chat_message_metadata_passthrough"]
			_, sealed := it["encrypted_function_args"]
			if meta || sealed {
				changed = true
				delete(it, "internal_chat_message_metadata_passthrough")
				delete(it, "encrypted_function_args")
				raw, _ = marshalPlain(it)
			}
			out = append(out, raw)
		}
		if changed {
			q["input"], _ = marshalPlain(out)
		}
	}
	if !changed {
		return body
	}
	nb, err := marshalPlain(q)
	if err != nil {
		return body
	}
	return nb
}

func userMessage(text string) map[string]any {
	return map[string]any{"type": "message", "role": "user",
		"content": []map[string]any{{"type": "input_text", "text": text}}}
}

// codexCompact compacts a conversation held by a magpie model: the model
// summarises it, and the summary goes back to Codex as the compaction item
// the backend would have made, marked as magpie's so magpie reads it back
// in later requests.
//
// A provider that answers the summary 404 — a relay that serves the
// conversation turn by turn but not the summary of it, "Upstream request
// failed" (#866) — or 400 for an item of it it doesn't have (#1008) is
// asked once more with the conversation as plain text, none of its items'
// ids or sealed reasoning in it; when that fails too the summary is
// magpie's own, the conversation's user messages and last reply,
// so the compaction still completes and Codex goes on. So is one the
// ChatGPT backend answers 502 "response protection is unavailable" (vs on
// Discord): the same long history failed so on every account and model,
// and as plain text was summarised — and one it breaks off with that
// refusal after the reply began, HTTP 200 and an error event (#1270). Any
// other failure (401, 429, 500) goes back to Codex as it came.
func (s *Server) codexCompact(w http.ResponseWriter, r *http.Request, body []byte) {
	ask := func(b []byte) (*recorder, compactResult, error) {
		rec := &recorder{header: http.Header{}, status: 200}
		s.serve(rec, r, provider.Responses, b)
		if rec.status >= 400 {
			return rec, compactResult{}, nil
		}
		res, err := compactReply(rec.body.Bytes())
		return rec, res, err
	}
	rec, res, err := ask(body)
	local := ""
	if first := rec.status; first == http.StatusNotFound || itemNotFound(rec) || upstreamFailed(rec) || protectionRefused(first, rec.body.Bytes()) ||
		err != nil && protectionWords.MatchString(err.Error()) {
		who, msg := compactFailure(body, rec)
		if err != nil {
			msg = "broken off: " + err.Error()
		}
		log.Printf("codex compaction: %s answered %d (%s); asking again with the conversation as text", who, first, msg)
		rec, res, err = ask(plainCompact(body))
		if rec.status >= 400 || err != nil {
			again := ""
			if err != nil {
				again = "broken off: " + err.Error()
			} else {
				_, again = compactFailure(body, rec)
			}
			log.Printf("codex compaction: %s answered %d again (%s); compacting locally", who, rec.status, again)
			local = localSummary(body, fmt.Sprintf("%s answered %d to the summary request: %s", who, first, msg))
			rec, res, err = &recorder{header: http.Header{}, status: 200}, compactResult{}, nil
		}
	}
	if rec.status >= 400 {
		for k, vs := range rec.header {
			w.Header()[k] = vs
		}
		w.WriteHeader(rec.status)
		w.Write(rec.body.Bytes())
		return
	}
	if err != nil {
		writeError(w, provider.Responses, 502, "compaction: "+err.Error())
		return
	}
	var sum strings.Builder
	if local != "" {
		sum.WriteString(local)
	} else {
		for _, o := range res.Output {
			if o.Type != "message" {
				continue
			}
			for _, c := range o.Content {
				sum.WriteString(c.Text)
			}
		}
	}
	if strings.TrimSpace(sum.String()) == "" {
		writeError(w, provider.Responses, 502, "compaction: the model wrote no summary")
		return
	}
	id := res.ID
	if id == "" {
		id = fmt.Sprintf("resp_magpie_%d", time.Now().UnixNano())
	}
	item := map[string]any{"type": "compaction", "id": "cmp_" + strings.TrimPrefix(id, "resp_"),
		"encrypted_content": magpieCompaction + base64.StdEncoding.EncodeToString([]byte(sum.String()))}
	done := map[string]any{"id": id, "object": "response", "status": "completed", "output": []any{item}}
	if len(res.Usage) > 0 {
		done["usage"] = res.Usage
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	for _, ev := range []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": id, "object": "response", "status": "in_progress", "output": []any{}}},
		{"type": "response.output_item.added", "output_index": 0, "item": item},
		{"type": "response.output_item.done", "output_index": 0, "item": item},
		{"type": "response.completed", "response": done},
	} {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev["type"], b)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// upstreamFailed is a relay's own word that the provider behind it turned
// the request away, without saying why: "flyapi: Upstream request failed"
// came as a 404 (#866), "apex: Upstream request failed" as a 400
// invalid_request_error (#1511). Codex asking the same relay itself
// compacts, so the summary request is asked again as text, and compacted
// without a model when that fails too.
func upstreamFailed(rec *recorder) bool {
	return rec.status >= 400 && rec.status < 500 && rec.status != http.StatusUnauthorized &&
		rec.status != http.StatusForbidden && rec.status != http.StatusTooManyRequests &&
		upstreamFailedWords.Match(rec.body.Bytes())
}

var upstreamFailedWords = regexp.MustCompile(`(?i)upstream request failed`)

// itemNotFound is a 400 refusing an item of the input by its id, as Azure
// OpenAI's and OpenAI's API answer one they never stored ("Item with id
// 'rs_…' not found", #1008), which a relay in front of them passes on.
func itemNotFound(rec *recorder) bool {
	b := rec.body.Bytes()
	return rec.status == http.StatusBadRequest && unreadableItem.Match(b) && bytes.Contains(bytes.ToLower(b), []byte("not found"))
}

// compactFailure names who answered a summary request with a failure — the
// provider, the model and the provider's base URL — and what it said.
func compactFailure(body []byte, rec *recorder) (who, msg string) {
	model := modelOf(body)
	who = cmp.Or(rec.header.Get(providerHeader), strings.SplitN(model, "/", 2)[0]) +
		" (" + cmp.Or(rec.header.Get(modelHeader), model)
	if p, _, ok := provider.Resolve(model); ok {
		if u := cmp.Or(p.Base(provider.Responses), p.Base(provider.Chat), p.Base(provider.Anthropic)); u != "" {
			who += ", " + u
		}
	}
	return who + ")", provider.APIError(rec.body.Bytes(), http.StatusText(rec.status))
}

// compactItems is a summary request and its input's items.
func compactItems(body []byte) (map[string]json.RawMessage, []map[string]any) {
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return nil, nil
	}
	var items []map[string]any
	json.Unmarshal(q["input"], &items)
	return q, items
}

// itemText is what an input item says, as text: a message's text parts, a
// call's name and arguments, a call's output (its middle left out when
// long), reasoning's summary.
func itemText(it map[string]any) (role, text string) {
	str := func(v any) string { s, _ := v.(string); return s }
	parts := func(v any) string {
		if s, ok := v.(string); ok {
			return s
		}
		var ts []string
		ps, _ := v.([]any)
		for _, p := range ps {
			if m, ok := p.(map[string]any); ok && str(m["text"]) != "" {
				ts = append(ts, str(m["text"]))
			}
		}
		return strings.Join(ts, "\n")
	}
	switch str(it["type"]) {
	case "message", "":
		return cmp.Or(str(it["role"]), "user"), parts(it["content"])
	case "function_call", "custom_tool_call":
		return "tool call", str(it["name"]) + " " + cmp.Or(str(it["arguments"]), str(it["input"]))
	case "function_call_output", "custom_tool_call_output":
		out := parts(it["output"])
		if len(out) > 4000 {
			out = out[:2000] + "\n…\n" + out[len(out)-2000:]
		}
		return "tool output", out
	case "reasoning":
		return "reasoning", parts(it["summary"])
	}
	return "", ""
}

// plainCompact is a summary request with the conversation in it as text, in
// one user message, and Codex's request to summarise it after: no item ids,
// sealed reasoning or calls a provider may look up and not find.
func plainCompact(body []byte) []byte {
	q, items := compactItems(body)
	if q == nil {
		return body
	}
	var b strings.Builder
	for _, it := range items {
		role, text := itemText(it)
		if strings.TrimSpace(text) == "" || text == codexCompactPrompt {
			continue
		}
		fmt.Fprintf(&b, "[%s]\n%s\n\n", role, text)
	}
	t := b.String()
	if len(t) > plainCompactMax {
		t = "…\n" + t[len(t)-plainCompactMax:]
	}
	q["input"], _ = json.Marshal([]map[string]any{
		userMessage("This is the conversation so far:\n\n" + t),
		userMessage(codexCompactPrompt),
	})
	delete(q, "previous_response_id")
	nb, err := json.Marshal(q)
	if err != nil {
		return body
	}
	return nb
}

// plainCompactMax bounds the conversation's text a plain summary request
// carries; the oldest of it is left out.
const plainCompactMax = 400_000

// localSummaryMax bounds the user's messages a local summary keeps, the
// latest of them, as Codex's own compaction does.
const localSummaryMax = 80_000

// localSummary is a summary made without a model: what the user asked (the
// summaries before this one with it), the latest of it kept, and the last
// reply.
func localSummary(body []byte, why string) string {
	_, items := compactItems(body)
	var asked []string
	last := ""
	for _, it := range items {
		role, text := itemText(it)
		text = strings.TrimSpace(text)
		switch {
		case text == "" || text == codexCompactPrompt:
		case role == "user":
			asked = append(asked, text)
		case role == "assistant":
			last = text
		}
	}
	from, n := len(asked), 0
	for from > 0 && n+len(asked[from-1]) <= localSummaryMax {
		from--
		n += len(asked[from])
	}
	var b strings.Builder
	b.WriteString("magpie compacted this conversation without a model (" + why + "), so this is no summary: it is what the user asked")
	if from > 0 {
		fmt.Fprintf(&b, " (the latest %d of %d messages)", len(asked)-from, len(asked))
	}
	b.WriteString(" and the last reply. Look at the workspace for where the work stands before going on.\n\n## The user's messages\n")
	for _, a := range asked[from:] {
		b.WriteString("\n" + a + "\n")
	}
	if last != "" {
		b.WriteString("\n## The last reply\n\n" + last + "\n")
	}
	return b.String()
}

type compactOutput struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type compactResult struct {
	ID     string          `json:"id"`
	Output []compactOutput `json:"output"`
	Usage  json.RawMessage `json:"usage"`
	// Status is how the response ended ("completed", "incomplete"), and
	// IncompleteDetails why when it was cut short: max_output_tokens (#1466)
	Status            string          `json:"status"`
	IncompleteDetails json.RawMessage `json:"incomplete_details"`
	Error             *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// compactReply reads the summary's reply, a Responses stream as Codex asks
// for it or a response as a whole: the items from output_item.done (the
// ChatGPT backend's completed response lists none), usage from completed.
func compactReply(b []byte) (compactResult, error) {
	var res compactResult
	t := bytes.TrimSpace(b)
	if len(t) > 0 && t[0] == '{' {
		err := json.Unmarshal(t, &res)
		return res, err
	}
	var items []compactOutput
	var failed string
	readSSE(bytes.NewReader(b), func(_, data string) error {
		var ev struct {
			Type     string         `json:"type"`
			Item     compactOutput  `json:"item"`
			Response *compactResult `json:"response"`
			Message  string         `json:"message"` // an error event's
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"` // or nested in its error, as the ChatGPT backend's (#1270)
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return nil
		}
		if ev.Response != nil && ev.Response.ID != "" {
			res.ID = ev.Response.ID
		}
		switch ev.Type {
		case "response.output_item.done":
			items = append(items, ev.Item)
		case "response.completed", "response.incomplete":
			if ev.Response != nil {
				res.Output, res.Usage = ev.Response.Output, ev.Response.Usage
				res.Status, res.IncompleteDetails = ev.Response.Status, ev.Response.IncompleteDetails
			}
			if res.Status == "" {
				res.Status = strings.TrimPrefix(ev.Type, "response.")
			}
		case "response.failed", "error":
			failed = ev.Message
			if failed == "" && ev.Error != nil {
				failed = ev.Error.Message
			}
			failed = cmp.Or(failed, "the model failed")
			if ev.Response != nil && ev.Response.Error != nil && ev.Response.Error.Message != "" {
				failed = ev.Response.Error.Message
			}
		}
		return nil
	})
	if failed != "" {
		return res, errors.New(failed)
	}
	if len(items) > 0 {
		res.Output = items
	}
	return res, nil
}

// recorder keeps a reply for a second look.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) WriteHeader(status int)      { r.status = status }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
