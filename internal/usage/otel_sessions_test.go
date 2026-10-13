package usage

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/settings"
)

func TestSessionTraceWirePrivacyAndTokenOwnership(t *testing.T) {
	at := time.Now()
	secret := "sk-ant-api03-" + strings.Repeat("Q7x", 30)
	for _, bodies := range []bool{false, true} {
		cfg := settings.OTel{Bodies: bodies}
		model := sessions.TraceSpan{Agent: "pi", Session: "session", Turn: "turn", ID: "1111111111111111", Parent: "2222222222222222", Name: "model m", Kind: "generation", Model: "m", Provider: "magpie", Start: at, End: at.Add(time.Second), Input: `{"api_key":"` + secret + `","prompt":"PRIVATE-PROMPT"}`, Tokens: sessions.Tokens{Input: 10, Output: 5}}
		tool := model
		tool.ID = "3333333333333333"
		tool.Kind = "tool"
		tool.Name = "bash"
		tool.Inferred = true
		root := model
		root.ID = model.Parent
		root.Parent = ""
		root.Kind = "span"
		root.Name = "pi interaction"
		e := newOTelExporter()
		payload := e.traces([]Record{sessionRecord(root, cfg), sessionRecord(model, cfg), sessionRecord(tool, cfg)})
		e.cancel()
		wire, _ := json.Marshal(payload)
		s := string(wire)
		if strings.Contains(s, secret) || !bodies && strings.Contains(s, "PRIVATE-PROMPT") {
			t.Fatalf("private content exported: %s", s)
		}
		if strings.Count(s, `"key":"gen_ai.usage.input_tokens"`) != 1 || strings.Count(s, `"key":"langfuse.session.id"`) != 3 || !strings.Contains(s, `"stringValue":"tool"`) || !strings.Contains(s, `"stringValue":"inferred"`) {
			t.Fatalf("wire: %s", s)
		}
	}
	cfg := settings.OTel{Bodies: true}
	long := strings.Repeat("A", 300<<10)
	if got := sessionBody(long, cfg); !strings.HasSuffix(got, BodyCut) {
		t.Fatal("body limit not applied")
	}
	// Scrub secret-named fields before cutting: truncation must not disable JSON masking.
	if got := sessionBody(`{"api_key":"`+long+`"}`, cfg); strings.Contains(got, strings.Repeat("A", 100)) {
		t.Fatal("truncation bypassed secret-field masking")
	}
	cfg.BodiesWhole = true
	if got := sessionBody(long, cfg); got != long {
		t.Fatal("whole body truncated")
	}
}

func TestSessionExporterDefersPendingRoot(t *testing.T) {
	e := newOTelExporter()
	defer e.cancel()
	cfg := settings.OTel{Bodies: true}
	at := time.Now()
	root := sessions.TraceSpan{Agent: "codex", Session: "session", Turn: "turn", ID: "1111111111111111", Kind: "span", Name: "codex interaction", Start: at, End: at, Pending: true}
	e.offerSession(root, cfg)
	if len(e.queue) != 0 {
		t.Fatal("unfinished root exported")
	}
	root.Pending = false
	root.End = at.Add(time.Minute)
	root.Input = `[{"type":"local_image","path":"/tmp/image.png"},{"type":"text","text":"Describe this image"}]`
	root.Output = "An image"
	e.offerSession(root, cfg)
	if len(e.queue) != 1 {
		t.Fatal("completed root missing")
	}
	record := (<-e.queue).record
	wire, err := json.Marshal(e.traces([]Record{record}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"langfuse.observation.input", "langfuse.observation.output", "local_image", "Describe this image", "An image"} {
		if !strings.Contains(string(wire), want) {
			t.Fatalf("completed root missing %s", want)
		}
	}
}

func TestSessionTracingRetainsRemoteGatewayTraces(t *testing.T) {
	received := make(chan []byte, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received <- b
		io.WriteString(w, `{}`)
	}))
	defer collector.Close()
	otelConfig(t, settings.OTel{Endpoint: collector.URL})
	t.Setenv("MAGPIE_OTEL_SESSIONS", "true")
	stop := StartOTel()
	t.Cleanup(stop)
	Append(Record{Time: time.Now(), Agent: "pi", Via: "office", Model: "remote-model", Input: 10, Status: 200})
	stop()
	select {
	case b := <-received:
		if !strings.Contains(string(b), "remote-model") {
			t.Fatal("missing remote usage")
		}
	case <-time.After(time.Second):
		t.Fatal("remote gateway trace suppressed")
	}
}

