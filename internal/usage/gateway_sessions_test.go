package usage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
)

func resetUsageTest(t *testing.T) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	_ = provider.Path()
}

func TestGatewaySessionsRequiresExplicitIdentityAndKeepsModels(t *testing.T) {
	resetUsageTest(t)
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	Append(Record{Time: at, Agent: "codex", Session: "s-1", Provider: "p", Model: "m1", Input: 10, Output: 2, Status: 200})
	Append(Record{Time: at.Add(time.Minute), Agent: "codex", Session: "s-1", Provider: "p", Model: "m2", Input: 3, Output: 4, Status: 200})
	Append(Record{Time: at, Agent: "codex", Provider: "p", Model: "m1", Input: 999, Output: 999, Status: 200})
	got := GatewaySessions(time.Time{}, nil)
	if len(got) != 1 || got[0].ID != "s-1" || len(got[0].Models) != 2 || got[0].Input != 13 || got[0].Output != 6 {
		t.Fatalf("sessions=%+v", got)
	}
}

func TestGatewaySessionsCacheRefreshAndOwnership(t *testing.T) {
	resetUsageTest(t)
	at := time.Now()
	Append(Record{Time: at, Agent: "pi", Session: "native", Provider: "p", Model: "m", Input: 10, Status: 200})
	first := GatewaySessions(time.Time{}, nil)
	if len(first) != 1 {
		t.Fatalf("first: %+v", first)
	}
	first[0].Models[0].Model = "changed by caller"
	first[0].Daily[0].Input = 999
	again := GatewaySessions(time.Time{}, nil)
	if again[0].Models[0].Model != "m" || again[0].Daily[0].Input != 10 {
		t.Fatalf("caller changed cache: %+v", again)
	}
	if got := GatewaySessions(time.Time{}, map[string]bool{"pi|native": true}); len(got) != 0 {
		t.Fatalf("native exclusion lost: %+v", got)
	}
	Append(Record{Time: at.Add(time.Second), Agent: "pi", Session: "native", Provider: "p", Model: "m", Input: 5, Status: 200})
	if got, ok := GatewaySessionByID("pi", "native", nil); !ok || got.Input != 15 {
		t.Fatalf("append not reflected: %+v %v", got, ok)
	}
}

// fogway makes up no session ids, so one starting "request-" is the
// client's own and is counted like any other (#1355).
func TestClientRequestPrefixedSessionsCount(t *testing.T) {
	resetUsageTest(t)
	Append(Record{Time: Clock(), Agent: "pi", Session: "request-mine", Provider: "p", Model: "m", Input: 10, Status: 200})
	summary := Summarize(All)
	if len(summary.Sessions) != 1 || summary.Sessions[0].ID != "request-mine" {
		t.Fatalf("the client's session is missing from Usage: %+v", summary.Sessions)
	}
	if got := Vias(time.Time{}); len(got["pi|request-mine"]) != 1 {
		t.Fatalf("the client's session is missing from Routing: %+v", got)
	}
	if _, ok := GatewaySessionByID("pi", "request-mine", nil); !ok {
		t.Fatal("the client's session is missing from Sessions")
	}
}

func TestGatewaySessionsNativeIdentityWins(t *testing.T) {
	resetUsageTest(t)
	Append(Record{Time: time.Now(), Agent: "pi", Session: "native", Provider: "p", Model: "m", Input: 1, Status: 200})
	if got := GatewaySessions(time.Time{}, map[string]bool{"pi|native": true}); len(got) != 0 {
		t.Fatalf("got duplicate gateway session: %+v", got)
	}
}
