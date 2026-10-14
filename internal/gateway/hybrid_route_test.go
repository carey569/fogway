package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/provider"
)

// edgeUp is a Chat API that keeps what it was asked and answers each kind
// of request as the test set it.
type edgeUp struct {
	mu    sync.Mutex
	asked []string
	// answer is the reply's text to a request of body b
	answer func(b string) string
}

func (u *edgeUp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.asked = append(u.asked, string(b))
	u.mu.Unlock()
	text, _ := json.Marshal(u.answer(string(b)))
	if strings.Contains(string(b), `"stream":true`) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse(
			`data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":`+string(text)+`}}]}`,
			`data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":`+string(text)+`},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
}

// turns are the requests that were an agent's own turn: not a judge's or a
// masking question.
func (u *edgeUp) turns() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []string
	for _, b := range u.asked {
		if !strings.Contains(b, "You are the router") && !strings.Contains(b, "You protect a user's private data") {
			out = append(out, b)
		}
	}
	return out
}

// lastUser is the text of the last user message of a Chat request.
func lastUser(b string) string {
	var q struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal([]byte(b), &q)
	for i := len(q.Messages) - 1; i >= 0; i-- {
		if q.Messages[i].Role == "user" {
			if s, ok := q.Messages[i].Content.(string); ok {
				return s
			}
			c, _ := json.Marshal(q.Messages[i].Content)
			return string(c)
		}
	}
	return ""
}

// edgeCloud is group/auto over local/q, which judges as said, and
// cloud/big, which says back what it was told.
func edgeCloud(t *testing.T, mask bool, said string) (*Server, *edgeUp, *edgeUp) {
	t.Helper()
	fresh(t)
	local := &edgeUp{answer: func(b string) string {
		switch {
		case strings.Contains(b, "You are the router"):
			return said
		case strings.Contains(b, "You protect a user's private data"):
			return `{"items": ["Project Nightjar", "not in the text"]}`
		}
		return "from local"
	}}
	cloud := &edgeUp{answer: func(b string) string { return "you said: " + lastUser(b) }}
	for _, x := range []struct {
		id, model string
		up        *edgeUp
		ctx       int
	}{{"local", "q", local, 8192}, {"cloud", "big", cloud, 200000}} {
		srv := httptest.NewServer(x.up)
		t.Cleanup(srv.Close)
		if err := provider.Save(provider.Provider{ID: x.id, Name: x.id, Key: "k", Models: []string{x.model}, Chat: srv.URL + "/v1"}); err != nil {
			t.Fatal(err)
		}
		if err := catalog.SaveLive(x.id, srv.URL+"/v1", []catalog.Model{{ID: x.model, Context: x.ctx}}); err != nil {
			t.Fatal(err)
		}
	}
	g := provider.Group{ID: provider.HybridGroupID, Name: "Edge-cloud auto", Members: []string{"local/q", "cloud/big"}, Hybrid: &provider.Hybrid{Local: "local/q", Mask: mask}}
	if err := provider.SaveGroup(g); err != nil {
		t.Fatal(err)
	}
	return New(), local, cloud
}

func edgeAsk(text string) string {
	return `{"model":"group/auto","messages":[{"role":"user","content":` + quote(text) + `}]}`
}

// A turn the local model can do is its own; it is asked once, as the turn
// begins.
func TestEdgeCloudKeepsWhatTheLocalModelCanDo(t *testing.T) {
	s, local, cloud := edgeCloud(t, true, `{"local": true, "sensitive": false, "why": "a small edit"}`)
	out, r := postOK(t, s, "e1", edgeAsk("rename foo to bar in util.go"))
	if !strings.Contains(out, "from local") || len(cloud.turns()) != 0 {
		t.Fatalf("%s; cloud asked %d", out, len(cloud.turns()))
	}
	if h := r.Rule.Hybrid; h == nil || !h.Local || h.Why != "a small edit" {
		t.Fatalf("decision %+v", r.Rule)
	}
	if len(local.asked) != 2 { // the judge, then the turn
		t.Fatalf("local asked %d times", len(local.asked))
	}
}

// With masking on, a sensitive turn for the cloud goes there with what the
// local model found (and the secrets the rules find) as placeholders, and
// the reply comes back with them restored.
func TestEdgeCloudMasksASensitiveTurnForTheCloud(t *testing.T) {
	s, _, cloud := edgeCloud(t, true, `{"local": false, "sensitive": true, "why": "an internal codename"}`)
	key := "sk-ant-api03-" + strings.Repeat("Q7v", 30)
	text := "Plan the migration of Project Nightjar to the new cluster; deploy key " + key
	out, r := postOK(t, s, "e2", edgeAsk(text))
	turns := cloud.turns()
	if len(turns) != 1 {
		t.Fatalf("cloud asked %d times", len(turns))
	}
	if strings.Contains(turns[0], "Project Nightjar") || strings.Contains(turns[0], key) {
		t.Fatalf("the cloud saw it: %s", turns[0])
	}
	if !strings.Contains(out, "Project Nightjar") || !strings.Contains(out, key) {
		t.Fatalf("not restored: %s", out)
	}
	if h := r.Rule.Hybrid; h == nil || !h.Masked || h.Terms != 1 {
		t.Fatalf("decision %+v", r.Rule.Hybrid)
	}
}

// Masking off, the same turn goes as it is.
func TestEdgeCloudUnmaskedWhenMaskingIsOff(t *testing.T) {
	s, _, cloud := edgeCloud(t, false, `{"local": false, "sensitive": true}`)
	postOK(t, s, "e3", edgeAsk("Plan the migration of Project Nightjar"))
	if turns := cloud.turns(); len(turns) != 1 || !strings.Contains(turns[0], "Project Nightjar") {
		t.Fatalf("cloud got %q", turns)
	}
}

// A secret the judge didn't see, past the head and tail of a long message
// it reads, makes the turn sensitive too: a turn for the local model stays
// on this computer.
func TestEdgeCloudSecretTheJudgeMissed(t *testing.T) {
	s, _, cloud := edgeCloud(t, true, `{"local": true, "sensitive": false}`)
	key := "ghp_" + strings.Repeat("a1B2", 9)
	text := strings.Repeat("log line\n", 500) + "token " + key + "\n" + strings.Repeat("log line\n", 500)
	out, r := postOK(t, s, "e4", edgeAsk(text))
	if h := r.Rule.Hybrid; h == nil || !h.Sensitive || !h.Only {
		t.Fatalf("decision %+v", r.Rule.Hybrid)
	}
	if !strings.Contains(out, "from local") || len(cloud.turns()) != 0 {
		t.Fatalf("%s; cloud asked %d", out, len(cloud.turns()))
	}
}

// A local model that can't judge leaves the turn to the cloud.
func TestEdgeCloudJudgeDown(t *testing.T) {
	s, _, cloud := edgeCloud(t, false, `I am not sure.`)
	out, r := postOK(t, s, "e5", edgeAsk("explain this stack trace"))
	if !strings.Contains(out, "you said") || len(cloud.turns()) != 1 {
		t.Fatalf("%s", out)
	}
	if h := r.Rule.Hybrid; h == nil || h.Local || h.Error == "" {
		t.Fatalf("decision %+v", r.Rule.Hybrid)
	}
}
