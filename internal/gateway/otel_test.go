package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

// fastOTel has the exporter read the agents' transcripts and send what it
// holds within a fraction of a second, not every 2s and 5s, so a test that
// waits for an export doesn't wait seconds (#1654).
func fastOTel(t *testing.T) {
	every, sessionsEvery := usage.OTelEvery, usage.OTelSessionsEvery
	usage.OTelEvery, usage.OTelSessionsEvery = 200*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { usage.OTelEvery, usage.OTelSessionsEvery = every, sessionsEvery })
}

// otelExports takes every export the collector has received once stop() has
// flushed, as trace exports and metric exports. A request that outlasts an
// export interval has its attempt spans sent in one export and its root in
// the next (#1794), so a test reads them all, never only the first.
func otelExports(received chan []byte) (traces, metrics [][]byte) {
	for len(received) > 0 {
		b := <-received
		if strings.Contains(string(b), `"resourceMetrics"`) {
			metrics = append(metrics, b)
		} else {
			traces = append(traces, b)
		}
	}
	return traces, metrics
}

func TestOTelSessionDiscoveredDuringFirstRequest(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("MAGPIE_OTEL_ENABLED", "true")
	t.Setenv("MAGPIE_OTEL_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("MAGPIE_OTEL_HEADERS", "")
	t.Setenv("MAGPIE_OTEL_SESSIONS", "true")
	fastOTel(t)
	stop := usage.StartOTel()
	t.Cleanup(stop)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("User-Agent", "pi/1.0")
	req.Header.Set("X-Session-Id", "late-session")
	_, pending := beginOTelRequest(req, "", nil)
	_, exported := beginOTelRequest(req, "", nil)
	if pending == nil || exported == nil || !pending.sessionCandidate {
		t.Fatal("unseen session must retain gateway fallback")
	}
	if exported.skip() {
		t.Fatal("unseen session suppressed")
	}
	dir := filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "sessions", "work")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-10-03T00-00-00_late-session.jsonl"), []byte("{\"type\":\"session\",\"id\":\"late-session\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if usage.OTelSession("pi", "late-session") {
		t.Fatal("test must exercise a fresh cached miss")
	}
	if !pending.skip() {
		t.Fatal("newly discovered transcript duplicated gateway trace")
	}
	if exported.skip() {
		t.Fatal("discovery after an exported attempt must retain its root")
	}
	remote := &otelRequest{agent: "pi", session: "late-session"}
	if remote.skip() {
		t.Fatal("remote/keyed request must retain gateway tracing")
	}
}

func TestOTelFastRequestTranscriptDiscovery(t *testing.T) {
	for _, transcript := range []bool{true, false} {
		t.Run(fmt.Sprint(transcript), func(t *testing.T) {
			f := &fake{ctype: "application/json", reply: `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`}
			setup(t, provider.Chat, f)
			t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
			t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
			t.Setenv("CODEX_HOME", t.TempDir())
			dir := filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "sessions", "work")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			f.refuse = func([]byte) (int, string) {
				if transcript {
					if err := os.WriteFile(filepath.Join(dir, "2026-10-04T00-00-00_fast-session.jsonl"), []byte("{\"type\":\"session\",\"id\":\"fast-session\"}\n"), 0600); err != nil {
						t.Error(err)
					}
				}
				if usage.OTelSession("pi", "fast-session") {
					t.Error("request must still have a cached discovery miss")
				}
				return 0, ""
			}
			received := make(chan []byte, 8)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				received <- b
				io.WriteString(w, `{}`)
			}))
			defer collector.Close()
			t.Setenv("MAGPIE_OTEL_ENABLED", "true")
			t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
			t.Setenv("MAGPIE_OTEL_HEADERS", "")
			t.Setenv("MAGPIE_OTEL_SESSIONS", "true")
			t.Setenv("MAGPIE_OTEL_METRICS", "false")
			fastOTel(t)
			stop := usage.StartOTel()
			t.Cleanup(stop)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hello"}]}`))
			req.RemoteAddr = "127.0.0.1:1234"
			req.Header.Set("User-Agent", "pi/1.0")
			req.Header.Set("X-Session-Id", "fast-session")
			req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			w := httptest.NewRecorder()
			New().Handler().ServeHTTP(w, req)
			if w.Code != 200 || f.calls != 1 {
				t.Fatalf("request: status=%d calls=%d", w.Code, f.calls)
			}
			stop()
			count := 0
			for len(received) > 0 {
				count += strings.Count(string(<-received), `"traceId":"11111111111111111111111111111111"`)
			}
			want := 2
			if transcript {
				want = 0
			}
			if count != want {
				t.Fatalf("gateway spans=%d want=%d", count, want)
			}
		})
	}
}

