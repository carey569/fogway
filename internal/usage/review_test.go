package usage

import (
	"bytes"
	"encoding/csv"
	"slices"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/sessions"
)

func TestGatewayCorrelation(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	r := Record{Time: at, Provider: "relay", Agent: "codex", Session: "override", NativeSession: "native", Input: 10, Output: 5, Millis: 1000, Status: 200}
	c := sessions.Call{Time: at.Add(time.Second), Agent: "codex", Session: "native", Tokens: sessions.Tokens{Input: 10, Output: 5}}
	cases := []struct {
		name string
		recs []Record
		logs []sessions.Call
		want int
	}{
		{"native header despite override", []Record{r}, []sessions.Call{c}, 1},
		{"later direct call stays", []Record{r}, []sessions.Call{c, {Time: at.Add(20 * time.Second), Agent: "codex", Session: "native", Tokens: c.Tokens}}, 1},
		{"different tokens stay", []Record{r}, []sessions.Call{{Time: c.Time, Agent: c.Agent, Session: c.Session, Tokens: sessions.Tokens{Input: 11, Output: 5}}}, 0},
		{"ambiguous file calls stay", []Record{r}, []sessions.Call{c, c}, 0},
		{"ambiguous gateway records stay", []Record{r, r}, []sessions.Call{c}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(gatewayMatches(tc.recs, tc.logs)); got != tc.want {
				t.Fatalf("matched %d, want %d", got, tc.want)
			}
		})
	}
	r.RequestID, c.RequestID = "req_same", "req_same"
	c.Session, c.Time = "other", at.Add(10*time.Second)
	if !gatewayMatches([]Record{r}, []sessions.Call{c})[0] {
		t.Fatal("matching request ID must work independently of session/time")
	}
	c.RequestID = "req_direct"
	if len(gatewayMatches([]Record{r}, []sessions.Call{c})) != 0 {
		t.Fatal("distinct request IDs must stay")
	}
}

func TestZeroTokenFailuresDeduplicateAndRejectedStayVisible(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	at := time.Now()
	r := Record{Time: at, Agent: "claude", Provider: "relay", Model: "m", Session: "s", Status: 429, Error: "rate limited", Millis: 1000}
	c := sessions.Call{Time: at.Add(time.Second), Agent: "claude", Session: "s", Error: "rate_limit"}
	if len(gatewayMatches([]Record{r}, []sessions.Call{c})) != 1 {
		t.Fatal("zero-token failure counted twice")
	}
	if len(gatewayMatches([]Record{r, r}, []sessions.Call{c})) != 0 {
		t.Fatal("ambiguous retries guessed")
	}
	r.Status, r.Error = 200, ""
	if len(gatewayMatches([]Record{r}, []sessions.Call{c})) != 0 {
		t.Fatal("failure matched an empty success")
	}
	r.Status, r.Provider, r.Rejected = 404, "", true
	if len(gatewayMatches([]Record{r}, []sessions.Call{c})) != 0 {
		t.Fatal("local rejection consumed a real API call")
	}
	Append(r)
	if len(Vias(time.Time{})) != 0 {
		t.Fatal("rejected request appears as a session provider")
	}
	Append(Record{Time: at, Agent: "claude", Provider: "relay", Model: "m", Session: "s", Status: 200})
	Append(Record{Time: at, Agent: "claude", Model: "m", Session: "s", Status: 400}) // legacy rejection
	if vias := Vias(time.Time{}); len(vias) != 1 || len(vias["claude|s"]) != 1 || vias["claude|s"][0].Calls != 1 {
		t.Fatalf("session via includes a local rejection: %+v", vias)
	}
	var out bytes.Buffer
	if err := WriteCSV(&out, []Row{{Record: r}}); err != nil {
		t.Fatal(err)
	}
	csvRows, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if col := slices.Index(csvRows[0], "rejected"); col < 0 || csvRows[1][col] != "true" {
		t.Fatal("CSV omits rejection")
	}
}
