package usage

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/settings"
)

// The ledger lists a period's calls newest first, each with the model
// asked for, sent and served, marked when another answered, and its cost
// at list price; it filters by agent, failure and text; a record written
// before the requested and served models were kept still loads; the CSV
// has one row a call.
func TestLedger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	// a maker's API list price for sol: $2 in, $8 out, $0.5 a
	// cached read, $2.5 a cache write, per million
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"anthropic":{"id":"anthropic","models":{"sol":{"id":"sol","cost":{"input":2,"output":8,"cache_read":0.5,"cache_write":2.5}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "https://relay.example/v1", Catalog: "vendor"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := `{"t":"` + now.Add(-3*time.Hour).Format(time.RFC3339Nano) + `","agent":"claude","provider":"relay","host":"relay.example","model":"sol","in":1000,"out":100,"ms":900,"status":200}`
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	if err := os.WriteFile(Path(), []byte(old+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	Append(Record{Time: now.Add(-2 * time.Hour), Agent: "codex", Provider: "relay", Host: "relay.example", Model: "sol", Requested: "fast", Served: "luna",
		Input: 2000, Output: 500, CacheRead: 4000, CacheWrite: 1000, Effort: "high", Millis: 3200, TTFT: 400, Status: 200, Session: "s1", RouteID: 123})
	Append(Record{Time: now.Add(-1 * time.Hour), Agent: "codex", Provider: "relay", Host: "relay.example", Model: "sol", Requested: "relay/sol", Served: "sol-2026-01-01",
		Input: 10, Output: 1, Millis: 100, Status: 200})
	Append(Record{Time: now.Add(-30 * time.Minute), Agent: "codex", Provider: "relay", Host: "relay.example", Model: "sol", Requested: "fast", Millis: 50, Status: 429})
	Append(Record{Time: now.Add(-40 * 24 * time.Hour), Agent: "codex", Provider: "relay", Model: "sol", Input: 1}) // before the month

	rows, sum, agents := Ledger(Month, Filter{})
	if len(rows) != 4 || sum.Calls != 4 || sum.Errors != 1 || strings.Join(agents, ",") != "claude,codex" {
		t.Fatalf("rows %d, %+v, agents %v", len(rows), sum, agents)
	}
	if rows[0].Status != 429 || rows[3].Agent != "claude" {
		t.Fatalf("not newest first: %+v", rows)
	}
	sw := rows[2]
	if sw.RouteID != 123 || sw.Requested != "fast" || sw.Model != "sol" || sw.Served != "luna" || !sw.Swapped || !sw.Priced {
		t.Fatalf("swapped row %+v", sw)
	}
	if want := (2000*2 + 500*8 + 4000*0.5 + 1000*2.5) / 1e6; sw.Cost != want {
		t.Fatalf("cost %v, want %v", sw.Cost, want)
	}
	if rows[1].Swapped || rows[1].Served != "sol-2026-01-01" {
		t.Fatalf("a dated name is the same model: %+v", rows[1])
	}
	if rows[3].RouteID != 0 || rows[3].Requested != "" || rows[3].Served != "" || rows[3].Input != 1000 || !rows[3].Priced {
		t.Fatalf("old record %+v", rows[3])
	}
	if rows[0].Priced || rows[0].Cost != 0 {
		t.Fatalf("a call with no tokens has no cost: %+v", rows[0])
	}

	if rows, _, _ := Ledger(Month, Filter{Agent: "claude"}); len(rows) != 1 || rows[0].Agent != "claude" {
		t.Fatalf("agent filter: %+v", rows)
	}
	if rows, s, _ := Ledger(Month, Filter{Failed: true}); len(rows) != 1 || rows[0].Status != 429 || s.Calls != 1 {
		t.Fatalf("failed filter: %+v", rows)
	}
	if rows, _, _ := Ledger(Month, Filter{Query: "LUNA"}); len(rows) != 1 || rows[0].Served != "luna" {
		t.Fatalf("query filter: %+v", rows)
	}
	if rows, _, _ := Ledger(All, Filter{}); len(rows) != 5 {
		t.Fatalf("all: %d", len(rows))
	}

	var b strings.Builder
	if err := WriteCSV(&b, rows[1:3]); err != nil {
		t.Fatal(err)
	}
	want := strings.Join(CSVHeader, ",") + "\n" +
		rows[1].Time.Format(time.RFC3339) + ",codex,relay/sol,relay,relay.example,sol,sol-2026-01-01,false,,10,1,0,0,0,0.000028,100,,200,false,,,,,,,,,,,,false,,,false,,,,,0,0,,false,,,,\n" +
		rows[2].Time.Format(time.RFC3339) + ",codex,fast,relay,relay.example,sol,luna,true,high,2000,500,1000,4000,0,0.012500,3200,400,200,false,s1,,,,,123,,,,,,false,,,false,,,,,1000,0,,false,,,,\n"
	if b.String() != want {
		t.Fatalf("csv:\n%s\nwant:\n%s", b.String(), want)
	}
}

// Calls use the provider catalog when available, falling back to the maker
// for subscriptions and unknown routes. A reseller-only model is still priced
// for that reseller, while a model absent from both catalogs stays unpriced.
func TestSubscriptionListPrice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{
	  "openai":{"id":"openai","models":{
	    "gpt-6-astra":{"id":"gpt-6-astra","cost":{"input":10,"output":50,"cache_read":1,"cache_write":12.5}},
	    "gpt-6-luna":{"id":"gpt-6-luna","cost":{"input":0.1,"output":0.5,"cache_read":0.01,"cache_write":0.125}}}},
	  "google":{"id":"google","models":{
	    "gemini-3.8-flash":{"id":"gemini-3.8-flash","cost":{"input":0.75,"output":3.75,"cache_read":0.075}}}},
	  "anthropic":{"id":"anthropic","models":{
	    "claude-opus-5-5":{"id":"claude-opus-5-5","cost":{"input":4,"output":20,"cache_read":0.2,"cache_write":5}}}},
	  "discount":{"id":"discount","models":{
	    "gpt-6-astra":{"id":"gpt-6-astra","cost":{"input":1,"output":2,"cache_read":0.01}},
	    "mystery-1":{"id":"mystery-1","cost":{"input":99,"output":99}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "https://relay.example/v1", Catalog: "discount"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	calls := []struct {
		prov, model string
		want        float64
	}{
		{"codex", "gpt-6-astra", (1e6*10 + 1e5*50 + 1e6*1) / 1e6},
		{"codex", "gpt-6-luna", (1e6*0.1 + 1e5*0.5 + 1e6*0.01) / 1e6},
		{"copilot", "gemini-3.8-flash", (1e6*0.75 + 1e5*3.75 + 1e6*0.075) / 1e6},
		{"copilot", "gpt-6-luna", (1e6*0.1 + 1e5*0.5 + 1e6*0.01) / 1e6},
		{"claude", "claude-opus-5-5", (1e6*4 + 1e5*20 + 1e6*0.2) / 1e6},
		{"relay", "openai/gpt-6-astra", (1e6*1 + 1e5*2 + 1e6*0.01) / 1e6},
		{"relay", "gpt-6-astra", (1e6*1 + 1e5*2 + 1e6*0.01) / 1e6},
		{UnknownProvider, "gpt-6-astra", (1e6*10 + 1e5*50 + 1e6*1) / 1e6},
		{"relay", "mystery-1", (1e6*99 + 1e5*99) / 1e6},
		{UnknownProvider, "mystery-1", 0},
	}
	for i, c := range calls {
		Append(Record{Time: now.Add(-time.Duration(len(calls)-i) * time.Minute), Agent: "codex", Provider: c.prov, Model: c.model,
			Input: 1_000_000, Output: 100_000, CacheRead: 1_000_000, Status: 200})
	}
	rows, sum, _ := Ledger(Month, Filter{})
	if len(rows) != len(calls) {
		t.Fatalf("rows %d", len(rows))
	}
	total := 0.0
	for i, c := range calls {
		r := rows[len(calls)-1-i]
		if r.Model != c.model || r.Priced != (c.want > 0) || math.Abs(r.Cost-c.want) > 1e-9 {
			t.Errorf("%s/%s: priced=%v cost=%v, want %v", c.prov, c.model, r.Priced, r.Cost, c.want)
		}
		total += c.want
	}
	s := Summarize(Month)
	if s.Unpriced != 1 || math.Abs(s.Cost-total) > 1e-9 || math.Abs(sum.Cost-total) > 1e-9 {
		t.Fatalf("summary cost %v unpriced %d, ledger %v; want %v and 1", s.Cost, s.Unpriced, sum.Cost, total)
	}
	for _, m := range s.Models {
		if m.Model != "mystery-1" && (m.Unpriced != 0 || m.Cost == 0) {
			t.Errorf("model %s unpriced: %+v", m.ID, m.Totals)
		}
	}
}

// The ledger adds the calls an agent's session file records that the
// gateway didn't log — a Claude Desktop turn on its own sign-in, say — as
// rows marked "log", priced by their model; leaves out those the gateway
// did log (the same session, a request that spans the file's time); counts
// and filters a failure the file tells, which has no status; and the
// failures the gateway logged keep what the vendor said.
func TestLedgerWithSessionLogCalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	// a models.dev catalog pricing Claude Sonnet 5: $3 in, $15 out, $0.3 a cached read, $3.75 a cache write, per million
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"anthropic":{"id":"anthropic","models":{"claude-sonnet-5":{"id":"claude-sonnet-5","cost":{"input":3,"output":15,"cache_read":0.3,"cache_write":3.75}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)

	// noon of the day Clock is held at, so that what the test lists a few
	// hours before it is that day's
	now := holdClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	// the gateway's: an answered request of session s1 that began at -10m and
	// took 20 s, and a failure of another with what the vendor said
	Append(Record{Time: now.Add(-10 * time.Minute), Agent: "claude", Provider: "claude", Model: "claude-sonnet-5", Input: 5, Output: 5,
		Millis: 20000, Status: 200, Session: "s1", RequestID: "req_gw", Endpoint: "/v1/messages"})
	Append(Record{Time: now.Add(-5 * time.Minute), Agent: "codex", Provider: "relay", Model: "sol", Millis: 40, Status: 429, Session: "s9",
		Error: "Relay: slow down", ErrType: "rate_limit_error", RequestID: "req_bad", Endpoint: "/v1/responses"})

	calls := []sessions.Call{
		// in s1, 8 s into the request the gateway logged: the same call
		{Time: now.Add(-10*time.Minute + 8*time.Second), Agent: "claude", Session: "s1", Model: "claude-sonnet-5", RequestID: "req_gw", Tokens: sessions.Tokens{Input: 5, Output: 5}},
		// in s1 an hour on: a call the gateway didn't see
		{Time: now.Add(-1 * time.Hour), Agent: "claude-desktop", Session: "s1", Model: "claude-sonnet-5", RequestID: "req_log",
			Tokens: sessions.Tokens{Input: 1000, Output: 200, CacheRead: 4000, CacheWrite: 500}},
		// a failure the file tells
		{Time: now.Add(-2 * time.Hour), Agent: "claude-desktop", Session: "s2", Error: "rate_limit", ErrorText: "You've hit your limit", RequestID: "req_lim"},
		// a Codex turn of a session the gateway never saw, a month ago
		{Time: now.Add(-20 * 24 * time.Hour), Agent: "codex", Session: "s3", Model: "gpt-6-luna", Tokens: sessions.Tokens{Input: 10, Output: 2}, TTFT: 700, Millis: 2100},
	}
	old := LogCalls
	LogCalls = func(time.Time) []sessions.Call { return calls }
	t.Cleanup(func() { LogCalls = old })

	rows, sum, agents := Ledger(Month, Filter{})
	if len(rows) != 5 || sum.Calls != 5 || sum.Errors != 2 || strings.Join(agents, ",") != "claude,claude-desktop,codex" {
		t.Fatalf("rows %d, %+v, agents %v", len(rows), sum, agents)
	}
	// newest first, the two logs interleaved by time
	var order []string
	for _, r := range rows {
		order = append(order, r.RequestID)
	}
	if strings.Join(order, ",") != "req_bad,req_gw,req_log,req_lim," {
		t.Fatalf("order %v", order)
	}
	if rows[0].Source != "" || rows[0].Error != "Relay: slow down" || rows[0].ErrType != "rate_limit_error" || rows[0].Endpoint != "/v1/responses" || !rows[0].Failed() {
		t.Fatalf("gateway failure %+v", rows[0])
	}
	lg := rows[2]
	if lg.Source != "log" || lg.Status != 0 || lg.Agent != "claude-desktop" || lg.Provider != UnknownProvider || !lg.Priced || lg.Failed() {
		t.Fatalf("logged call %+v", lg)
	}
	if want := (1000*3 + 200*15 + 4000*0.3 + 500*3.75) / 1e6; lg.Cost != want {
		t.Fatalf("cost %v, want %v", lg.Cost, want)
	}
	// Claude Code's file names the model the API answered with; a swap is
	// told against what was asked for, which it doesn't say
	if lg.Served != "claude-sonnet-5" || lg.Requested != "" || lg.Swapped {
		t.Fatalf("Claude's model is the served one: %+v", lg)
	}
	lim := rows[3]
	if lim.Source != "log" || !lim.Failed() || lim.Error != "You've hit your limit" || lim.ErrType != "rate_limit" || lim.Priced {
		t.Fatalf("failure the file tells %+v", lim)
	}
	// Codex's file names the model it asked for, and none that answered
	if cx := rows[4]; cx.Requested != "gpt-6-luna" || cx.Served != "" || cx.Swapped {
		t.Fatalf("Codex's model is the requested one: %+v", cx)
	}
	if cx := rows[4]; cx.Provider != UnknownProvider || cx.TTFT != 700 || cx.Millis != 2100 || cx.Priced {
		t.Fatalf("codex call %+v", cx)
	}

	if rows, s, _ := Ledger(Month, Filter{Failed: true}); len(rows) != 2 || s.Calls != 2 || s.Errors != 2 {
		t.Fatalf("failed filter: %d rows, %+v", len(rows), s)
	}
	if rows, _, _ := Ledger(Month, Filter{Agent: "claude-desktop"}); len(rows) != 2 {
		t.Fatalf("agent filter: %+v", rows)
	}
	if rows, _, _ := Ledger(Today, Filter{}); len(rows) != 4 {
		t.Fatalf("today: %d", len(rows)) // the month-old Codex turn is out of it
	}
	var b strings.Builder
	if err := WriteCSV(&b, rows[2:4]); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{",req_log,,,,log,false,,,false,,,,,500,0,,false,,,,\n", ",true,s2,,,,,,req_lim,,You've hit your limit,rate_limit,log,false,,,false,,,,,0,0,,false,,,,\n"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("csv lacks %q:\n%s", want, b.String())
		}
	}
}

// A call is counted at the price the user set for that provider and model,
// all four token tiers included: the ledger is where a wrong price becomes a
// wrong number, and a tier left at the catalogue's price would not show in
// the input and output columns alone.
func TestLedgerUsesTheStatedPrice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"openai":{"id":"openai","models":{"sol":{"id":"sol",`+
		`"cost":{"input":2,"output":8,"cache_read":0.5,"cache_write":2.5}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "https://relay.example/v1"}); err != nil {
		t.Fatal(err)
	}
	// all four parts distinct from the catalogue's, so a tier counted at the
	// catalogue's price instead would move the total
	if err := settings.Save(settings.Settings{ModelPrices: map[string]settings.ModelPrice{
		"relay/sol": {Input: new(float64(0.2)), Output: new(float64(1)),
			CacheRead: new(float64(0.05)), CacheWrite: new(float64(0.25))},
	}}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	Append(Record{Time: time.Now(), Agent: "codex", Provider: "relay", Host: "relay.example",
		Model: "sol", Input: 1000, Output: 100, CacheRead: 2000, CacheWrite: 400, Status: 200})

	rows, sum, _ := Ledger(Month, Filter{})
	if len(rows) != 1 || !rows[0].Priced {
		t.Fatalf("rows %+v", rows)
	}
	// (1000*0.2 + 100*1 + 2000*0.05 + 400*0.25) / 1e6
	const want = 0.0005
	if math.Abs(rows[0].Cost-want) > 1e-12 || math.Abs(sum.Cost-want) > 1e-12 {
		t.Fatalf("row cost %v, total %v; want %v", rows[0].Cost, sum.Cost, want)
	}
}

// Overrides are isolated by provider and refresh cached pages when changed.
func TestLedgerConfiguredPricesAndRefresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"openai":{"models":{"sol":{"id":"sol","cost":{"input":2,"output":8}}}},"discount":{"models":{"sol":{"id":"sol","cost":{"input":1,"output":2}}}}}`), 0o644)
	price := func(v float64) settings.ModelPrice {
		return settings.ModelPrice{Input: new(v), Output: new(v), CacheRead: new(v), CacheWrite: new(v)}
	}
	if err := provider.Save(provider.Provider{ID: "b", Name: "Relay", Key: "k", Chat: "https://relay.example/v1", Catalog: "discount"}); err != nil {
		t.Fatal(err)
	}
	cfg := settings.Settings{ModelPrices: map[string]settings.ModelPrice{"a/sol": price(0), "a/*": price(9), "b/*": price(3)}}
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	for _, id := range []string{"a", "b", "unknown"} {
		Append(Record{Time: time.Now(), Provider: id, Model: "sol", Input: 1000000, Status: 200})
	}
	check := func(want map[string]float64) {
		t.Helper()
		rows, sum, _ := Ledger(Month, Filter{})
		page := QueryPage(Month, Filter{}, 0, 20)
		for _, got := range [][]Row{rows, page.Rows} {
			if len(got) != 3 {
				t.Fatalf("rows: %+v", got)
			}
			for _, r := range got {
				if !r.Priced || r.Cost != want[r.Provider] {
					t.Fatalf("%s priced=%v cost=%v want=%v", r.Provider, r.Priced, r.Cost, want[r.Provider])
				}
			}
		}
		total := want["a"] + want["b"] + want["unknown"]
		if sum.Cost != total || page.Sum.Cost != total {
			t.Fatalf("totals: ledger=%v page=%v want=%v", sum.Cost, page.Sum.Cost, total)
		}
	}
	check(map[string]float64{"a": 0, "b": 3, "unknown": 2})
	cfg.ModelPrices["a/sol"] = price(4)
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	check(map[string]float64{"a": 4, "b": 3, "unknown": 2})
	delete(cfg.ModelPrices, "a/sol")
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	check(map[string]float64{"a": 9, "b": 3, "unknown": 2})
	delete(cfg.ModelPrices, "a/*")
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	check(map[string]float64{"a": 2, "b": 3, "unknown": 2})
	delete(cfg.ModelPrices, "b/*")
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	// Removing an override falls back to the relay's catalog, not the maker.
	check(map[string]float64{"a": 2, "b": 1, "unknown": 2})
}

// A relay that sells a model under a name of its own (moonshot-kimi-k3)
// answers with the model's own name (kimi-k3), which the record keeps as
// served: a call no price is known for by the name asked is priced as the
// one that answered, at the same provider (#1498, liuweifeng). A model
// with a price of its own keeps it, a zero the user set included, however
// its reply names itself.
func TestLedgerPricesARenamedModelAsTheOneServed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"moonshotai":{"id":"moonshotai","models":{"kimi-k3":{"id":"kimi-k3",`+
		`"cost":{"input":3,"output":15,"cache_read":0.3,"cache_write":3}}}},`+
		`"openai":{"id":"openai","models":{"sol":{"id":"sol","cost":{"input":2,"output":8}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay-a", Name: "Relay A", Key: "k", Chat: "https://relay.example/v1"}); err != nil {
		t.Fatal(err)
	}
	free := new(float64(0))
	if err := settings.Save(settings.Settings{ModelPrices: map[string]settings.ModelPrice{
		"relay-a/house-free": {Input: free, Output: free, CacheRead: free, CacheWrite: free},
	}}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	now := time.Now()
	// the reporter's record, as the gateway wrote it
	Append(Record{Time: now, Provider: "relay-a", Model: "moonshot-kimi-k3", Requested: "relay-a/moonshot-kimi-k3",
		Served: "kimi-k3", Input: 74, Output: 227, CacheRead: 288256, CacheWrite: 512, Status: 200})
	// priced by its own name, answered as kimi-k3 all the same
	Append(Record{Time: now.Add(-time.Second), Provider: "relay-a", Model: "sol", Served: "kimi-k3", Input: 1000000, Status: 200})
	// set free by the user
	Append(Record{Time: now.Add(-2 * time.Second), Provider: "relay-a", Model: "house-free", Served: "kimi-k3", Input: 1000000, Status: 200})
	// no price by either name
	Append(Record{Time: now.Add(-3 * time.Second), Provider: "relay-a", Model: "mystery", Served: "mystery-2", Input: 1000000, Status: 200})

	rows, _, _ := Ledger(Month, Filter{})
	page := QueryPage(Month, Filter{}, 0, 20)
	for _, got := range [][]Row{rows, page.Rows} {
		if len(got) != 4 {
			t.Fatalf("rows: %+v", got)
		}
		cost := map[string]float64{}
		priced := map[string]bool{}
		for _, r := range got {
			cost[r.Model], priced[r.Model] = r.Cost, r.Priced
		}
		// (74*3 + 227*15 + 288256*0.3 + 512*3) / 1e6
		if want := 0.0916398; !priced["moonshot-kimi-k3"] || math.Abs(cost["moonshot-kimi-k3"]-want) > 1e-9 {
			t.Fatalf("renamed: priced=%v cost=%v, want %v", priced["moonshot-kimi-k3"], cost["moonshot-kimi-k3"], want)
		}
		if !priced["sol"] || cost["sol"] != 2 {
			t.Fatalf("sol: priced=%v cost=%v, want its own 2", priced["sol"], cost["sol"])
		}
		if !priced["house-free"] || cost["house-free"] != 0 {
			t.Fatalf("house-free: priced=%v cost=%v, want the user's 0", priced["house-free"], cost["house-free"])
		}
		if priced["mystery"] {
			t.Fatalf("mystery priced at %v", cost["mystery"])
		}
	}
}

// A model models.dev lists only under another id is priced as that entry,
// by its canonical_model_id, though the reply names it as asked and so
// gives no served name to fall back on (#1520, liuweifeng): WorkBuddy's
// deepseek-v4.1-flash is DeepSeek's deepseek-flash. The entries are
// models.dev's bytes of 2026-10-10.
func TestLedgerPricesAModelListedUnderItsCanonicalID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{"deepseek":{"id":"deepseek","models":{"deepseek-flash":{"id":"deepseek-flash","name":"DeepSeek V4.1 Flash","description":"DeepSeek V4.1 Flash model for reasoning and agentic coding","family":"deepseek-flash","attachment":true,"reasoning":true,"reasoning_options":[{"type":"toggle"},{"type":"effort","values":["low","high","max"]}],"tool_call":true,"interleaved":{"field":"reasoning_content"},"structured_output":true,"temperature":true,"knowledge":"2025-05","release_date":"2026-09-10","last_updated":"2026-09-10","modalities":{"input":["text","image"],"output":["text"]},"open_weights":true,"limit":{"context":1000000,"output":393216},"cost":{"input":0.15,"output":0.6,"reasoning":0.6,"cache_read":0.003},"canonical_model_id":"deepseek/deepseek-v4.1-flash"},
"deepseek-v4-pro":{"id":"deepseek-v4-pro","name":"DeepSeek V4 Pro","description":"DeepSeek V4 Pro snapshot with million-token context and support for thinking and non-thinking modes","family":"deepseek-thinking","attachment":false,"reasoning":true,"reasoning_options":[{"type":"toggle"},{"type":"effort","values":["low","high","max"]}],"tool_call":true,"interleaved":{"field":"reasoning_content"},"structured_output":true,"temperature":true,"release_date":"2026-08-12","last_updated":"2026-08-22","modalities":{"input":["text"],"output":["text"]},"open_weights":true,"limit":{"context":1000000,"output":393216},"cost":{"input":0.66,"output":1.98,"reasoning":1.98,"cache_read":0.022},"canonical_model_id":"deepseek/deepseek-v4-pro-0813"}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	for _, m := range []string{"deepseek-v4.1-flash", "deepseek-v4-pro"} {
		if _, ok := provider.EffectivePriceIn(settings.Load(), "workbuddy", m); !ok {
			t.Errorf("workbuddy/%s: no price", m)
		}
	}
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	// the reporter's record, as the gateway wrote it
	Append(Record{Time: time.Now(), Provider: "workbuddy", Model: "deepseek-v4.1-flash", Requested: "workbuddy/deepseek-v4.1-flash",
		Served: "deepseek-v4.1-flash", Input: 73723, Output: 2612, CacheRead: 124288, Status: 200})

	rows, _, _ := Ledger(Month, Filter{})
	page := QueryPage(Month, Filter{}, 0, 20)
	for _, got := range [][]Row{rows, page.Rows} {
		if len(got) != 1 {
			t.Fatalf("rows: %+v", got)
		}
		// (73723*0.15 + 2612*0.6 + 124288*0.003) / 1e6
		if want := 0.012998514; !got[0].Priced || math.Abs(got[0].Cost-want) > 1e-9 {
			t.Fatalf("priced=%v cost=%v, want %v", got[0].Priced, got[0].Cost, want)
		}
	}
}
