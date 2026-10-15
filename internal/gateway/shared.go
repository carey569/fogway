package gateway

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/carey569/fogway/internal/codexcat"
	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/usage"
	"github.com/tidwall/gjson"
)

// Helpers the request paths share that lived beside the subscriptions
// Fogway no longer serves.

// withoutBillingHeader is a system prompt without the billing line Claude
// Code puts first in its own (x-anthropic-billing-header: cc_version=…),
// whose last part is a hash of the request's first user message. Its auto
// mode classifier sends its policy, about 35k tokens, with the transcript
// so far as that message: kept, every check's instructions began with
// another line, so none was read from the cache and each check wrote all of
// it again at twice the input price, on the session's model (X, AncientTwo).
// The run's Claude Code sends its own.
func withoutBillingHeader(system string) string {
	return billingHeader.ReplaceAllString(system, "")
}

// billingHeader is the line with its fields (cc_version=…; cc_entrypoint=…;
// cch=…;), which the block after follows with no newline between.
var billingHeader = regexp.MustCompile(`^x-anthropic-billing-header:(\s*[A-Za-z_]+=[^;\s]*;)*\s*`)

// sealedTaskError is what a subagent is told whose task its lead sealed
// when nothing model names can read it. lead is the provider that
// answered the lead, "" when fogway didn't (the lead was one of Codex's
// own models) or has no record of it in the last sealerKeep. A task
// already sealed opens only where it was sealed, so the way on with it
// is said too (#1367). earlier is a provider that answered the lead's
// earlier turns but can't have sealed the task (canSeal): the lead moved
// on to Codex's own model, whose turns fogway keeps no record of, and the
// task was sealed there (plugins#70). Each says what is sealed: the task
// itself, which fork_turns="none" doesn't change (#1855).
func sealedTaskError(model, lead, earlier string) string {
	if earlier != "" {
		return fmt.Sprintf("This subagent's task was sealed by the ChatGPT backend, which answered the agent that sent it on one of Codex's own models (a subagent of this thread put there, or the lead after a switch), and only a ChatGPT account can open it. %s can't. %s answered the lead's earlier turns, but it neither seals a task nor opens one. "+sealedTaskItself+" Set Codex's subagent model on fogway's Agents page to a model of a ChatGPT account added in fogway, or to one that isn't served by the ChatGPT backend, so subagents get tasks they can read. To go on with this task, have it spawned again from an agent that isn't on Codex's own models.", model, earlier)
	}
	if lead != "" {
		return fmt.Sprintf("This subagent's task was sealed by the server that answered its lead (%s), and only that server can open it; %s can't. "+sealedTaskItself+" Give the subagent the lead's model, or a lead model that isn't served by the ChatGPT backend, so its subagents get a task they can read. To go on with this task, use a model on %s for the lead and the subagent, or have the lead spawn the subagent again.", lead, model, lead)
	}
	return fmt.Sprintf("This subagent's task was sealed by the ChatGPT backend that answered its lead, and only that backend can open it: a ChatGPT account, or the Responses API provider that answered the lead. %s is neither: fogway has no record of it answering the lead in the last 30 days. "+sealedTaskItself+" Give the subagent the lead's model, or a lead model that isn't served by the ChatGPT backend, so its subagents get a task they can read. To go on with this task, use the lead's model, or have the lead spawn the subagent again.", model)
}

// canSeal says whether the provider id names could have sealed a
// subagent's task, as sealedReader has who can open one: a ChatGPT
// account, or a provider of no sign-in serving the Responses API (a relay
// of the ChatGPT backend, #1109). One fogway can't find is taken as one
// that could.
func canSeal(id string) bool {
	p, err := provider.Find(id)
	if err != nil || p == nil {
		return true
	}

	return slices.Contains(p.Speaks(), provider.Responses)
}

// leadProvider is the provider that answered a subagent's lead, the
// thread parent names, in any scope fogway routed it in: "" when it
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
		note := "[fogway: this message from " + from + " was sealed by the ChatGPT backend, which answered that agent on one of Codex's own models, and only it can open it; this model can't, so it was left out. An agent's final result still reaches the agent that spawned it as text.]"
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