func TestSessionObservationUpdatesDoNotDuplicateMetrics(t *testing.T) {
	var metric string
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/metrics") {
			metric = string(b)
		}
		io.WriteString(w, `{}`)
	}))
	defer collector.Close()
	otelConfig(t, settings.OTel{Endpoint: collector.URL, Metrics: true})
	cfg, err := settings.OTelExport()
	if err != nil {
		t.Fatal(err)
	}
	e := newOTelExporter()
	defer e.cancel()
	now := time.Now()
	span := sessions.TraceSpan{Agent: "gemini", Session: "s", Turn: "t", ID: "1111111111111111", Parent: "2222222222222222", Kind: "generation", Model: "m", Start: now, End: now.Add(time.Second), Tokens: sessions.Tokens{Input: 10, Output: 2}}
	first := sessionRecord(span, cfg)
	span.Update = true
	second := sessionRecord(span, cfg)
	e.flush([]otelItem{{record: first, config: cfg}, {record: second, config: cfg}})
	if metric == "" || strings.Contains(metric, `"count":"2"`) {
		t.Fatalf("update metrics: %s", metric)
	}
}

func TestSessionTracingMatchesNativeConversation(t *testing.T) {
	otelConfig(t, settings.OTel{Endpoint: "http://localhost:4318"})
	t.Setenv("MAGPIE_OTEL_SESSIONS", "true")
	e := newOTelExporter()
	previous := otel.Swap(e)
	defer func() { otel.Store(previous); e.cancel() }()
	cfg, err := settings.OTelExport()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	e.sessions.Store(&sessionAvailability{config: cfg,
		agents: map[string]time.Time{"codex": now},
		seen: map[observedSession]time.Time{
			{"codex", "local"}:    now,
			{"codex", "stale"}:    now.Add(-6 * time.Minute),
			{"pi", "other-agent"}: now,
		},
	})
	for _, tc := range []struct {
		name, session, native string
		dedup                 bool
	}{
		{"visible", "local", "", true},
		{"WSL mirrored", "wsl", "", false},
		{"Docker Desktop", "docker", "", false},
		{"native takes precedence", "routing", "local", true},
		{"unseen native", "local", "wsl", false},
		{"stale session despite recent agent", "stale", "", false},
		{"same ID different agent", "other-agent", "", false},
		{"no ID fallback", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offerOTel(Record{Agent: "codex", Local: true, Session: tc.session, NativeSession: tc.native, Time: now, Status: 200})
			want := 1
			if tc.dedup {
				want = 0
			}
			if len(e.queue) != want {
				t.Fatalf("queued=%d want=%d", len(e.queue), want)
			}
			if want != 0 {
				<-e.queue
			}
		})
	}
	cfg.Bodies = !cfg.Bodies
	e.sessions.Store(&sessionAvailability{config: cfg, seen: map[observedSession]time.Time{{"codex", "local"}: now}})
	if OTelSession("codex", "local") {
		t.Fatal("session readiness survived changed config")
	}
}

func TestSessionTracingRetainsUnrepresentedCalls(t *testing.T) {
	otelConfig(t, settings.OTel{Endpoint: "http://localhost:4318"})
	t.Setenv("MAGPIE_OTEL_SESSIONS", "true")
	e := newOTelExporter()
	previous := otel.Swap(e)
	defer func() { otel.Store(previous); e.cancel() }()
	cfg, err := settings.OTelExport()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	e.sessions.Store(&sessionAvailability{config: cfg, agents: map[string]time.Time{"claude": now, "claude-desktop": now, "codex": now}, seen: map[observedSession]time.Time{{"claude", "parent"}: now, {"claude-desktop", "parent"}: now, {"codex", "parent"}: now}})
	for _, tc := range []struct{ agent, kind string }{
		{"claude", "title_generation"},
		{"claude-desktop", "title_generation"}, {"codex", "thread_title"}, {"codex", "review"},
	} {
		offerOTel(Record{Agent: tc.agent, Kind: tc.kind, Session: "parent", Local: true, Time: now, Status: 200})
		if len(e.queue) != 1 {
			t.Fatalf("%s/%s unrepresented request lost", tc.agent, tc.kind)
		}
		<-e.queue
	}
}
