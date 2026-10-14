package gateway

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/codexcat"
	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/usage"
)

// codexAPIBase is where a Codex signed in with an API key sends its requests;
// its model list still comes from the ChatGPT backend. A var so tests can
// point it elsewhere.
var codexAPIBase = "https://api.openai.com/v1"

// codexChatGPTBase is the ChatGPT backend a Codex signed in to ChatGPT sends
// its own models' turns to. A var so tests can point it elsewhere.
var codexChatGPTBase = "https://chatgpt.com/backend-api/codex"

// codexModelsWait is how long the ChatGPT backend is given for Codex's own
// model list before the one Codex had last stands in: Codex waits 5 s.
var codexModelsWait = 3 * time.Second

// codexBackend answers CodexPath, where Codex reaches the gateway as its
// openai_base_url, and as the base URL of the [model_providers] table the
// gateway is when Codex isn't signed in to ChatGPT. A fogway model
// (provider/model, group/…) is served like any other request; one of
// Codex's own models goes on as it came, with Codex's own sign-in
// (codexUpstream); the model list is Codex's own with fogway's after it.
func (s *Server) codexBackend(w http.ResponseWriter, r *http.Request) {
	// Responses over a WebSocket: 426 sends Codex to plain HTTP at once
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "fogway speaks HTTP", http.StatusUpgradeRequired)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, CodexPath)
	body, ok := s.readRequestBody(w, r, provider.Responses, 0)
	if !ok {
		return
	}
	if r.Method == http.MethodGet && rest == "/models" {
		s.codexModels(w, r)
		return
	}
	if r.Method == http.MethodPost && (rest == "/responses" || rest == "/responses/compact") {
		body, model, err := requestModel(body)
		if err != nil {
			writeError(w, provider.Responses, 400, err.Error())
			return
		}
		// a thread's title, where Settings sends it (#705) — the request
		// still on Codex's own model, through its sign-in, when it says
		// nothing of them
		if rest == "/responses" {
			if to := codexTitlesTo(r.Header, body, false); to != "" {
				s.codexTitle(w, r, body, to)
				return
			}
			if to := codexDescriptionsTo(r.Header, body, false); to != "" {
				s.codexDescription(w, r, body, to)
				return
			}
			if to := codexPredictionsTo(r.Header, body, false); to != "" {
				s.codexPrediction(w, r, body, to)
				return
			}
			// a subagent on one of Codex's own models, put on the model
			// set for Codex's subagents; the request on the model asked
			// for when it can't be, the trace saying why
			if !strings.Contains(model, "/") {
				pick := codexSubagentPick(r, agentOf(r), requestCallKind(r.Header, requestSessionMetadata(r.Header, body)), model, body)
				r = withSubagentPick(r, pick)
				if pick.Moved() {
					body, model = withModel(body, pick.To), pick.To
				}
			}
			// spawn_agent names every model a subagent may be put on,
			// not only Codex's first five (Gearclickist on Discord)
			body = spawnAgentModels(r, body, model, true)
		}
		// one of Codex's own models the user disabled (#1867) is refused
		// here, never relayed; a fogway model is refused by serve
		if !strings.Contains(model, "/") {
			if msg, off := codexNativeOff(model); off {
				writeError(w, provider.Responses, http.StatusForbidden, msg)
				return
			}
		}
		// The namespace owns the route even if a model is not in the catalog.
		// Unknown providers/groups must fail locally, never fall through to OpenAI.
		if strings.Contains(model, "/") {
			if rest == "/responses/compact" {
				writeError(w, provider.Responses, 400, "/responses/compact is not supported for Fogway models; use a compaction_trigger on /responses")
				return
			}
			body, compact := codexInput(body, true)
			if compact {
				s.codexCompact(w, r, body)
				return
			}
			s.serveAgent(w, r, provider.Responses, body)
			return
		}
		// the ChatGPT backend checks every item id as OpenAI does
		body = plainItemIDs(boundCallIDs(callItemIDs(body)))
		if rest == "/responses" {
			body, _ = codexInput(body, false)
			withGatewaySession(w, r)
			var recorded func()
			w, recorded = recordConversation(w, r, provider.Responses, body)
			defer recorded()
		}
		s.codexUpstream(w, r, rest, body)
		return
	}
	s.codexUpstream(w, r, rest, body)
}

