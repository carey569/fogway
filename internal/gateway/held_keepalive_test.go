package gateway

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// thinkingOn serves Claude thinking at length: its first events, then
// Anthropic's pings until heard is closed (or 3s), then the rest of reply.
func thinkingOn(t *testing.T, id, reply string, heard <-chan struct{}) {
	t.Helper()
	events := strings.SplitAfter(reply, "\n\n")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		io.WriteString(w, events[0]+events[1]+events[2])
		f.Flush()
		for wait := time.After(3 * time.Second); ; {
			select {
			case <-heard:
			case <-wait:
			case <-time.After(10 * time.Millisecond):
				io.WriteString(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n")
				f.Flush()
				continue
			}
			break
		}
		for _, ev := range events[3:] {
			io.WriteString(w, ev)
		}
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"claude-opus-5-5"}, Anthropic: up.URL}); err != nil {
		t.Fatal(err)
	}
}

// ask posts claudeCodeAsk to the gateway over HTTP and reads the reply as
// it comes, closing heard once a keepalive arrived.
func ask(t *testing.T, s *Server, heard chan struct{}) (int, string) {
	t.Helper()
	gw := httptest.NewServer(s.Handler())
	t.Cleanup(gw.Close)
	resp, err := http.Post(gw.URL+"/v1/messages", "application/json", strings.NewReader(claudeCodeAsk))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var once sync.Once
	var body strings.Builder
	rd := bufio.NewReader(resp.Body)
	for {
		line, err := rd.ReadString('\n')
		body.WriteString(line)
		if strings.HasPrefix(line, ": keepalive") {
			once.Do(func() { close(heard) })
		}
		if err != nil {
			break
		}
	}
	return resp.StatusCode, body.String()
}

// Claude thinking for longer than the agent's idle timeout through a
// group, where its stream is held for a refusal to go to another account
// (#248): the agent heard nothing for up to four minutes, and Alma's 240s
// timeout for a reasoning model ended the turn first (#751). It is kept
// alive with comments meanwhile, which say nothing of the reasoning, and a
// refusal after them still goes to the next member, whose answer follows
// in the same stream.
func TestHeldThinkingKeepsTheAgentAlive(t *testing.T) {
	fresh(t)
	keepFast(t, keepaliveGap, 20*time.Millisecond, keepaliveLongest)
	k := keepHeldAfter
	keepHeldAfter = 30 * time.Millisecond
	t.Cleanup(func() { keepHeldAfter = k })

	heard := make(chan struct{})
	thinkingOn(t, "a", anthropicThoughtThenRefused, heard)
	b := &scripted{replies: []reply{{200, "text/event-stream", anthropicAnswer}}}
	scriptedOn(t, "b", provider.Anthropic, b)
	refusalGroup(t, "a/claude-opus-5-5", "b/m")
	s := New()
	code, body := ask(t, s, heard)
	alive := strings.Index(body, ": keepalive")
	if code != 200 || alive < 0 || !strings.Contains(body[alive:], "from b") || strings.Contains(body, "user wants") || strings.Contains(body, `"refusal"`) || b.n != 1 {
		t.Fatalf("%d %q (b %d)", code, body, b.n)
	}
	if strings.Count(body, "event: message_start") != 1 {
		t.Fatalf("one reply in the stream: %q", body)
	}
	r := lastRoute(s)
	if len(r.Tries) != 2 || r.Tries[0].Fail != failRefused || r.Status != 200 {
		t.Fatalf("tries: %+v", r.Tries)
	}

	// thinking that goes on to its answer comes through whole after them
	fresh(t)
	heard = make(chan struct{})
	thinkingOn(t, "a", anthropicThoughtThenAnswered, heard)
	scriptedOn(t, "b", provider.Anthropic, &scripted{replies: []reply{{200, "text/event-stream", anthropicAnswer}}})
	refusalGroup(t, "a/claude-opus-5-5", "b/m")
	code, body = ask(t, New(), heard)
	alive = strings.Index(body, ": keepalive")
	if code != 200 || alive < 0 || !strings.Contains(body[alive:], `"thinking":"hm"`) || !strings.Contains(body, "from a") || strings.Contains(body, "from b") {
		t.Fatalf("answered: %d %q", code, body)
	}

	// refused with nobody left: nothing was held for it, as nobody else
	// could answer, so the thinking came as it came and the refusal after
	// it as Claude's own (iTianbao on X)
	fresh(t)
	heard = make(chan struct{})
	thinkingOn(t, "a", anthropicThoughtThenRefused, heard)
	refusalGroup(t, "a/claude-opus-5-5")
	code, body = ask(t, New(), heard)
	if code != 200 || !strings.Contains(body, "user wants") || !strings.Contains(body, `"stop_reason":"refusal"`) || strings.Count(body, "event: message_start") != 1 {
		t.Fatalf("refused, nobody left: %d %q", code, body)
	}
}
