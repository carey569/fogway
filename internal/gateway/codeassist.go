package gateway

import (
	"cmp"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/carey569/fogway/internal/catalog"
)

// ---- Gemini's request and stream (provider side) ----------------------------
//
// A Gemini request is built in Code Assist's envelope, {model, request},
// and unwrapped for Gemini's API and Vertex AI (geminiRequest); a stream's
// chunks are read bare or wrapped the same way, {response}.

// skipSignature stands in for a thought signature on a function call the
// model didn't make here: Google checks the ones Gemini 3 hands out, and
// this one tells it not to.
const skipSignature = "skip_thought_signature_validator"

var unsafeToolID = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// buildCodeAssistSent builds the Gemini envelope, {model, request}, for a
// request to the model sent; geminiRequest unwraps it. agent "vertex" is
// Vertex AI's request (buildVertex).
func buildCodeAssistSent(r *Request, sent, agent string) []byte {
	vx := agent == "vertex"
	claude := strings.Contains(strings.ToLower(sent), "claude")
	names := map[string]string{}
	var contents []map[string]any
	for _, m := range r.Messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		var parts []map[string]any
		toolImage := map[int]bool{} // which of parts are a tool's images
		// signed: a step Gemini signed, whose later calls go without, as
		// Gemini signs the first (#1445, as chat's #687)
		signed := false
		for _, p := range m.Parts {
			if p.Kind == ToolCall && p.Signature != "" {
				signed = true
			}
		}
		for _, p := range m.Parts {
			switch p.Kind {
			case Text:
				if p.Text != "" || (p.Signature != "" && !claude) {
					part := map[string]any{"text": p.Text}
					if p.Signature != "" && !claude {
						// Gemini's signature on its text, back as it gave it
						part["thoughtSignature"] = p.Signature
					}
					parts = append(parts, part)
				}
			case Image, File:
				if p.Data != "" {
					parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": p.MediaType, "data": p.Data}})
				} else if p.URL != "" {
					parts = append(parts, map[string]any{"fileData": map[string]any{"mimeType": p.MediaType, "fileUri": p.URL}})
				}
			case ToolCall:
				names[p.ID] = p.Name
				call := map[string]any{"name": p.Name, "args": json.RawMessage(argsOf(p))}
				if p.ID != "" {
					call["id"] = p.ID
				}
				part := map[string]any{"functionCall": call}
				switch {
				case claude:
					part["thoughtSignature"] = skipSignature
				case p.Signature != "":
					// Gemini's own signature, back as it gave it (#1445)
					part["thoughtSignature"] = p.Signature
				case !signed:
					part["thoughtSignature"] = skipSignature
				}
				parts = append(parts, part)
			case ToolResult:
				name := names[p.CallID]
				if name == "" {
					name = "tool"
				}
				key := "output"
				if p.IsError {
					key = "error"
				}
				res := map[string]any{"name": name, "response": map[string]any{key: p.Text}}
				if p.CallID != "" {
					res["id"] = p.CallID
				}
				parts = append(parts, map[string]any{"functionResponse": res})
				// the images a tool returned follow its response, as Gemini
				// CLI sends a file it read
				for _, im := range p.Images {
					if im.Data != "" {
						toolImage[len(parts)] = true
						parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": im.MediaType, "data": im.Data}})
					} else if im.URL != "" {
						toolImage[len(parts)] = true
						parts = append(parts, map[string]any{"fileData": map[string]any{"mimeType": im.MediaType, "fileUri": im.URL}})
					}
				}
			}
			// thinking isn't sent back: its signatures belong to whoever
			// made them, and Google turns away ones it didn't
		}
		// what the user said goes in a turn of its own after the tools'
		// responses: Gemini answers a turn mixing a functionResponse with
		// text "Requests ending with a model turn are not supported" (#1533,
		// yycsyu: Codex's compaction prompt after a tool's output), and takes
		// the two as separate turns.
		var said []map[string]any
		if role == "user" && hasResponse(parts) {
			var tools []map[string]any
			for i, part := range parts {
				if part["functionResponse"] != nil || toolImage[i] {
					tools = append(tools, part)
				} else {
					said = append(said, part)
				}
			}
			parts = tools
		}
		for _, ps := range [][]map[string]any{parts, said} {
			if len(ps) == 0 {
				continue
			}
			if n := len(contents); n > 0 && contents[n-1]["role"] == role && (role != "user" || hasResponse(contents[n-1]["parts"].([]map[string]any)) == hasResponse(ps)) {
				contents[n-1]["parts"] = append(contents[n-1]["parts"].([]map[string]any), ps...)
				continue
			}
			contents = append(contents, map[string]any{"role": role, "parts": ps})
		}
	}
	contents = pairFunctionTurns(contents)
	if len(contents) > 0 && contents[0]["role"] == "model" {
		// Gemini turns away a history that opens with the model's turn
		// (what an agent's compaction can leave): "function call turn comes
		// immediately after a user turn". Gemini CLI's hardenHistory puts
		// this user turn before it, and so does this.
		contents = append([]map[string]any{{"role": "user", "parts": []map[string]any{{"text": "[Continuing from previous AI thoughts...]"}}}}, contents...)
	}
	if n := len(contents); n > 0 && contents[n-1]["role"] == "model" {
		// nor one that ends with it: "Requests ending with a model turn
		// are not supported" on Vertex AI and Gemini's API alike (yycsyu: Codex compacting on gemini-3.8-flash). A user
		// turn goes after it, as Gemini CLI keeps its history ending
		contents = append(contents, map[string]any{"role": "user", "parts": []map[string]any{{"text": modelLastAsk(r)}}})
	}
	req := map[string]any{"contents": contents}
	if r.System != "" {
		req["systemInstruction"] = map[string]any{"role": "user", "parts": []map[string]any{{"text": r.System}}}
	}

	if len(r.Tools) > 0 && r.ToolChoice != "none" {
		var decls []map[string]any
		for _, t := range r.Tools {
			d := map[string]any{"name": t.Name}
			if t.Description != "" {
				d["description"] = t.Description
			}
			schema := t.Schema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			d["parametersJsonSchema"] = schema
			decls = append(decls, d)
		}
		req["tools"] = []map[string]any{{"functionDeclarations": decls}}
		mode := "AUTO"
		fc := map[string]any{}
		switch {
		case r.ToolChoice == "required":
			mode = "ANY"
		case strings.HasPrefix(r.ToolChoice, "name:"):
			mode = "ANY"
			fc["allowedFunctionNames"] = []string{strings.TrimPrefix(r.ToolChoice, "name:")}
		}
		fc["mode"] = mode
		req["toolConfig"] = map[string]any{"functionCallingConfig": fc}
	} else if r.WebSearch {
		// Google's own search, which Gemini runs as long as no function is
		// declared beside it
		req["tools"] = []map[string]any{{"googleSearch": map[string]any{}}}
	}

	gen := map[string]any{}
	if r.MaxTokens > 0 {
		gen["maxOutputTokens"] = r.MaxTokens
	}
	if r.Temp != nil {
		gen["temperature"] = *r.Temp
	}
	if r.TopP != nil {
		gen["topP"] = *r.TopP
	}
	if len(r.Stop) > 0 {
		gen["stopSequences"] = r.Stop
	}
	tc := thinkingConfig(r, sent, claude)
	if vx {
		tc = vertexThinking(r, sent)
	}
	if tc != nil {
		gen["thinkingConfig"] = tc
		// Claude's answer has to have room past its thinking
		if b, ok := tc["thinkingBudget"].(int); ok && claude && gen["maxOutputTokens"] == nil {
			gen["maxOutputTokens"] = b + 32000
		}
	}
	if r.Format != nil {
		// the answer as JSON, of the client's schema when it gave one
		gen["responseMimeType"] = "application/json"
		if sc := r.Format.schema(); len(sc) > 0 {
			gen["responseJsonSchema"] = sc
		}
	}
	if catalog.DrawsID(sent) {
		// an image model draws only when asked for images too
		gen["responseModalities"] = []string{"TEXT", "IMAGE"}
	}
	if len(gen) > 0 {
		req["generationConfig"] = gen
	}
	b, _ := json.Marshal(map[string]any{"model": sent, "request": req})
	return b
}