// codexModels is Codex's own model list for its sign-in, from the ChatGPT
// backend, with fogway's models after it. Should the backend not answer, or
// not in time, Codex's last list of its own stands in.
func (s *Server) codexModels(w http.ResponseWriter, r *http.Request) {
	var own []any
	etag := ""
	u := codexChatGPTBase + "/models"
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	ctx, cancel := context.WithTimeout(r.Context(), codexModelsWait)
	defer cancel()
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil); err == nil && r.Header.Get("Authorization") != "" {
		copyHeaders(req.Header, r.Header)
		req.Header.Del("Accept-Encoding")
		if res, err := s.client.Do(req); err == nil {
			var list struct {
				Models []any `json:"models"`
			}
			b, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
			res.Body.Close()
			if res.StatusCode < 300 && json.Unmarshal(b, &list) == nil {
				own, etag = list.Models, res.Header.Get("ETag")
				// the versions as the backend says them, before any is
				// stamped below (see codexcat.V1)
				codexcat.Remember(own)
			} else if res.StatusCode == 401 || res.StatusCode == 403 {
				// a sign-in to renew is Codex's to see
				w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
				w.WriteHeader(res.StatusCode)
				w.Write(b)
				return
			}
		}
	}
	defer provider.Hold()()
	if own == nil {
		for _, e := range codexcat.CacheEntries() {
			own = append(own, e)
		}
	}
	// the ones the user disabled (#1867), which are refused (codexNativeOff)
	if len(provider.DisabledModels()) > 0 {
		own = slices.DeleteFunc(own, func(m any) bool {
			o, _ := m.(map[string]any)
			slug, _ := o["slug"].(string)
			_, off := codexNativeOff(slug)
			return slug != "" && off
		})
	}
	for _, m := range own {
		if o, ok := m.(map[string]any); ok {
			// multi-agent V1 when the user asked for it (#141): the
			// version is all that changes
			if codexcat.V1() {
				codexcat.Stamp(o)
			}
			// and the auto-review model the user picked (#938)
			codexcat.AutoReview(o)
		}
	}
	// the list is Codex's and fogway's, and so is its ETag
	w.Header().Set("ETag", codexcat.WithTag(etag, provider.CodexListTag()))
	all := append(own, codexcat.Entries(provider.CodexListed(), len(own)+100)...)
	writeJSON(w, 200, map[string]any{"models": all})
}

// modelsEtag is the X-Models-Etag of a backend reply as Codex should read
// it: with fogway's models in it, as the list's own ETag has them, so a
// change to either has Codex ask for the list again.
func modelsEtag(h http.Header) {
	if v := h.Get("X-Models-Etag"); v != "" {
		h.Set("X-Models-Etag", codexcat.WithTag(v, provider.CodexListTag()))
	}
}

