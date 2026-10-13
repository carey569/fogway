package gateway

// The edge-cloud route (provider.Hybrid): a routing group whose member on
// this computer judges each user's turn as it begins (hybrid_judge.go) and
// is sent the turns it can do, the others going to the group's cloud
// members. A turn stays where it was sent, as a rule's does (rules.go),
// with three exceptions, each the user's choice (see provider.Hybrid):
//
//   - a turn the local model can't finish — the conversation outgrew its
//     window, its reply failed, it went round in circles — goes on in the
//     cloud, for the rest of the turn (escalated), unless the turn is
//     sensitive and masking is on: that one stays on this computer, and the
//     agent is told the local model's error;
//   - a turn for the cloud the local model judged sensitive goes masked
//     while masking is on (hybrid_mask.go);
//   - a cloud that doesn't answer leaves the turn to the local model, as a
//     group's last member.

import (
	"slices"
	"strings"
	"time"

	"github.com/fogway/fogway/internal/provider"
	"github.com/fogway/fogway/internal/redact"
)

// HybridHit is what the trace tells of the edge-cloud route's decision.
type HybridHit struct {
	Local     bool   `json:"local"`               // the turn goes to the local model first
	Sensitive bool   `json:"sensitive,omitempty"` // the judge said it carries what mustn't leave
	Masked    bool   `json:"masked,omitempty"`    // its requests to the cloud go masked
	Only      bool   `json:"only,omitempty"`      // sensitive, kept on this computer: no escalation
	Escalated bool   `json:"escalated,omitempty"` // the local model failed within it; the cloud has it now
	Why       string `json:"why,omitempty"`       // the judge's words
	Error     string `json:"error,omitempty"`     // why the judge couldn't say: the cloud has the turn
	Ms        int    `json:"ms,omitempty"`        // how long judging took
	Terms     int    `json:"terms,omitempty"`     // strings the local model found to mask
}

// hybridLocal is the group's member on this computer, among ms: the one
// Hybrid names, ready or not.
func hybridLocal(g provider.Group, ms []provider.Member) (provider.Member, bool) {
	i := slices.IndexFunc(ms, func(m provider.Member) bool { return named(m, g.Hybrid.Local) })
	if i < 0 {
		return provider.Member{}, false
	}
	return ms[i], true
}