// interruptedToolNote is the result a call left unanswered is answered
// with, the words the chat build gives such a call (pairToolMessages).
const interruptedToolNote = "[The result of this tool call is unavailable: the turn was interrupted.]"

// pairFunctionTurns answers a model turn's function calls in the turn
// right after it, as Google's API reads its histories ("Please ensure
// that the number of function response parts is equal to the number of
// function call parts of the function call turn"): a call the client
// answers nowhere — an interrupted turn leaves its call pending, and
// Google turns every later turn of the conversation away with 400 over
// the one — is answered with the synthetic result the chat build gives
// such a call (pairToolMessages, #1275). The answer goes in the turn that
// follows the calls when that turn already holds answers, and otherwise
// in a turn of its own before it: a user turn mixing a functionResponse
// with text is turned away too (#1533). A call with no id counts as
// answered by a response of its name with no id.
func pairFunctionTurns(contents []map[string]any) []map[string]any {
	if len(contents) == 0 {
		return contents
	}
	partsOf := func(c map[string]any) []map[string]any {
		ps, _ := c["parts"].([]map[string]any)
		return ps
	}
	answered := map[string]bool{}
	answeredName := map[string]bool{}
	for _, c := range contents {
		for _, p := range partsOf(c) {
			if fr, ok := p["functionResponse"].(map[string]any); ok {
				if id, _ := fr["id"].(string); id != "" {
					answered[id] = true
				} else {
					name, _ := fr["name"].(string)
					answeredName[name] = true
				}
			}
		}
	}
	out := make([]map[string]any, 0, len(contents)+1)
	for i, c := range contents {
		out = append(out, c)
		if c["role"] != "model" {
			continue
		}
		var synth []map[string]any
		for _, p := range partsOf(c) {
			fc, ok := p["functionCall"].(map[string]any)
			if !ok {
				continue
			}
			id, _ := fc["id"].(string)
			name, _ := fc["name"].(string)
			if id != "" && answered[id] || id == "" && answeredName[name] {
				continue
			}
			res := map[string]any{"name": name, "response": map[string]any{"output": interruptedToolNote}}
			if id != "" {
				res["id"] = id
			}
			synth = append(synth, map[string]any{"functionResponse": res})
		}
		if len(synth) == 0 {
			continue
		}
		if i+1 < len(contents) && contents[i+1]["role"] == "user" && hasResponse(partsOf(contents[i+1])) {
			contents[i+1]["parts"] = append(synth, partsOf(contents[i+1])...)
			continue
		}
		out = append(out, map[string]any{"role": "user", "parts": synth})
	}
	return out
}

