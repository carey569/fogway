package gateway

// Masking a sensitive turn before it goes to the cloud: the group's local
// model reads what the user and the agent's tools put in the conversation
// and names the strings that must not leave this computer, which are then
// masked as the user's own words are (redact.Options.Words): the same
// placeholder for the same string every time, so the cloud's cache of the
// conversation holds, and the reply has them back on the way to the agent.
//
// A conversation's text is read once: what was read is remembered by its
// hash, and what was found by the conversation, so a long session costs
// one look at each new message or tool result, not at the whole history
// every request. A request with more new text than maskChunks of
// maskChunk is read that far; the rest goes with the rules' masking alone
// (secrets and personal data), which every masked request has.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maskChunk  = 4000 // characters of text the local model reads at once
	maskChunks = 8    // read for one request at most
	maskMin    = 3    // the shortest string taken as sensitive
	maskMax    = 200  // and the longest: a whole paragraph is no name
)

// maskTimeout is how long the local model is given to read one chunk: a
// few thousand characters in and a short list out, which a small model on
// a laptop's CPU takes seconds over, not the classifier's eight.
var maskTimeout = 60 * time.Second

// maskSeen is, by conversation, what of its text was read and the strings
// found in it.
var maskSeen = struct {
	sync.Mutex
	m map[string]*maskConv
}{m: map[string]*maskConv{}}

type maskConv struct {
	read  map[string]bool // sha256 of each chunk read
	terms []string
	at    time.Time
}

const maskPrompt = `You protect a user's private data before their conversation with a coding assistant is sent to a cloud model.
List every string in the text below that must not leave the user's computer, exactly as it appears in the text:
secrets (passwords, API keys, tokens, private keys, connection strings), personal data (people's names together with their details, ID numbers, phone numbers, emails, addresses),
internal hostnames, domains, IP addresses, internal project or product code names, customer or company names in confidential business data, and anything the text says is confidential.
Do not list ordinary code, keywords, public library names, generic words or file paths that reveal nothing.
Answer with JSON only: {"items": ["…", "…"]}; {"items": []} when there is none.`

// maskBody is the Chat request asking model for the sensitive strings of
// text.
func maskBody(model, text string) []byte {
	req := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": maskPrompt},
			{"role": "user", "content": "<text>\n" + text + "\n</text>"},
		},
		"stream":               false,
		"temperature":          0,
		"max_tokens":           1024,
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
		"reasoning_effort":     "none",
		"response_format":      map[string]any{"type": "json_object"},
	}
	body, _ := json.Marshal(req)
	return body
}

// readMaskItems is the strings an answer names that are in text: a model
// may name what isn't there, or reword it, which masking can't match.
func readMaskItems(answer, text string) []string {
	var out struct {
		Items []any `json:"items"`
	}
	m := jsonObject.FindString(answer)
	if m == "" || json.Unmarshal([]byte(m), &out) != nil {
		return nil
	}
	var items []string
	for _, it := range out.Items {
		s, _ := it.(string)
		s = strings.TrimSpace(s)
		n := utf8.RuneCountInString(s)
		if n < maskMin || n > maskMax || !strings.Contains(text, s) || slices.Contains(items, s) {
			continue
		}
		items = append(items, s)
	}
	return items
}

// conversationText is what the user and the agent's tools put in the
// request, in chunks of at most maskChunk characters: the user's words,
// the tools' results, and what the assistant wrote (which repeats what it
// read). The agent's system prompt and tool definitions are its own.
func conversationText(req *Request) []string {
	var out []string
	add := func(s string) {
		for s != "" {
			r := []rune(s)
			if len(r) <= maskChunk {
				out = append(out, s)
				return
			}
			out = append(out, string(r[:maskChunk]))
			s = string(r[maskChunk:])
		}
	}
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			switch p.Kind {
			case Text, ToolResult:
				if strings.TrimSpace(p.Text) != "" {
					add(p.Text)
				}
			case ToolCall:
				if len(p.Args) > 2 {
					add(string(p.Args))
				}
			}
		}
	}
	return out
}

// sensitiveTerms is what the local model found sensitive in the
// conversation key, reading what of req it hasn't read, newest first, at
// most maskChunks of it now. A chunk whose answer couldn't be read is read
// again next time.
func (s *Server) sensitiveTerms(key, model string, req *Request) []string {
	maskSeen.Lock()
	c := maskSeen.m[key]
	if c == nil {
		c = &maskConv{read: map[string]bool{}}
		maskSeen.m[key] = c
	}
	c.at = time.Now()
	var todo []string
	chunks := conversationText(req)
	for i := len(chunks) - 1; i >= 0 && len(todo) < maskChunks; i-- {
		h := sha256.Sum256([]byte(chunks[i]))
		k := hex.EncodeToString(h[:12])
		if !c.read[k] {
			todo = append(todo, chunks[i])
		}
	}
	maskSeen.Unlock()

	type found struct {
		chunk string
		items []string
		ok    bool
	}
	results := make([]found, len(todo))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2) // llama-server's slots
	for i, t := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			answer, err := s.askChatFor(model, maskBody(model, t), maskTimeout)
			results[i] = found{chunk: t, ok: err == nil}
			if err == nil {
				results[i].items = readMaskItems(answer, t)
			}
		}()
	}
	wg.Wait()

	maskSeen.Lock()
	defer maskSeen.Unlock()
	for _, r := range results {
		if !r.ok {
			continue
		}
		h := sha256.Sum256([]byte(r.chunk))
		c.read[hex.EncodeToString(h[:12])] = true
		for _, it := range r.items {
			if !slices.Contains(c.terms, it) {
				c.terms = append(c.terms, it)
			}
		}
	}
	if len(maskSeen.m) > 1024 {
		for k, v := range maskSeen.m {
			if time.Since(v.at) > stickKeep {
				delete(maskSeen.m, k)
			}
		}
	}
	return slices.Clone(c.terms)
}

// maskNote is what the trace says of a masked request.
func maskNote(terms []string, n int) string {
	return fmt.Sprintf("%d masked (%d found by the local model)", n, len(terms))
}
