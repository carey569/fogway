package gateway

import (
	"errors"
	"slices"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// hybridGroup is group/auto over a local model and two cloud models.
func hybridGroup(mask bool) (provider.Group, []provider.Member) {
	g := provider.Group{ID: "auto", Members: []string{"local/q", "deep/v4", "kimi/k3"}, Hybrid: &provider.Hybrid{Local: "local/q", Mask: mask}}
	var ms []provider.Member
	for _, id := range g.Members {
		pid, model, _ := cutModel(id)
		ms = append(ms, provider.Member{ID: id, Path: []string{id}, Provider: provider.Provider{ID: pid}, Model: model})
	}
	return g, ms
}

func cutModel(id string) (string, string, bool) {
	for i := range id {
		if id[i] == '/' {
			return id[:i], id[i+1:], true
		}
	}
	return id, "", false
}

// said is a judge that answers j, counting how often it was asked.
func said(j judgement, err error, asked *int) judge {
	return func(model, text string, prev *judgement) (judgement, error) {
		*asked++
		return j, err
	}
}

func userTurn(text string) *Request {
	return &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: Text, Text: text}}}}}
}

func TestHybridRoutesTheTurnAsTheLocalModelJudgesIt(t *testing.T) {
	cases := []struct {
		name      string
		mask      bool
		j         judgement
		err       error
		use       string
		then      []string
		only      bool
		masked    bool
		sensitive bool
	}{
		{"simple stays here", true, judgement{Local: true}, nil, "local/q", nil, false, false, false},
		{"hard goes to the cloud, local behind it", true, judgement{}, nil, "deep/v4", []string{"kimi/k3"}, false, false, false},
		{"sensitive and simple stays here only", true, judgement{Local: true, Sensitive: true}, nil, "local/q", nil, true, false, true},
		{"sensitive and hard goes masked", true, judgement{Sensitive: true}, nil, "deep/v4", []string{"kimi/k3"}, false, true, true},
		{"masking off: sensitive decides nothing", false, judgement{Local: true, Sensitive: true}, nil, "local/q", nil, false, false, false},
		{"masking off: hard goes as it is", false, judgement{Sensitive: true}, nil, "deep/v4", []string{"kimi/k3"}, false, false, false},
		{"no judge: the cloud, masked when masking is on", true, judgement{}, errors.New("down"), "deep/v4", []string{"kimi/k3"}, false, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fresh(t)
			g, ms := hybridGroup(c.mask)
			asked := 0
			hit := hybridFor("k|"+c.name, g, ms, userTurn("fix the build"), said(c.j, c.err, &asked))
			h := hit.Hybrid
			if asked != 1 || hit.Use != c.use || !slices.Equal(hit.Then, c.then) || h.Only != c.only || h.Masked != c.masked || h.Sensitive != c.sensitive {
				t.Fatalf("asked %d, use %q then %v, %+v", asked, hit.Use, hit.Then, *h)
			}
		})
	}
}

func TestHybridTurnHoldsItsDecisionAndEscalates(t *testing.T) {
	fresh(t)
	g, ms := hybridGroup(true)
	asked := 0
	ask := said(judgement{Local: true}, nil, &asked)
	key := "k|hold"
	turn := userTurn("rename foo to bar")
	if hit := hybridFor(key, g, ms, turn, ask); hit.Use != "local/q" {
		t.Fatalf("first: %q", hit.Use)
	}
	// the agent's tool loop within the turn: held, nobody asked again
	within := &Request{Messages: append(slices.Clone(turn.Messages),
		Message{Role: "assistant", Parts: []Part{{Kind: ToolCall, ID: "c1", Name: "bash", Args: []byte(`{}`)}}},
		Message{Role: "user", Parts: []Part{{Kind: ToolResult, CallID: "c1", Text: "ok"}}})}
	if hit := hybridFor(key, g, ms, within, ask); !hit.Held || hit.Use != "local/q" || asked != 1 {
		t.Fatalf("within: held %v use %q asked %d", hit.Held, hit.Use, asked)
	}
	// the cloud answered (the local model failed): the rest of the turn is the cloud's
	hit := hybridFor(key, g, ms, within, ask)
	hybridAnswered(key, hit, ms, candidate{p: provider.Provider{ID: "deep"}, model: "v4", rest: "deep"})
	if hit := hybridFor(key, g, ms, within, ask); hit.Use != "deep/v4" || !hit.Hybrid.Escalated {
		t.Fatalf("escalated: use %q %+v", hit.Use, *hit.Hybrid)
	}
}