// hybridCloud are the group's other members, in its order.
func hybridCloud(g provider.Group, ms []provider.Member) []string {
	var out []string
	for _, m := range ms {
		if !named(m, g.Hybrid.Local) && !slices.Contains(out, m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

// hybridFor decides a hybrid group's request, as ruleFor does a group's
// rules: at a turn's start by the local model's judgement, within it by
// what was decided then (escalated, by the cloud).
func hybridFor(key string, g provider.Group, ms []provider.Member, req *Request, ask judge) *RuleHit {
	turn, within := turnIn(req)
	now := ruleClock()
	size := estimate(req)
	local := g.Hybrid.Local
	cloud := hybridCloud(g, ms)
	hit := &RuleHit{Turn: turn, Tokens: size}
	put := func(h *HybridHit) {
		hit.Hybrid = h
		switch {
		case h.Local || len(cloud) == 0:
			hit.Use, hit.N = local, 1
		default:
			hit.Use, hit.Then, hit.N = cloud[0], cloud[1:], 1
		}
	}
	turnRules.Lock()
	tr, had := turnRules.m[key]
	turnRules.Unlock()
	if had && now.Sub(tr.at) > stickKeep {
		had = false
	}
	if within && had && tr.hybrid != nil && (tr.turn == turn || turn == 0) {
		h := *tr.hybrid
		// the local model's tool calls failing over and over (a call the
		// agent couldn't run, twice running) is a turn it can't finish
		if h.Local && !h.Only && failingTools(req) >= 2 {
			h.Local, h.Escalated = false, true
			turnRules.Lock()
			if cur, ok := turnRules.m[key]; ok {
				cur.hybrid = &h
				turnRules.m[key] = cur
			}
			turnRules.Unlock()
		}
		put(&h)
		hit.Held = true
		turnRules.Lock()
		if cur, ok := turnRules.m[key]; ok {
			cur.at, cur.size = now, size
			turnRules.m[key] = cur
		}
		turnRules.Unlock()
		return hit
	}
	// a request that compacts the conversation is as long as it is: the
	// cloud's, masked as the turn is, never the local window's
	if compacting(req) {
		h := &HybridHit{Why: "compacting"}
		if had && tr.hybrid != nil {
			h.Sensitive, h.Masked = tr.hybrid.Sensitive, tr.hybrid.Masked || tr.hybrid.Only && g.Hybrid.Mask
		}
		put(h)
		hit.Compact = true
		return hit
	}
	h := &HybridHit{}
	var prev *judgement
	if had && tr.hybrid != nil {
		prev = &judgement{Local: tr.hybrid.Local && !tr.hybrid.Escalated, Sensitive: tr.hybrid.Sensitive}
	}
	text := userText(req)
	switch {
	case ask == nil:
		h.Error = "a classifier's own call is judged by no one"
	case text == "":
		// tool results alone, or an empty message: as the turn before
		if prev != nil {
			h.Local, h.Sensitive = prev.Local, prev.Sensitive
		}
	default:
		t0 := time.Now()
		j, err := ask(local, text, prev)
		h.Ms = int(time.Since(t0).Milliseconds())
		if err != nil {
			h.Error = err.Error()
			// no judge: the cloud takes the turn, masked when masking is
			// on, as nothing says the turn isn't sensitive
			h.Sensitive = g.Hybrid.Mask
		} else {
			h.Local, h.Sensitive, h.Why = j.Local, j.Sensitive, j.Why
		}
	}
	// what the judge read is the message's head and tail (userText); a
	// secret anywhere in the conversation the rules find makes the turn
	// sensitive too
	if g.Hybrid.Mask && !h.Sensitive && carriesSecrets(req) {
		h.Sensitive = true
		h.Why = strings.TrimPrefix(h.Why+"; a secret in the conversation", "; ")
	}
	if !g.Hybrid.Mask {
		h.Sensitive = false // masking off: what is sensitive decides nothing
	}
	if len(cloud) == 0 {
		h.Local = true
	}
	h.Only = h.Local && h.Sensitive
	h.Masked = !h.Local && h.Sensitive
	put(h)
	turnRules.Lock()
	defer turnRules.Unlock()
	input, answered := tr.input, tr.answered
	if cur, ok := turnRules.m[key]; ok {
		input, answered = cur.input, cur.answered
	}
	turnRules.m[key] = turnRule{turn: turn, use: hit.Use, n: hit.N, at: now, input: input, answered: answered, size: size, hybrid: h}
	return hit
}

// judge asks the local model to judge a turn's message.
type judge func(model, text string, prev *judgement) (judgement, error)

// askJudge is judge through the gateway, as a classifier is asked.
func (s *Server) askJudge(model, text string, prev *judgement) (judgement, error) {
	answer, err := s.askChat(model, judgeBody(model, text, prev))
	if err != nil {
		return judgement{}, err
	}
	return readJudgement(answer)
}

// hybridOnly keeps of cands those of the local member, for a sensitive
// turn kept on this computer: none of the cloud's is asked, whatever
// fails.
func hybridOnly(hit *RuleHit, ms []provider.Member, cands []candidate, pl planned) ([]candidate, planned) {
	if hit == nil || hit.Hybrid == nil || !hit.Hybrid.Only {
		return cands, pl
	}
	var out []candidate
	var order []Weighed
	for i, c := range cands {
		if slices.ContainsFunc(ruleMembers(hit, ms), func(m provider.Member) bool { return ofMember(c, m) }) {
			out = append(out, c)
			order = append(order, pl.order[i])
		}
	}
	pl.order = order
	return out, pl
}

// hybridAnswered notes who answered a hybrid turn's request: the cloud
// answering a turn sent to the local model first is the turn escalated,
// and its requests after go to the cloud first.
func hybridAnswered(key string, hit *RuleHit, ms []provider.Member, c candidate) {
	if hit == nil || hit.Hybrid == nil || !hit.Hybrid.Local || hit.Hybrid.Escalated {
		return
	}
	if slices.ContainsFunc(ruleMembers(hit, ms), func(m provider.Member) bool { return ofMember(c, m) }) {
		return // the local model answered
	}
	turnRules.Lock()
	defer turnRules.Unlock()
	if tr, ok := turnRules.m[key]; ok && tr.hybrid != nil {
		h := *tr.hybrid
		h.Local, h.Escalated = false, true
		tr.hybrid = &h
		turnRules.m[key] = tr
	}
}

// failingTools is how many of the conversation's last tool results, back
// to the last that went well, report an error: the agent couldn't run the
// call, or the call failed.
func failingTools(req *Request) int {
	n := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		for j := len(m.Parts) - 1; j >= 0; j-- {
			p := m.Parts[j]
			if p.Kind != ToolResult {
				continue
			}
			if !p.IsError {
				return n
			}
			n++
		}
		if m.Role == "user" && slices.ContainsFunc(m.Parts, func(p Part) bool { return p.Kind == Text && strings.TrimSpace(p.Text) != "" && userSaid(m) != "" }) {
			return n // the user's own words began the turn
		}
	}
	return n
}

// hybridEscalate marks a hybrid turn the local model can't finish — its
// reply went round in circles until the loop guard ended it — as the
// cloud's from its next request on.
func hybridEscalate(key string) {
	turnRules.Lock()
	defer turnRules.Unlock()
	if tr, ok := turnRules.m[key]; ok && tr.hybrid != nil && tr.hybrid.Local && !tr.hybrid.Only {
		h := *tr.hybrid
		h.Local, h.Escalated = false, true
		tr.hybrid = &h
		turnRules.m[key] = tr
	}
}

// carriesSecrets says the rules find a secret (an API key, a private key, a
// token, a password) in what the user and the agent's tools put in req.
func carriesSecrets(req *Request) bool {
	for _, t := range conversationText(req) {
		if _, n := redact.Mask(t, redact.Options{Secrets: true}); n > 0 {
			return true
		}
	}
	return false
}