func TestOTelExportsGatewayUsageWithoutContent(t *testing.T) {
	f := &fake{ctype: "application/json", reply: `{"id":"c1","model":"m1","choices":[{"message":{"role":"assistant","content":"PRIVATE-REPLY"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`}
	setup(t, provider.Chat, f)
	received := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- string(b)
		io.WriteString(w, `{}`)
	}))
	defer collector.Close()
	t.Setenv("MAGPIE_OTEL_ENABLED", "true")
	t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
	t.Setenv("MAGPIE_OTEL_HEADERS", "")
	t.Setenv("MAGPIE_OTEL_METRICS", "false")
	fastOTel(t)
	stop := usage.StartOTel()
	t.Cleanup(stop)
	code, body := post(t, "/v1/chat/completions", `{"model":"m1","messages":[{"role":"user","content":"PRIVATE-PROMPT"}]}`)
	if code != 200 || !strings.Contains(body, "PRIVATE-REPLY") {
		t.Fatalf("gateway: %d %s", code, body)
	}
	stop()
	select {
	case exported := <-received:
		if strings.Contains(exported, "PRIVATE-") || !strings.Contains(exported, `"magpie.route.id"`) || !strings.Contains(exported, `"intValue":"10"`) {
			t.Fatalf("export: %s", exported)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway usage was not exported")
	}
}