func TestHybridSensitiveLocalTurnNeverEscalates(t *testing.T) {
	fresh(t)
	g, ms := hybridGroup(true)
	asked := 0
	key := "k|only"
	turn := userTurn("our db password is hunter22, tidy this config")
	hit := hybridFor(key, g, ms, turn, said(judgement{Local: true, Sensitive: true}, nil, &asked))
	cands := []candidate{
		{p: provider.Provider{ID: "local"}, model: "q", rest: "local"},
		{p: provider.Provider{ID: "deep"}, model: "v4", rest: "deep"},
		{p: provider.Provider{ID: "kimi"}, model: "k3", rest: "kimi"},
	}
	pl := planned{order: make([]Weighed, len(cands))}
	got, pl := hybridOnly(hit, ms, cands, pl)
	if len(got) != 1 || got[0].rest != "local" || len(pl.order) != 1 {
		t.Fatalf("candidates %+v", got)
	}
	// tool errors twice running: still kept here
	within := &Request{Messages: append(slices.Clone(turn.Messages),
		Message{Role: "assistant", Parts: []Part{{Kind: ToolCall, ID: "a"}, {Kind: ToolCall, ID: "b"}}},
		Message{Role: "user", Parts: []Part{{Kind: ToolResult, CallID: "a", IsError: true}, {Kind: ToolResult, CallID: "b", IsError: true}}})}
	if hit := hybridFor(key, g, ms, within, nil); hit.Use != "local/q" || hit.Hybrid.Escalated {
		t.Fatalf("a sensitive turn moved: %q %+v", hit.Use, *hit.Hybrid)
	}
}

func TestHybridToolErrorsEscalate(t *testing.T) {
	fresh(t)
	g, ms := hybridGroup(false)
	asked := 0
	key := "k|tools"
	turn := userTurn("run the tests")
	hybridFor(key, g, ms, turn, said(judgement{Local: true}, nil, &asked))
	within := &Request{Messages: append(slices.Clone(turn.Messages),
		Message{Role: "assistant", Parts: []Part{{Kind: ToolCall, ID: "a"}}},
		Message{Role: "user", Parts: []Part{{Kind: ToolResult, CallID: "a", IsError: true, Text: "unknown tool"}}},
		Message{Role: "assistant", Parts: []Part{{Kind: ToolCall, ID: "b"}}},
		Message{Role: "user", Parts: []Part{{Kind: ToolResult, CallID: "b", IsError: true, Text: "bad arguments"}}})}
	if hit := hybridFor(key, g, ms, within, nil); hit.Use != "deep/v4" || !hit.Hybrid.Escalated {
		t.Fatalf("not escalated: %q %+v", hit.Use, *hit.Hybrid)
	}
}

func TestReadJudgement(t *testing.T) {
	for in, want := range map[string]judgement{
		`{"local": true, "sensitive": false, "why": "small edit"}`: {Local: true, Why: "small edit"},
		"```json\n{\"local\": false, \"sensitive\": \"yes\"}\n```": {Sensitive: true},
		`Sure. {"local":1,"sensitive":0}`:                          {Local: true},
		"local: no\nsensitive: yes":                                {Sensitive: true},
	} {
		got, err := readJudgement(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v; want %+v", in, got, err, want)
		}
	}
	if _, err := readJudgement("I can't tell."); err == nil {
		t.Error("an answer without the two was read")
	}
}

func TestReadMaskItemsKeepsWhatIsInTheText(t *testing.T) {
	text := "connect to db-prod-07.corp.acme.cn as svc_billing with password Tr0ub4dor&3"
	got := readMaskItems(`{"items": ["db-prod-07.corp.acme.cn", "Tr0ub4dor&3", "not in the text", "as", "db-prod-07.corp.acme.cn"]}`, text)
	if !slices.Equal(got, []string{"db-prod-07.corp.acme.cn", "Tr0ub4dor&3"}) {
		t.Fatalf("%q", got)
	}
}