// codexUpstream relays a request for one of Codex's own models as it came,
// the sign-in included, to where Codex would have sent it: the ChatGPT
// backend for a Codex signed in to ChatGPT, OpenAI's API for one signed in
// with a key. The gateway keeps no ChatGPT account of its own: this is
// Codex's own sign-in, passed through.
func (s *Server) codexUpstream(w http.ResponseWriter, r *http.Request, rest string, body []byte) {
	start := time.Now()
	usage.Saw(agentOf(r))
	if r.Method == http.MethodPost {
		var unmask func()
		w, body, unmask = redacted(w, body)
		defer unmask()
	}
	base := codexChatGPTBase
	if apiKey(r.Header) && rest != "/models" {
		base = codexAPIBase
	}
	u := base + rest
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	// a turn goes in the Routing view's live trace as the others do, to
	// the one it can go to: Codex's own sign-in, or its key
	var tr *Route
	first := firstToken{start: start} // the reply's first tokens (#196), counted as ms are
	served := ""                      // the model the reply says answered
	var uu Usage
	metadata := requestSessionMetadata(r.Header, body)
	kind := requestCallKind(r.Header, metadata)
	var titleReply capturedBody // bounded; ordinary streams incur no copy
	captureTitle := false
	effort := "" // the reasoning Codex asked for, which goes on as it is
	end := func(status int, msg string, tokens, out int) {}
	var tools *toolScan // whether the reply calls a tool, for a turn's end
	if rest == "/responses" {
		who := "Codex's own sign-in"
		if base == codexAPIBase {
			who = "Codex's API key"
		}
		model := modelOf(body)
		effort = requestEffort(provider.Responses, body)
		seat := Weighed{ID: "codex", Provider: "openai", Name: "OpenAI", Icon: "openai", Who: who, Kind: "account", Agent: "codex", Model: model}
		link := s.titlePrompts.observe(r, body, metadata, kind, start)
		captureTitle = link != nil && isTitleKind(kind)
		turn, scan := agentTurnOf(r, kind)
		tools = scan
		tr = s.trace.beginFor(r, Route{turn: turn, dirOf: dirOf(provider.Responses, body), tools: tools, TitleLink: link, Time: start, Agent: agentOf(r), Session: sessionOf(r.Header), Conv: convOf(r.Header, body), ParentSession: titleParentSession(r.Header, metadata, kind), Kind: kind, Model: model, Effort: effort, Tier: bodyTier(provider.Responses, body), Provider: "openai", Subagent: subagentPickOf(r),
			Order: []Weighed{seat}, Tries: []Try{{ID: seat.ID, Model: model, Effort: effort, Start: start}}})
		promptRead := s.inspectPrompt(tr, provider.Responses, body, body)
		end = func(status int, msg string, tokens, out int) {
			ms := time.Since(start).Milliseconds()
			ttft, text := first.ms()
			replyDigest := ""
			if captureTitle && status < 400 && msg == "" && !titleReply.truncated {
				replyDigest = titleReplyDigest(titleReply.buf.Bytes(), nil)
			}
			prompt := promptRead() // outside the trace's lock, which reading it takes
			s.trace.update(tr, func(t *Route) {
				t.Tries[0].Done, t.Tries[0].Status, t.Tries[0].Millis, t.Tries[0].Error = true, status, ms, msg
				t.Tries[0].TTFT, t.Tries[0].FirstText = ttft, text
				t.ReasoningHidden = first.hidesReasoning(provider.Responses)
				t.Tries[0].ReasoningHidden = t.ReasoningHidden
				t.Done, t.Status, t.Error, t.Millis, t.Tokens = true, status, msg, ms, tokens
				if t.TitleLink != nil && isTitleKind(kind) && status < 400 && msg == "" && !titleReply.truncated {
					link := *t.TitleLink
					link.Reply = replyDigest
					t.TitleLink = &link
				}
				t.Output, t.Reasoning, t.TTFT, t.FirstText = out, uu.Reasoning, ttft, text
				flow := first.flowFor(uu.Reasoning)
				t.Flow, t.Tries[0].Flow = flow, flow
				t.Usage = routeUsage("openai", model, uu)
				if prompt != nil {
					t.Prompt = prompt.calibrated(promptCounted(t.Usage), catalog.ContextOf(model))
				}
				t.Tries[0].Served, t.Tries[0].Swapped = served, swapped(model, served)
				t.Served, t.Swapped = t.Tries[0].Served, t.Tries[0].Swapped
			})
		}
	}
	var res *http.Response
	for tries := 0; ; tries++ {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, u, bytes.NewReader(body))
		if err != nil {
			writeError(w, provider.Responses, 502, err.Error())
			end(502, err.Error(), 0, 0)
			return
		}
		copyHeaders(req.Header, r.Header)
		// left to the transport, the reply comes back plain for the usage
		// in it; the body goes plain too, as fogway read it
		req.Header.Del("Accept-Encoding")
		req.Header.Del("Content-Encoding")
		if res, err = s.client.Do(req); err != nil {
			msg := "OpenAI: " + err.Error()
			if rest == "/responses" {
				msg = codexUnreached(modelOf(body), err)
			}
			writeError(w, provider.Responses, 502, msg)
			end(502, msg, 0, 0)
			return
		}
		if rest != "/responses" || tries >= 3 || (res.StatusCode != 400 && res.StatusCode != 404) {
			break
		}
		// an item OpenAI can't take — another vendor's that it looks up
		// and doesn't have — is taken out and the rest asked again,
		// rather than the conversation stuck on it for good
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		res.Body = io.NopCloser(bytes.NewReader(msg))
		b, ok := withoutUnreadable(body, msg)
		if !ok {
			break
		}
		body = b
	}
	if rest == "/responses" {
		// a web page or nothing at all, served 200, is the 502 it stands
		// for (#1012)
		res = notAnAPIReply(res, "OpenAI: ")
	}
	defer res.Body.Close()
	if base == codexAPIBase && res.StatusCode == http.StatusUnauthorized {
		// Codex signed in with an API key OpenAI refuses — often one a
		// relay issued, left in ~/.codex/auth.json — and one of Codex's own
		// models was picked, which goes out with Codex's own sign-in
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		writeError(w, provider.Responses, res.StatusCode, codexKeyRefused(msg))
		s.record(Call{Time: start, From: provider.Responses, To: provider.Responses, Model: modelOf(body),
			Provider: "openai", Agent: agentOf(r), Status: res.StatusCode,
			Millis: time.Since(start).Milliseconds(), Error: res.Status})
		end(res.StatusCode, res.Status, 0, 0)
		return
	}
	if rest == "/responses" && res.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body = io.NopCloser(bytes.NewReader(msg))
		if len(bytes.TrimSpace(msg)) == 0 {
			// a failure with nothing said of it, which Codex shows as
			// "Unknown error" alone (#409): said where the turn went
			said := codexFailedEmpty(modelOf(body), res.Status)
			writeError(w, provider.Responses, res.StatusCode, said)
			s.record(Call{Time: start, From: provider.Responses, To: provider.Responses, Model: modelOf(body),
				Provider: "openai", Agent: agentOf(r), Kind: callKind(r.Header), Status: res.StatusCode,
				Millis: time.Since(start).Milliseconds(), Error: said})
			end(res.StatusCode, said, 0, 0)
			return
		}
	}
	for k, vs := range res.Header {
		if !hopHeader(k) {
			w.Header()[k] = vs
		}
	}
	modelsEtag(w.Header())
	w.WriteHeader(res.StatusCode)
	var sniff *usageSniffer
	if rest == "/responses" {
		ct := res.Header.Get("Content-Type")
		if ct == "" && streamOf(body) { // the ChatGPT backend streams without saying so
			ct = "text/event-stream"
		}
		sniff = newSniffer(provider.Responses, ct)
	}
	f, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	var refusal []byte // what OpenAI said, of a request it turned away
	for {
		n, err := res.Body.Read(buf)
		if n > 0 {
			if captureTitle {
				titleReply.add(buf[:n])
			}
			if sniff != nil {
				sniff.write(buf[:n])
				first.see(buf[:n])
				tools.add(buf[:n])
			}
			if res.StatusCode >= 400 && len(refusal) < 8<<10 {
				refusal = append(refusal, buf[:n]...)
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if f != nil {
				f.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	if sniff == nil {
		return
	}
	call := Call{Time: start, From: provider.Responses, To: provider.Responses, Model: modelOf(body),
		Provider: "openai", Agent: agentOf(r), Kind: kind, Status: res.StatusCode,
		Millis: time.Since(start).Milliseconds()}
	call.TTFT, call.FirstText = first.ms()
	call.ReasoningHidden = first.hidesReasoning(provider.Responses)
	uu.add(sniff.usage())
	call.Flow = first.flowFor(uu.Reasoning)
	call.Usage, served = uu, uu.Served
	errType := ""
	if res.StatusCode >= 400 {
		// its words, the status in front when it said none: the log and
		// the Routing view show why, not just that
		call.Error = provider.APIError(refusal, res.Status)
		errType = provider.ErrorType(refusal)
	}
	end(call.Status, call.Error, uu.Input+uu.Output+uu.CacheRead+uu.CacheWrite, uu.Output)
	s.record(call)
	rec := usage.Record{RouteID: tr.ID, Time: start, Agent: call.Agent, Provider: call.Provider, Host: provider.HostOf(base), Model: call.Model,
		Requested: call.Model, Served: served,
		Input: uu.Input, Output: uu.Output, CacheRead: uu.CacheRead, CacheWrite: uu.CacheWrite,
		Reasoning: uu.Reasoning, Effort: effort, Millis: call.Millis,
		// Codex's own sign-in: the request goes as Codex sent it
		TierAsked: bodyTier(provider.Responses, body), TierSent: bodyTier(provider.Responses, body), TierServed: uu.Tier, TTFT: call.TTFT, FirstText: call.FirstText, ReasoningHidden: call.ReasoningHidden, Flow: call.Flow, Status: call.Status, Session: sessionOf(r.Header), NativeSession: nativeSessionOf(r.Header), Kind: call.Kind,
		RequestID: requestID(res.Header), ResponseID: uu.ResponseID, Endpoint: r.URL.Path}
	failedWith(&rec, call.Status, call.Error, errType)
	appendUsage(r, rec)
}

// codexCompact compacts a conversation held by a fogway model: the model
// summarises it, and the summary goes back to Codex as the compaction item
// the backend would have made, marked as fogway's so fogway reads it back
// in later requests.
//
// A provider that answers the summary 404 — a relay that serves the
// conversation turn by turn but not the summary of it, "Upstream request
// failed" (#866) — or 400 for an item of it it doesn't have (#1008) is
// asked once more with the conversation as plain text, none of its items'
// ids or sealed reasoning in it; when that fails too the summary is
// fogway's own, the conversation's user messages and last reply,
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
		id = fmt.Sprintf("resp_fogway_%d", time.Now().UnixNano())
	}
	item := map[string]any{"type": "compaction", "id": "cmp_" + strings.TrimPrefix(id, "resp_"),
		"encrypted_content": fogwayCompaction + base64.StdEncoding.EncodeToString([]byte(sum.String()))}
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

// boundCallIDs is a Responses request with every call_id longer than 64
// characters as provider.BoundCallID has it, the rest of the request byte
// for byte (#732, congee949): a conversation that had a foreign provider's
// tool calls (two ids joined, 86 or 87 characters) went on Codex's own
// ChatGPT sign-in as it came, and the backend turned it away with 400
// "input[7].call_id … maximum length 64". A call and its output get the
// same id, on every request.
func boundCallIDs(body []byte) []byte {
	if !longCallID.Match(body) {
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
		var id string
		if json.Unmarshal(raw, &it) != nil || json.Unmarshal(it["call_id"], &id) != nil {
			continue
		}
		b := provider.BoundCallID(id)
		if b == id {
			continue
		}
		it["call_id"], _ = json.Marshal(b)
		if nb, err := marshalPlain(it); err == nil {
			items[i], changed = nb, true
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
	b.WriteString("fogway compacted this conversation without a model (" + why + "), so this is no summary: it is what the user asked")
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

func withSubagentPick(r *http.Request, p *SubagentPick) *http.Request {
	if p == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), subagentPickKey{}, p))
}

// withoutUnreadable is body without what OpenAI's refusal msg says it
// can't read: the item it names, or the reasoning another account sealed.
func withoutUnreadable(body, msg []byte) ([]byte, bool) {
	if !foreignReasoning.Match(msg) && !bytes.Contains(bytes.ToLower(msg), []byte("not found")) {
		return nil, false
	}
	if m := unreadableItem.FindSubmatch(msg); m != nil {
		if b, ok := withoutItem(body, string(m[1])); ok {
			return b, true
		}
	}
	if foreignReasoning.Match(msg) {
		return withoutReasoning(body)
	}
	return nil, false
}

// withoutItem is a Responses request without the input item of this id.
func withoutItem(body []byte, id string) ([]byte, bool) {
	var q map[string]json.RawMessage
	if json.Unmarshal(body, &q) != nil {
		return nil, false
	}
	var items []json.RawMessage
	if json.Unmarshal(q["input"], &items) != nil {
		return nil, false
	}
	kept := items[:0:0]
	for _, it := range items {
		var t struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(it, &t) == nil && t.ID == id {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == len(items) {
		return nil, false
	}
	q["input"], _ = json.Marshal(kept)
	b, err := json.Marshal(q)
	return b, err == nil
}

// codexKeyRefused says why a model of Codex's own failed with 401: what
// OpenAI said, and what to do about it.
func codexKeyRefused(msg []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	said := strings.TrimSpace(string(msg))
	if json.Unmarshal(msg, &e) == nil && e.Error.Message != "" {
		said = e.Error.Message
	}
	return "OpenAI refused the API key Codex is signed in with (" + said + "). " +
		"This is one of Codex's own models, which goes to OpenAI with Codex's own sign-in: " +
		"pick one of fogway's models in Codex (provider/model, or set it in fogway's Agents view), " +
		"or sign Codex in with ChatGPT, or with an OpenAI API key"
}

// codexUnreached says why a model of Codex's own failed with 502: OpenAI,
// where it goes with Codex's own sign-in, couldn't be reached — often from
// a Codex whose only sign-in is a relay's key, whose own models it still
// lists (#322) — and what to do about it.
func codexUnreached(model string, err error) string {
	return "OpenAI can't be reached (" + err.Error() + "). " + model + " is one of Codex's own models, " +
		"which goes to OpenAI with Codex's own sign-in: pick one of fogway's models in Codex " +
		"(provider/model, or set it in fogway's Agents view), or let Codex reach OpenAI"
}

// codexFailedEmpty says why a model of Codex's own failed when OpenAI gave
// an error status and nothing else: where it went, and what to do — a
// Codex left on its own model while fogway's Agents view picked another
// for it sends the turn to OpenAI, not to that one (#409).
func codexFailedEmpty(model, status string) string {
	return "OpenAI answered " + status + " and said nothing more. " + model + " is one of Codex's own models, " +
		"which goes to OpenAI with Codex's own sign-in, not to a provider in fogway: pick one of fogway's models in Codex " +
		"(provider/model, or set it in fogway's Agents view), or try again later"
}

// apiKey reports whether Codex signed in with an API key rather than a
// ChatGPT account.
func apiKey(h http.Header) bool {
	return strings.HasPrefix(strings.TrimPrefix(h.Get("Authorization"), "Bearer "), "sk-")
}

func hopHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Upgrade", "Te", "Trailer", "Content-Length":
		return true
	}
	return false
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		if !hopHeader(k) && http.CanonicalHeaderKey(k) != "Host" && http.CanonicalHeaderKey(k) != AccountHeader {
			dst[k] = vs
		}
	}
}

// codexNativeOff is the refusal for one of Codex's own models, asked at
// the ChatGPT backend's route by its bare id: disabled by that id, or as
// the codex provider's ("codex/gpt-5.5").
func codexNativeOff(model string) (string, bool) {
	if model == "" {
		return "", false
	}
	if provider.DisabledUnknown() {
		return unknownDisabledError(model), true
	}
	if pat, off := provider.IDDisabled(model); off {
		return disabledError(model, pat), true
	}
	if pat, off := provider.ModelDisabled("codex", model); off {
		return disabledError(model, pat), true
	}
	return "", false
}

// longCallID finds a call_id longer than the ChatGPT backend takes.
var longCallID = regexp.MustCompile(`"call_id"\s*:\s*"[^"]{65,}"`)

var upstreamFailedWords = regexp.MustCompile(`(?i)upstream request failed`)

// plainCompactMax bounds the conversation's text a plain summary request
// carries; the oldest of it is left out.
const plainCompactMax = 400_000

// localSummaryMax bounds the user's messages a local summary keeps, the
// latest of them, as Codex's own compaction does.
const localSummaryMax = 80_000