// with bodies on (#538) the span carries the request and the reply as
// Langfuse's observation input and output — a stream's text put together,
// a secret in the request still masked — and with it off neither
func TestOTelExportsBodiesWhenOn(t *testing.T) {
	stream := "data: {\"id\":\"c1\",\"model\":\"m1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"PRIVATE-\"}}]}\n\n" +
		"data: {\"id\":\"c1\",\"model\":\"m1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"REPLY\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"id\":\"c1\",\"model\":\"m1\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\ndata: [DONE]\n\n"
	for _, c := range []struct {
		name, bodies, ctype, reply, request string
	}{
		{"off", "false", "application/json", `{"id":"c1","model":"m1","choices":[{"message":{"role":"assistant","content":"PRIVATE-REPLY"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, `{"model":"m1","messages":[{"role":"user","content":"PRIVATE-PROMPT"}]}`},
		{"on", "true", "application/json", `{"id":"c1","model":"m1","choices":[{"message":{"role":"assistant","content":"PRIVATE-REPLY"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`, `{"model":"m1","messages":[{"role":"user","content":"PRIVATE-PROMPT"}]}`},
		{"on, streamed", "true", "text/event-stream", stream, `{"model":"m1","stream":true,"messages":[{"role":"user","content":"PRIVATE-PROMPT"}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{ctype: c.ctype, reply: c.reply}
			setup(t, provider.Chat, f)
			received := make(chan string, 1)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				received <- string(b)
				io.WriteString(w, `{}`)
			}))
			defer collector.Close()
			t.Setenv("MAGPIE_OTEL_ENABLED", "true")
			t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
			t.Setenv("MAGPIE_OTEL_HEADERS", "")
			t.Setenv("MAGPIE_OTEL_METRICS", "false")
			t.Setenv("MAGPIE_OTEL_BODIES", c.bodies)
			fastOTel(t)
			stop := usage.StartOTel()
			t.Cleanup(stop)
			secret := "sk-ant-api03-" + strings.Repeat("Q7x", 30)
			request := strings.Replace(c.request, `"model":"m1"`, `"model":"m1","metadata":{"api_key":"`+secret+`"}`, 1)
			if code, body := post(t, "/v1/chat/completions", request); code != 200 || !strings.Contains(body, "REPLY") {
				t.Fatalf("gateway: %d %s", code, body)
			}
			stop()
			var exported string
			select {
			case exported = <-received:
			case <-time.After(time.Second):
				t.Fatal("gateway usage was not exported")
			}
			if strings.Contains(exported, secret) {
				t.Fatalf("secret exported: %s", exported)
			}
			attrs := map[string]string{}
			var wire struct {
				ResourceSpans []struct {
					ScopeSpans []struct {
						Spans []struct {
							Attributes []struct {
								Key   string            `json:"key"`
								Value map[string]string `json:"value"`
							} `json:"attributes"`
						} `json:"spans"`
					} `json:"scopeSpans"`
				} `json:"resourceSpans"`
			}
			if err := json.Unmarshal([]byte(exported), &wire); err != nil {
				t.Fatal(err)
			}
			for _, a := range wire.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes {
				attrs[a.Key] = a.Value["stringValue"]
			}
			in, out := attrs["langfuse.observation.input"], attrs["langfuse.observation.output"]
			if c.bodies == "false" {
				if in != "" || out != "" || strings.Contains(exported, "PRIVATE-") {
					t.Fatalf("bodies exported while off: %s", exported)
				}
				return
			}
			if !strings.Contains(in, `"content":"PRIVATE-PROMPT"`) {
				t.Fatalf("input: %q", in)
			}
			if c.ctype == "text/event-stream" {
				if out != "PRIVATE-REPLY" {
					t.Fatalf("streamed output: %q", out)
				}
			} else if !strings.Contains(out, `"content":"PRIVATE-REPLY"`) {
				t.Fatalf("output: %q", out)
			}
		})
	}
}

func TestOTelParentValidation(t *testing.T) {
	valid := "00-" + strings.Repeat("a", 32) + "-" + strings.Repeat("b", 16) + "-01"
	for _, value := range []string{valid, "", strings.Replace(valid, "00-", "ff-", 1), strings.ToUpper(valid), strings.Replace(valid, strings.Repeat("a", 32), strings.Repeat("0", 32), 1), valid + "-extra", valid[:len(valid)-1] + "z"} {
		trace, parent := otelParent(value)
		if value == valid {
			if trace == "" || parent == "" {
				t.Fatal("valid parent rejected")
			}
		} else if trace != "" || parent != "" {
			t.Fatalf("accepted malformed parent: %q", value)
		}
	}
}

func TestOTelWaterfallFailureAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "failure"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &fake{ctype: "application/json", code: 400, reply: `{"error":{"message":"invalid request"}}`}
			if canceled {
				f.refuse = func([]byte) (int, string) { cancel(); return 400, f.reply }
			}
			setup(t, provider.Chat, f)
			received := make(chan []byte, 64)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				received <- b
				io.WriteString(w, `{}`)
			}))
			defer collector.Close()
			t.Setenv("MAGPIE_OTEL_ENABLED", "true")
			t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
			t.Setenv("MAGPIE_OTEL_HEADERS", "")
			t.Setenv("MAGPIE_OTEL_METRICS", "false")
			t.Setenv("MAGPIE_OTEL_BODIES", "false")
			fastOTel(t)
			stop := usage.StartOTel()
			t.Cleanup(stop)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`)).WithContext(ctx)
			New().Handler().ServeHTTP(httptest.NewRecorder(), req)
			stop()
			traces, _ := otelExports(received)
			if len(traces) == 0 {
				t.Fatal("no failure trace")
			}
			b := string(bytes.Join(traces, []byte("\n")))
			if strings.Count(b, `"code":2`) != 2 || strings.Count(b, `"parentSpanId"`) != 1 {
				t.Fatalf("expected failed child and root: %s", b)
			}
			if canceled && !strings.Contains(b, `"intValue":"499"`) {
				t.Fatalf("cancellation status missing: %s", b)
			}
		})
	}
}

// with bodies whole (#538) the export carries the request and reply entire,
// not the 256 KiB the Recent-calls page keeps, and no cut mark; with it off
// the first 256 KiB of each goes, cut, as before
func TestOTelExportsWholeBodiesWhenOn(t *testing.T) {
	big := strings.Repeat("A", 300<<10)
	reply := `{"id":"c1","model":"m1","choices":[{"message":{"role":"assistant","content":"` + big + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`
	request := `{"model":"m1","messages":[{"role":"user","content":"` + big + `"}]}`
	for _, c := range []struct {
		name   string
		whole  bool
		cut    bool
		stream bool
	}{
		{"cut", false, true, false},
		{"whole", true, false, false},
		{"cut streamed", false, true, true},
		{"whole streamed", true, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fake{ctype: "application/json", reply: reply}
			if c.stream {
				f.ctype = "text/event-stream"
				f.reply = "data: {\"choices\":[{\"delta\":{\"content\":\"" + big + "\"}}]}\n\ndata: [DONE]\n\n"
			}
			setup(t, provider.Chat, f)
			received := make(chan string, 1)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				received <- string(b)
				io.WriteString(w, `{}`)
			}))
			defer collector.Close()
			t.Setenv("MAGPIE_OTEL_ENABLED", "true")
			t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
			t.Setenv("MAGPIE_OTEL_HEADERS", "")
			t.Setenv("MAGPIE_OTEL_METRICS", "false")
			t.Setenv("MAGPIE_OTEL_BODIES", "true")
			t.Setenv("MAGPIE_OTEL_BODIES_WHOLE", fmt.Sprintf("%t", c.whole))
			fastOTel(t)
			stop := usage.StartOTel()
			t.Cleanup(stop)
			s := New()
			response := httptest.NewRecorder()
			s.Handler().ServeHTTP(response, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(request)))
			if response.Code != 200 || !strings.Contains(response.Body.String(), big) {
				t.Fatalf("gateway: %d", response.Code)
			}
			recent := s.Recent()
			if len(recent) != 1 {
				t.Fatalf("recent calls: %d, want 1", len(recent))
			}
			if recent[0].otelIn != nil || recent[0].otelOut != nil {
				t.Fatal("Recent calls retained whole OTel bodies")
			}
			if len(recent[0].RequestBody) != callBodyLimit || len(recent[0].ResponseBody) != callBodyLimit || !recent[0].RequestTruncated || !recent[0].ResponseTruncated {
				t.Fatal("Recent calls must retain only the truncated diagnostic bodies")
			}
			stop()
			var exported string
			select {
			case exported = <-received:
			case <-time.After(3 * time.Second):
				t.Fatal("gateway usage was not exported")
			}
			attrs := otelSpanAttrs(t, exported)
			in, out := attrs["langfuse.observation.input"], attrs["langfuse.observation.output"]
			if c.cut {
				if !strings.Contains(in, usage.BodyCut) || !strings.Contains(out, usage.BodyCut) {
					t.Fatalf("expected cut bodies, got in=%d out=%d", len(in), len(out))
				}
				return
			}
			if strings.Contains(in, usage.BodyCut) || strings.Contains(out, usage.BodyCut) {
				t.Fatalf("whole bodies were cut")
			}
			if len(in) < len(big) || len(out) < len(big) {
				t.Fatalf("short bodies: in=%d out=%d want at least %d", len(in), len(out), len(big))
			}
		})
	}
}