// callKind is what an agent made a call for when it isn't a turn of the
// conversation, as Codex names it in x-openai-subagent: "guardian" (auto
// review of an approval), "review", "compact", "memory_consolidation",
// "thread_title", "collab_spawn"… A turn Codex sends on Luna Reserve, once
// the plan's own allowance is used up, is "luna_reserve". A call Codex
// makes on a hidden thread of its own goes without x-openai-subagent: its
// x-codex-turn-metadata names the thread's source instead — "thread_title"
// for the title of a new chat (#314), "guardian_review". A web search
// fogway runs for a model that can't search is "web_search".
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

// codexInput restores summaries fogway made. For a fogway model, it also
// replaces Codex's compaction trigger with a request to summarise the input.
// OpenAI's own models keep the trigger for their backend to handle.
func codexInput(body []byte, fogwayModel bool) (_ []byte, compact bool) {
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
			sum, ok := strings.CutPrefix(enc, fogwayCompaction)
			if !ok {
				out = append(out, it) // OpenAI's own, for OpenAI
				continue
			}
			changed = true
			if b, err := base64.StdEncoding.DecodeString(sum); err == nil {
				out = append(out, userMessage(codexSummaryPrefix+"\n"+string(b)))
			}
		case "compaction_trigger":
			if !fogwayModel {
				out = append(out, it)
				continue
			}
			changed, compact = true, true
			out = append(out, userMessage(codexCompactPrompt))
		case "reasoning":
			// OpenAI accepts no reasoning content parts in replayed input.
			// Drop the item so its encrypted_content cannot fail there too.
			if !fogwayModel {
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
// summary only, as fogway gives Codex in a translated reply (rs_ and
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
// a vendor's reply may have handed Codex with another: fogway, or the
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
	return strings.HasSuffix(p.Host(), "openai.com") || p.IsAzure()

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

// forVendor is a Responses request as Codex sends it to a provider not
// OpenAI's: without its messages' internal metadata, a call's
// encrypted_function_args, stream_options' reasoning_summary_delivery and
// configuration_update items.
// A relay that checks it is Codex's turned the lot away ("invalid codex
// request", #292). OpenAI's API and the ChatGPT backend get it as it came;
// so does anything else, byte for byte, when none of them is in it.
func forVendor(p provider.Provider, body []byte) []byte {
	if strings.HasSuffix(p.Host(), "openai.com") {
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

// jevLevels are the efforts the classifier chooses among, with what each
// is for, lowest first (named for Jev, which asked them first).
var jevLevels = []struct{ effort, what string }{
	{"low", "Little to think about: a greeting, a question answered from what is already known, a mechanical or one-line change, running a command"},
	{"medium", "Some thought: an ordinary bug fix or a small feature in code already understood"},
	{"high", "Careful thought: a change across several files, a bug whose cause is not known yet, a design choice with trade-offs"},
	{"xhigh", "Deep thought: a subtle bug (concurrency, performance, security), an architecture or algorithm to design, a long multi-step plan"},
}

// withEffort asks a request, in the client's own API, for reasoning at
// effort, in place of what the agent asked. Only a request that asked for
// reasoning is changed: one that didn't (a session title) stays without.
func withEffort(proto provider.Protocol, body []byte, effort string) []byte {
	var v struct {
		ReasoningEffort string `json:"reasoning_effort"`
		Reasoning       *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		Thinking *struct {
			Type string `json:"type"`
		} `json:"thinking"`
		OutputConfig map[string]any `json:"output_config"`
		MaxTokens    int            `json:"max_tokens"`
	}
	if effort == "" || decodeJSON(body, &v) != nil {
		return body
	}
	switch proto {
	case provider.Chat:
		if v.ReasoningEffort == "" || v.ReasoningEffort == "none" {
			return body
		}
		return withFields(body, map[string]any{"reasoning_effort": effort})
	case provider.Responses:
		if v.Reasoning == nil || v.Reasoning.Effort == "" || v.Reasoning.Effort == "none" {
			return body
		}
		// the effort alone: the rest of reasoning goes as the agent sent
		// it — Codex's Responses Lite asks for context "all_turns", which
		// the ChatGPT backend wants with its X-OpenAI-Internal-Codex-
		// Responses-Lite header ("requires `reasoning.context` to be
		// `all_turns`", #534)
		return withBodyEffort(proto, body, effort)
	case provider.Anthropic:
		if v.Thinking == nil {
			return body
		}
		switch v.Thinking.Type {
		case "adaptive":
			oc := v.OutputConfig
			if oc == nil {
				oc = map[string]any{}
			}
			oc["effort"] = effort
			return withFields(body, map[string]any{"output_config": oc})
		case "enabled":
			// the budget must leave room for the answer
			budget := budgetOf(effort)
			if v.MaxTokens > 0 {
				budget = min(budget, v.MaxTokens-1)
			}
			if budget < 1024 {
				return body
			}
			fields := map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": budget}}
			if oc := v.OutputConfig; oc != nil && oc["effort"] != nil {
				oc["effort"] = effort
				fields["output_config"] = oc
			}
			return withFields(body, fields)
		}
	}
	return body
}

// requestEffort is the reasoning a request asks its model for, in its own
// API's words: reasoning_effort, reasoning.effort, or — for Anthropic, when
// it asks to think — output_config's effort or thinking's budget as the
// level it is nearest. "" when it asks for none.
func requestEffort(proto provider.Protocol, body []byte) string {
	if proto != provider.Anthropic {
		return bodyEffort(proto, body)
	}
	var v struct {
		Thinking *struct {
			Type   string `json:"type"`
			Budget int    `json:"budget_tokens"`
		} `json:"thinking"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	if decodeJSON(body, &v) != nil || v.Thinking == nil || v.Thinking.Type != "enabled" && v.Thinking.Type != "adaptive" {
		return ""
	}
	if e := effortOf(v.OutputConfig.Effort); e != "" {
		return e
	}
	return effortOfBudget(v.Thinking.Budget)
}

// sentEffort is the reasoning a request goes to model at: what it asks
// for, fitted to the model's levels as it is fitted on the way out.
func sentEffort(proto provider.Protocol, body []byte, p provider.Provider, model string) string {
	e := requestEffort(proto, body)
	if e == "" {
		return ""
	}
	return fitFor(p, model, e)
}

// fitFor is the effort a request asking for want goes to model at, at p:
// the level of its own nearest (fitEffort), and for a model known to have
// none to pick from — a thinking switch alone, as Xiaomi's mimo-v2.6-flash
// has — no more than high, which it takes where it turns max away (#214).
// A model whose levels just aren't known is asked as the agent asked.
func fitFor(p provider.Provider, model, want string) string {
	levels := p.Efforts(model)
	e := fitEffort(want, levels)
	if len(levels) == 0 && slices.Index(effortRank, e) > slices.Index(effortRank, "high") && p.Levelless(model) {
		return "high"
	}
	return e
}

// fitLevel is the level of the model's own nearest the one picked for it
// (fitEffort); one whose levels aren't known isn't asked for more than
// high, which every vendor with levels takes.
func fitLevel(want string, levels []string) string {
	level := fitEffort(want, levels)
	if len(levels) == 0 && level == "xhigh" {
		level = "high"
	}
	return level
}

// withFixedEffort asks a request, in the client's own API, for reasoning
// at effort — that of a group's member fixed at it (#189) — whatever the
// agent asked: unlike withEffort, a request that asked for none is asked
// for it too, and "none" turns reasoning off.
func withFixedEffort(proto provider.Protocol, body []byte, effort string) []byte {
	if effort == "" {
		return body
	}
	switch proto {
	case provider.Chat:
		return withFields(body, map[string]any{"reasoning_effort": effort})
	case provider.Responses:
		var v struct {
			Reasoning map[string]any `json:"reasoning"`
		}
		if decodeJSON(body, &v) != nil {
			return body
		}
		if v.Reasoning == nil {
			v.Reasoning = map[string]any{}
		}
		v.Reasoning["effort"] = effort
		return withFields(body, map[string]any{"reasoning": v.Reasoning})
	case provider.Anthropic:
		var v struct {
			Thinking *struct {
				Type string `json:"type"`
			} `json:"thinking"`
			MaxTokens int `json:"max_tokens"`
		}
		if decodeJSON(body, &v) != nil {
			return body
		}
		if effort == "none" {
			return withFields(body, map[string]any{"thinking": map[string]any{"type": "disabled"}})
		}
		level := effortOf(effort) // in Anthropic's words: minimal is low
		if v.Thinking != nil && (v.Thinking.Type == "enabled" || v.Thinking.Type == "adaptive") {
			return withEffort(proto, body, level)
		}
		// asked to think, with room left for the answer
		budget := budgetOf(level)
		if v.MaxTokens > 0 {
			budget = min(budget, v.MaxTokens-1)
		}
		if budget < 1024 {
			return body
		}
		return withFields(body, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": budget}})
	}
	return body
}

// groupLevels are the reasoning levels the group offers for the member a
// candidate is of, where the user named them (Group.Levels, #295): the
// group's own, else those of the outermost group in it that names some.
// nil when none is named. They bound what the candidate is asked for
// (withinLevels): the agent, a model's suffix or the turn's pick asking
// for a level the group doesn't offer is sent the nearest it does (#671).
func groupLevels(g provider.Group, ms []provider.Member, c candidate) []string {
	if len(g.Levels) > 0 {
		return g.Levels
	}
	for _, m := range ms {
		if m.Provider.ID != c.p.ID || m.Model != c.model || m.Effort != c.effort {
			continue
		}
		if i := slices.IndexFunc(m.Via, func(v provider.Group) bool { return len(v.Levels) > 0 }); i >= 0 {
			return m.Via[i].Levels
		}
	}
	return nil
}

// withinLevels is want, when levels offer it, else the level of levels
// nearest it (fitEffort). Codex's ultra is max with agents of its own, so
// a group that offers max offers it too.
func withinLevels(want string, levels []string) string {
	if want == "" || len(levels) == 0 || slices.Contains(levels, want) || want == "ultra" && slices.Contains(levels, "max") {
		return want
	}
	return fitEffort(want, levels)
}

// agentEffort is the reasoning level a request asks for in its own API's
// words: Chat's and Responses' as the agent named it (minimal and none
// kept), Anthropic's as requestEffort reads it. "" when it names none.
func agentEffort(proto provider.Protocol, body []byte) string {
	if proto == provider.Anthropic {
		return requestEffort(proto, body)
	}
	return strings.ToLower(strings.TrimSpace(bodyEffort(proto, body)))
}

func vendorSaid(text string) string {
	if text = strings.TrimSpace(text); text == "" {
		return ""
	}
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return ": " + text
}

// vendorMessage is the message of a vendor's error, or its body.
func vendorMessage(b []byte) string {
	var e struct {
		Error  json.RawMessage `json:"error"`
		Errors struct {
			Message string `json:"message"`
		} `json:"errors"` // ModelScope's
		Code json.RawMessage `json:"code"` // WorkBuddy's {code, msg}
		Msg  string          `json:"msg"`
	}
	if json.Unmarshal(b, &e) == nil && e.Errors.Message != "" {
		return e.Errors.Message
	}
	if json.Unmarshal(b, &e) == nil && len(e.Error) == 0 && e.Msg != "" {
		if code := strings.Trim(string(e.Code), `"`); code != "" && code != "0" && code != "null" {
			return e.Msg + " (" + code + ")"
		}
	}
	if json.Unmarshal(b, &e) == nil && len(e.Error) > 0 {
		var m struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Error, &m) == nil && m.Message != "" {
			return m.Message
		}
		var s string
		if json.Unmarshal(e.Error, &s) == nil {
			return s
		}
	}
	return string(b)
}

// caller is who a request is recorded as: its agent, and the computer it
// was passed on from, "" for this one. session and native are the session
// the caller named (sessionOf's X-Fogway-Session, and the agent's own
// header nativeSessionOf takes, by its name), passed on to a remote fogway
// with the request (#672).
type caller struct {
	agent, via string
	session    string
	native     [2]string // header, value
	// ua is the User-Agent the request came with, as it came: the one a
	// provider that passes the agent's on is sent (userAgentFor)
	ua string
}

type callerCtx struct{}

// withCaller notes on each request who it is recorded as, and takes the
// headers that said so off it.
func withCaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := caller{agent: agentOf(r), session: boundedSession(r.Header.Get(SessionHeader)), ua: r.Header.Get("User-Agent")}
		for _, h := range sessionHeaders {
			if v := boundedSession(r.Header.Get(h)); v != "" {
				c.native = [2]string{h, v}
				break
			}
		}
		if strings.HasPrefix(r.Header.Get("User-Agent"), "fogway/") {
			if a := label(r.Header.Get(AgentHeader)); a != "" {
				c.agent = usage.AgentOf(a)
				c.via = label(r.Header.Get(ViaHeader))
				if c.via == "" {
					c.via, _, _ = net.SplitHostPort(r.RemoteAddr)
				}
			}
		}
		ctx := context.WithValue(r.Context(), callerCtx{}, c)
		if searchingFrom(r) {
			// another fogway's search: only the model itself searches
			ctx = context.WithValue(ctx, searchingKey{}, true)
		}
		r.Header.Del(AgentHeader)
		r.Header.Del(ViaHeader)
		r.Header.Del(SearchingHeader)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// callerOf is who the request is recorded as.
func callerOf(r *http.Request) caller {
	if c, ok := r.Context().Value(callerCtx{}).(caller); ok {
		return c
	}
	return caller{agent: agentOf(r)}
}

// relay answers the client with a reply's events: abort is called when
// there is no answer to give, ended when one was given, before its last
// event goes out.
func relay(w http.ResponseWriter, r *http.Request, from provider.Protocol, name string, req *Request, events <-chan Event, usage *Usage,
	abort, begin func(), ended func(said, stop string, ok bool)) (int, string) {
	stream := req.Stream
	if stream {
		// an error before any of the answer — out of quota, rate limited —
		// is a status, not a stream, so another account can take over
		var head []Event
		for ev := range events {
			head = append(head, ev)
			if ev.Kind != KStart && ev.Kind != KUsage {
				break
			}
		}
		if n := len(head); n == 0 || head[n-1].Kind == KError {
			msg := name + " ended without an answer"
			if n > 0 {
				msg = head[n-1].Text
			}
			abort()
			code := 502
			if n > 0 {
				usage.add(Usage{ErrType: head[n-1].Code, RequestID: head[n-1].RequestID})
				// the status Claude Code got, else a guess from the words
				if head[n-1].Status >= 400 {
					code = head[n-1].Status
				} else if quotaSaid(msg) {
					code = 429
				}
			}
			return writeError(w, from, code, name+": "+msg), msg
		}
		if begin != nil {
			begin()
		}
		sw := newSSEWriter(w)
		enc := encoder(from, sw, req, usage)
		var failed, said, stop string
		see := func(ev Event) {
			switch ev.Kind {
			case KError:
				failed = ev.Text
			case KStart, KUsage:
				usage.add(ev.Usage)
				usage.add(Usage{Served: ev.Model, RequestID: ev.RequestID})
			case KText:
				said += ev.Text
			case KStop:
				stop = ev.Stop
			}
			enc.event(ev)
		}
		for _, ev := range head {
			see(ev)
		}
		// a client waiting on a reply that goes on is kept from its idle
		// timeout while none of it comes (#436)
		relayEvents(events, sw, enc, func(ev Event) bool {
			see(ev)
			return true
		})
		// before the reply's last event, which the agent may answer at once
		ended(said, stop, failed == "" && r.Context().Err() == nil)
		// an error event already ended the reply in the client's protocol
		if failed == "" {
			enc.finish()
		}
		return 200, failed
	}
	var col collector
	var said, stop string
	for ev := range events {
		switch ev.Kind {
		case KText:
			said += ev.Text
		case KStop:
			stop = ev.Stop
		}
		col.add(ev)
	}
	if col.err != "" && !saidAnything(col.res.Parts) {
		abort()
		// a status, as in a stream, so another account can take over
		code := 502
		usage.add(Usage{ErrType: col.errCode})
		if col.errStatus >= 400 {
			code = col.errStatus
		} else if quotaSaid(col.err) {
			code = 429
		}
		return writeError(w, from, code, name+": "+col.err), col.err
	}
	ended(said, stop, col.err == "" && r.Context().Err() == nil)
	if begin != nil {
		begin()
	}
	res := col.finish()
	usage.add(res.Usage)
	usage.add(Usage{Served: res.Model})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(renderUsage(from, res, req, usage))
	return 200, col.err
}

// fogwayCompaction marks a compaction item fogway made: its summary, which
// only fogway reads back.
const fogwayCompaction = "fogway1:"

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

// sealedTaskItself says what a sealed task is (#1855): spawn_agent's
// message, which the backend that answered the lead returns sealed (its
// function call's encrypted_function_args), and which Codex hands the
// subagent as it came, as its NEW_TASK (codex-rs multi_agents_v2.rs
// agent_message_from_tool). fork_turns="none" leaves only the lead's
// history out of the subagent's first request; the task is still sealed.
const sealedTaskItself = `What is sealed is the task itself: the message the agent that spawned this subagent wrote in spawn_agent, which Codex hands on as the backend sealed it. fork_turns="none" keeps the lead's earlier turns out of the subagent, but not the seal on its task, so it doesn't help here.`

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
		if r.Provider == "" || r.Provider == "fogway" || r.IsRejected() || r.IsReported() || r.Status <= 0 || r.Status >= 400 || r.Computer != "" || isTitleKind(r.Kind) {
			return
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

// reasoningContentRefusal is OpenAI's refusal of a reasoning item with its
// text in it, naming the item: "Invalid 'input[1].content': array too
// long. Expected an array with maximum length 0, but got an array with
// length 1 instead." (the ChatGPT backend's; a relay's may word the rest
// otherwise, #1411)
var reasoningContentRefusal = regexp.MustCompile(`input\[(\d+)\]\.content'?:? array too long`)

// openaiOnly are the parts of a Responses request Codex sends only to a
// provider named "OpenAI": its built-in one (signed in, through
// openai_base_url), or CC Switch's table so named for remote compaction.
// Under any other name Codex leaves them out itself (client.rs, !is_openai).
var openaiOnly = [][]byte{[]byte(`"internal_chat_message_metadata_passthrough"`),
	[]byte(`"encrypted_function_args"`), []byte(`"stream_options"`), []byte(`"configuration_update"`)}

func userMessage(text string) map[string]any {
	return map[string]any{"type": "message", "role": "user",
		"content": []map[string]any{{"type": "input_text", "text": text}}}
}

type compactOutput struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// codexTiers are the service tiers Codex here is offered on each of shown,
// by id: Fast on a ChatGPT account's GPT model, the tiers of Codex's own
// entry on a relay's. Another fogway that has this one as its provider
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

// askedTier is the service tier a Responses request asks for.
func askedTier(body []byte) string {
	var q struct {
		Tier string `json:"service_tier"`
	}
	decodeJSON(body, &q)
	return q.Tier
}

const AgentHeader = "X-Fogway-Agent"

const ViaHeader = "X-Fogway-Via"

// label is a header's name for an agent or a computer, if it is a fit one
// to show: short, one word of printable characters.
func label(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 64 || strings.IndexFunc(v, func(r rune) bool { return !unicode.IsPrint(r) || unicode.IsSpace(r) }) >= 0 {
		return ""
	}
	return v
}

// boundedSession is a session header's value as fogway keeps it: trimmed,
// at most 128 bytes, as sessionOf takes it.
func boundedSession(v string) string {
	v = strings.TrimSpace(v)
	return v[:min(len(v), 128)]
}

// SearchingHeader marks a request a fogway made to search for another
// model, to a remote fogway, which then searches only by the model itself.
const SearchingHeader = "X-Fogway-Searching"

// searchingFrom is whether a request is a search another fogway made.
func searchingFrom(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("User-Agent"), "fogway/") && r.Header.Get(SearchingHeader) != ""
}
func (u Usage) plus(v Usage, only bool) Usage {
	for _, f := range []struct{ a, b *int }{{&u.Input, &v.Input}, {&u.Output, &v.Output}, {&u.CacheRead, &v.CacheRead}, {&u.CacheWrite, &v.CacheWrite}, {&u.CacheWrite1h, &v.CacheWrite1h}, {&u.Reasoning, &v.Reasoning}} {
		if *f.a > 0 || !only {
			*f.a += *f.b
		}
	}
	return u
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(status int) { r.status = status }

func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }

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

	return slices.Contains(sealers, p.ID) && slices.Contains(s.usable(p, model), provider.Responses)
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
	return cands, pl
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

// CodexPath is where Codex's built-in OpenAI provider reaches fogway, as its
// `openai_base_url`. Codex keeps its own sign-in and sends what it would send
// the ChatGPT backend: a request for one of its own models goes on there as
// it came, one for a fogway model (a catalog id, provider/model) is served
// like any other, and the model list is OpenAI's with fogway's added. Ending
// in /backend-api/codex keeps Codex treating it as that backend.
const CodexPath = "/backend-api/codex"