// thinkingConfig says how hard the model should think: a level for
// Gemini 3, a budget for the rest. A model that doesn't think gets none.
//
// Gemini 3's levels: medium goes as medium to Flash, and up to high to
// Pro, which took low and high only (3 Pro). Minimal and reasoning off
// (the auto mode classifier's least level) go at low, which every Gemini 3
// text model takes: 3.7 and 3.8 Flash have no minimal (AI Studio answers
// 3.8's with a 400). An image model may have no low (3.1 Flash Image:
// minimal and high), and goes at high.
func thinkingConfig(r *Request, model string, claude bool) map[string]any {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "gpt-oss") || claude && !strings.Contains(m, "thinking") {
		return nil
	}
	if r.Effort == "" && !r.Thinking {
		// Claude asked to think still has to be told how much
		if !claude {
			return nil
		}
	}
	tc := map[string]any{"includeThoughts": true}
	if strings.HasPrefix(m, "gemini-3") || strings.HasPrefix(m, "gemini-pro-agent") {
		if effort := r.Effort; effort != "" {
			level := "high"
			switch {
			case effort == "low":
				level = "low"
			case offEffort(effort) && !catalog.DrawsID(m):
				level = "low"
			case effort == "medium" && strings.Contains(m, "flash"):
				level = "medium"
			}
			tc["thinkingLevel"] = level
		}
		return tc
	}
	budget := budgetOf(effortOf(r.Effort))
	if claude && r.MaxTokens > 0 && budget >= r.MaxTokens {
		budget = r.MaxTokens - 1
		if budget < 1024 {
			return nil
		}
	}
	tc["thinkingBudget"] = budget
	return tc
}