// otelSpanAttrs is the first span's string attributes in one exported batch.
func otelSpanAttrs(t *testing.T, exported string) map[string]string {
	t.Helper()
	var wire struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []struct {
					Attributes []struct {
						Key   string            `json:"key"`
						Value map[string]string `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal([]byte(exported), &wire); err != nil {
		t.Fatal(err)
	}
	attrs := map[string]string{}
	for _, a := range wire.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes {
		attrs[a.Key] = a.Value["stringValue"]
	}
	return attrs
}

// Each attempt keeps its own complete body across fallback.
func TestOTelWholeWaterfallFallback(t *testing.T) {
	big := strings.Repeat("A", 300<<10)
	secret := "sk-ant-api03-" + strings.Repeat("Q7x", 30)
	plan := &fake{ctype: "application/json", code: 429, reply: `{"error":{"message":"` + big + `"}}`}
	spare := &fake{ctype: "application/json", reply: `{"choices":[{"message":{"content":"` + big + `"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`}
	twoProviders(t, plan, spare)
	received := make(chan string, 4)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- string(b)
		io.WriteString(w, `{}`)
	}))
	defer collector.Close()
	t.Setenv("MAGPIE_OTEL_ENABLED", "true")
	t.Setenv("MAGPIE_OTEL_ENDPOINT", collector.URL)
	t.Setenv("MAGPIE_OTEL_HEADERS", "")
	t.Setenv("MAGPIE_OTEL_METRICS", "false")
	t.Setenv("MAGPIE_OTEL_BODIES", "true")
	t.Setenv("MAGPIE_OTEL_BODIES_WHOLE", "true")
	fastOTel(t)
	stop := usage.StartOTel()
	t.Cleanup(stop)
	request := `{"model":"plan/m1","messages":[{"role":"user","content":"` + big + `"}],"metadata":{"api_key":"` + secret + `"}}`
	if code, _ := post(t, "/v1/chat/completions", request); code != 200 {
		t.Fatalf("gateway: %d", code)
	}
	stop()
	exported := ""
	for len(received) > 0 {
		exported += <-received + "\n"
	}
	if strings.Contains(exported, secret) {
		t.Fatal("secret after capture limit leaked")
	}
	var wire struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []struct {
					ParentID   string `json:"parentSpanId"`
					SpanID     string `json:"spanId"`
					Attributes []struct {
						Key   string
						Value map[string]string
					}
				}
			}
		}
	}
	var spans []struct {
		ParentID   string `json:"parentSpanId"`
		SpanID     string `json:"spanId"`
		Attributes []struct {
			Key   string
			Value map[string]string
		}
	}
	for _, batch := range strings.Split(strings.TrimSpace(exported), "\n") {
		if err := json.Unmarshal([]byte(batch), &wire); err != nil {
			t.Fatal(err)
		}
		spans = append(spans, wire.ResourceSpans[0].ScopeSpans[0].Spans...)
	}
	if len(spans) != 3 {
		t.Fatalf("spans: %d", len(spans))
	}
	for i, span := range spans[:2] {
		attrs := map[string]string{}
		for _, a := range span.Attributes {
			attrs[a.Key] = a.Value["stringValue"]
		}
		for _, key := range []string{"langfuse.observation.input", "langfuse.observation.output"} {
			value := attrs[key]
			if !strings.Contains(value, big) || strings.Contains(value, usage.BodyCut) {
				t.Fatalf("attempt %d %s incomplete (%d bytes)", i, key, len(value))
			}
		}
		if span.ParentID != spans[2].SpanID {
			t.Fatal("wrong waterfall parent")
		}
	}
}

func TestOTelClaudeHelperClassification(t *testing.T) {
	for _, agent := range []string{"claude", "claude-desktop"} {
		for _, tc := range []struct{ body, kind, want string }{
			{`{"model":"claude-haiku","max_tokens":200,"messages":[]}`, "", "auxiliary"},
			{`{"model":"claude-sonnet","max_tokens":4096,"tools":[]}`, "", "auxiliary"},
			{`{"model":"claude-haiku","max_tokens":4096,"tools":[{"name":"Read"}]}`, "", ""},
			{`{"max_tokens":8192}`, "", ""},
			{`{"max_tokens":200}`, "title_generation", "title_generation"},
			{`{broken`, "", ""},
		} {
			if got := otelCallKind(agent, tc.kind, []byte(tc.body)); got != tc.want {
				t.Fatalf("%s %s: %q want %q", agent, tc.body, got, tc.want)
			}
		}
	}
	if got := otelCallKind("codex", "", []byte(`{"max_tokens":200}`)); got != "" {
		t.Fatal("Claude heuristic applied to Codex")
	}
}
