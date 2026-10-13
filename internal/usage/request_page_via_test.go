package usage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/sessions"
)

// viaAll builds a ledger with two gateway rows and three session-file calls,
// as the requests page's two source cells count them, with the packed chunks
// the streaming path reads so both paths can be compared.
func viaAll(t *testing.T) (Ledgered, *rowChunk, []*rowChunk) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	at := time.Now().Add(-time.Hour)
	recs := []Record{
		{Time: at, Agent: "codex", Provider: "relay", Model: "gpt-6-sol", Input: 100, Output: 10, Status: 200},
		{Time: at.Add(time.Minute), Agent: "codex", Provider: "relay", Model: "gpt-6-sol", Input: 200, Output: 20, Status: 200},
		// a call the gateway turned away itself: it is one of the period's
		// rows but neither source's, as the page says under its totals
		// ("local rejections excluded"), so the two cells are the calls that
		// were answered and not every row the period holds
		{Time: at.Add(30 * time.Second), Agent: "codex", Provider: "relay", Model: "gpt-6-sol", Rejected: true, Status: 400},
	}
	logs := []sessions.Call{
		{Time: at.Add(2 * time.Minute), Agent: "dsh", Session: "s", Model: "deepseek-v4", Tokens: sessions.Tokens{Input: 300, Output: 30}},
		{Time: at.Add(3 * time.Minute), Agent: "dsh", Session: "s", Model: "deepseek-v4", Tokens: sessions.Tokens{Input: 400, Output: 40}},
		{Time: at.Add(4 * time.Minute), Agent: "dsh", Session: "s", Model: "deepseek-v4", Tokens: sessions.Tokens{Input: 500, Output: 50}},
	}
	rows, sum, agents, providers := ledgerWith(Period(All).Since(time.Now()), Filter{}, recs, logs)
	gateway := &rowChunk{}
	for i, r := range recs {
		r.Agent = AgentOf(r.Agent)
		gateway.add(Row{Record: r}, "", int64(i), false)
	}
	local := &rowChunk{}
	for i, c := range logs {
		local.add(Row{Record: logRecord(c), Source: "log"}, "", int64(i), false)
	}
	return Ledgered{rows, sum, agents, providers}, gateway, []*rowChunk{local}
}