// codeAssistDecoder reads Code Assist's stream: Gemini chunks, each wrapped
// in {response}.
type codeAssistDecoder struct {
	started bool
	tools   bool
	stopped bool
	// searched is a googleSearch's grounding told already: it is given
	// again with each chunk after it
	searched bool
	usage    *Usage
	// held is text that may be the start of a call the model wrote out as
	// text (see textCall), kept back until it is one or isn't
	held string
}

func (d *codeAssistDecoder) decode(data string, emit func(Event)) error {
	var ch struct {
		Response *geminiChunk `json:"response"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
		geminiChunk
	}
	if err := json.Unmarshal([]byte(data), &ch); err != nil {
		// an error in a shape of its own is still the reply's end (#1933)
		if e, ok := carriedError(data); ok {
			emit(e)
		}
		return nil
	}
	if ch.Error != nil {
		emit(Event{Kind: KError, Text: cmp.Or(ch.Error.Message, streamErrorText(data))})
		return nil
	}
	c := ch.geminiChunk
	if ch.Response != nil {
		c = *ch.Response
	}
	if c.UsageMetadata != nil {
		u := c.UsageMetadata
		d.usage = &Usage{Input: max(u.Prompt-u.Cached, 0), CacheRead: u.Cached,
			Output: u.Candidates + u.Thoughts, Reasoning: u.Thoughts}
	}
	if !d.started {
		d.started = true
		// the prompt's count rides on the first chunk: told at the start,
		// as Anthropic's message_start tells it, which Claude Code's
		// context meter reads before the turn ends (#1887)
		var prompt Usage
		if d.usage != nil {
			prompt = Usage{Input: d.usage.Input, CacheRead: d.usage.CacheRead}
		}
		emit(Event{Kind: KStart, MsgID: c.ResponseID, Model: c.ModelVersion, Usage: prompt})
	}
	for _, cand := range c.Candidates {
		for _, p := range cand.Content.Parts {
			switch {
			case p.FunctionCall != nil:
				d.flush(emit)
				id := p.FunctionCall.ID
				if id == "" {
					id = "call_" + newID()
				}
				args := p.FunctionCall.Args
				if len(args) == 0 || string(args) == "null" {
					args = json.RawMessage("{}")
				}
				d.tools = true
				// its signature rides in the id, and comes back off it
				// (signedID, unsignCalls), as on chat (#687, #1445)
				emit(Event{Kind: KToolStart, ID: signedID(id, p.Signature), Name: toolOfCall(p.FunctionCall.Name)})
				emit(Event{Kind: KToolArgs, Text: string(args)})
			case p.Thought:
				d.flush(emit)
				if p.Text != "" {
					emit(Event{Kind: KThink, Text: p.Text})
				}
				if p.Signature != "" {
					emit(Event{Kind: KSig, Text: p.Signature})
				}
			case p.InlineData != nil && p.InlineData.Data != "":
				// a picture an image model drew (gemini-*-image): its
				// thought images, drafts on the way, stay with the
				// reasoning above (#620)
				d.flush(emit)
				emit(Event{Kind: KImage, Name: p.InlineData.MimeType, Text: p.InlineData.Data})
			case p.Text != "" || p.Signature != "":
				d.text(p.Text, emit)
				if p.Signature != "" {
					// Gemini's signature on its text, after it (#1445)
					d.flush(emit)
					emit(Event{Kind: KTextSig, Text: p.Signature})
				}
			}
		}
		if g := cand.GroundingMetadata; g != nil && !d.searched {
			// what googleSearch found: the pages, by their site's name and
			// an address of Google's that leads to them
			var hits []Hit
			for _, c := range g.GroundingChunks {
				if c.Web != nil && c.Web.URI != "" {
					hits = append(hits, Hit{Title: cmp.Or(c.Web.Title, c.Web.URI), URL: c.Web.URI})
				}
			}
			if len(hits) > 0 || len(g.WebSearchQueries) > 0 {
				d.flush(emit)
				d.searched = true
				emit(Event{Kind: KSearch, Text: strings.Join(g.WebSearchQueries, "; "), Hits: hits})
			}
		}
		if cand.FinishReason != "" && !d.stopped {
			d.flush(emit)
			d.stopped = true
			stop := stopFromGemini(cand.FinishReason)
			if d.tools && stop == "stop" {
				stop = "tool"
			}
			emit(Event{Kind: KStop, Stop: stop})
		}
	}
	if d.stopped && d.usage != nil {
		emit(Event{Kind: KUsage, Usage: *d.usage})
		d.usage = nil
	}
	return nil
}

// Gemini's own tools live in a namespace, default_api, and a model now and
// then names one of the client's with it: as a call's name
// (default_api:bash), or, on Antigravity, as a whole call written out in
// its reply's text — "Let's look.call:default_api:bash{command:ls}" — with
// no call made (#636). Either way the client is left with nothing it can
// run, so the decoder takes the namespace off a call's name and makes a
// call written out in text the call it meant. Text that only looks like
// the start of one is held back until it is one or isn't.
const callMarker = "call:" + geminiNamespace + ":"

const geminiNamespace = "default_api"

// toolOfCall is a call's tool name without Gemini's namespace.
func toolOfCall(name string) string {
	for _, sep := range []string{":", "."} {
		if t, ok := strings.CutPrefix(name, geminiNamespace+sep); ok && t != "" {
			return t
		}
	}
	return name
}

// maxHeld bounds a call written out as text: one that goes on longer is
// taken for text after all.
const maxHeld = 256 << 10

// text passes a text part on, making any call written out in it a call.
func (d *codeAssistDecoder) text(s string, emit func(Event)) {
	s = d.held + s
	d.held = ""
	say := func(t string) {
		if t != "" {
			emit(Event{Kind: KText, Text: t})
		}
	}
	for s != "" {
		i := strings.Index(s, callMarker)
		if i < 0 {
			// a tail that may be the start of the marker waits for the
			// next part
			k := len(callMarker) - 1
			for ; k > 0 && !strings.HasSuffix(s, callMarker[:k]); k-- {
			}
			say(s[:len(s)-k])
			d.held = s[len(s)-k:]
			return
		}
		say(s[:i])
		s = s[i:]
		name, args, n := textCall(s)
		switch {
		case n == 0 && len(s) < maxHeld: // not all here yet
			d.held = s
			return
		case n <= 0: // not a call
			say(s[:len(callMarker)])
			s = s[len(callMarker):]
		default:
			d.tools = true
			emit(Event{Kind: KToolStart, ID: "call_" + newID(), Name: name})
			emit(Event{Kind: KToolArgs, Text: string(args)})
			s = s[n:]
		}
	}
}

// flush passes held text on as text: what comes next isn't more of it.
func (d *codeAssistDecoder) flush(emit func(Event)) {
	if d.held != "" {
		emit(Event{Kind: KText, Text: d.held})
		d.held = ""
	}
}

// ctrl46 quotes a string in the calls Gemini writes out as text.
const ctrl46 = "<ctrl46>"

// textCall reads a call written out as text at the start of s,
// call:default_api:<tool>{<args>}: its tool, its arguments as JSON and its
// length; a length of 0 where s may yet be one and isn't all here, -1
// where it isn't one.
func textCall(s string) (string, json.RawMessage, int) {
	j := len(callMarker)
	for j < len(s) && isIdent(s[j]) {
		j++
	}
	if j == len(s) {
		return "", nil, 0
	}
	name := s[len(callMarker):j]
	if name == "" || s[j] != '{' {
		return "", nil, -1
	}
	end := closeBrace(s, j)
	if end < 0 {
		return "", nil, 0
	}
	args, ok := textArgs(s[j+1 : end])
	if !ok {
		return "", nil, -1
	}
	return name, args, end + 1
}

func isIdent(c byte) bool {
	return c == '_' || c == '-' || c == '.' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// closeBrace is where the brace open at s[i] closes, with braces inside
// <ctrl46> quotes left out; -1 where it doesn't in s.
func closeBrace(s string, i int) int {
	depth := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], ctrl46) {
			k := strings.Index(s[i+len(ctrl46):], ctrl46)
			if k < 0 {
				return -1
			}
			i += 2*len(ctrl46) + k
			continue
		}
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

// textArgs makes a written-out call's arguments a JSON object. They come
// as JSON's own members or as key:value pairs, the value quoted with
// <ctrl46>, JSON, or bare text running to the next ",key:" or the end.
func textArgs(body string) (json.RawMessage, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return json.RawMessage("{}"), true
	}
	if obj := "{" + body + "}"; json.Valid([]byte(obj)) {
		return json.RawMessage(obj), true
	}
	var b strings.Builder
	b.WriteByte('{')
	for first := true; body != ""; first = false {
		k := 0
		for k < len(body) && isIdent(body[k]) {
			k++
		}
		if k == 0 || k == len(body) || body[k] != ':' {
			return nil, false
		}
		key, _ := json.Marshal(body[:k])
		body = body[k+1:]
		var val []byte
		if rest, ok := strings.CutPrefix(body, ctrl46); ok {
			e := strings.Index(rest, ctrl46)
			if e < 0 {
				return nil, false
			}
			val, _ = json.Marshal(rest[:e])
			body = strings.TrimLeft(rest[e+len(ctrl46):], " ")
			if body != "" {
				if body[0] != ',' {
					return nil, false
				}
				body = strings.TrimLeft(body[1:], " ")
			}
		} else {
			e := nextKey(body)
			v := strings.TrimSpace(body[:e])
			if json.Valid([]byte(v)) {
				val = []byte(v)
			} else {
				val, _ = json.Marshal(v)
			}
			body = body[e:]
			if body != "" {
				body = strings.TrimLeft(body[1:], " ")
			}
		}
		if !first {
			b.WriteByte(',')
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String()), true
}

// nextKey is where a bare value ends: at the "," of the next ",key:" outside
// brackets, or the end.
func nextKey(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth != 0 {
				continue
			}
			k := i + 1
			for k < len(s) && s[k] == ' ' {
				k++
			}
			start := k
			for k < len(s) && isIdent(s[k]) {
				k++
			}
			if k > start && k < len(s) && s[k] == ':' {
				return i
			}
		}
	}
	return len(s)
}

type geminiChunk struct {
	Candidates []struct {
		Content struct {
			Parts []gPart `json:"parts"`
		} `json:"content"`
		FinishReason      string `json:"finishReason"`
		GroundingMetadata *struct {
			WebSearchQueries []string `json:"webSearchQueries"`
			GroundingChunks  []struct {
				Web *struct {
					Title string `json:"title"`
					URI   string `json:"uri"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
	UsageMetadata *struct {
		Prompt     int `json:"promptTokenCount"`
		Candidates int `json:"candidatesTokenCount"`
		Thoughts   int `json:"thoughtsTokenCount"`
		Cached     int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
	ResponseID   string `json:"responseId"`
}

func stopFromGemini(s string) string {
	switch s {
	case "MAX_TOKENS":
		return "length"
	case "STOP", "FINISH_REASON_UNSPECIFIED", "OTHER":
		return "stop"
	case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
		return "stop"
	}
	// SAFETY, RECITATION, BLOCKLIST, PROHIBITED_CONTENT, SPII, ...
	return "filter"
}

// hasResponse says whether a Gemini turn's parts hold a tool's response.
func hasResponse(parts []map[string]any) bool {
	for _, p := range parts {
		if p["functionResponse"] != nil {
			return true
		}
	}
	return false
}

// buildCodeAssist builds the Gemini envelope for a request to model.
func buildCodeAssist(r *Request, model, agent string) []byte {
	return buildCodeAssistSent(r, model, agent)
}
