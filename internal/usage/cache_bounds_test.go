package usage

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func holdLogTick(t *testing.T, tick time.Time) {
	t.Helper()
	changeTime, now := logChangeTime, logIndexNow
	logChangeTime = func(info os.FileInfo) time.Time {
		if info == nil {
			return time.Time{}
		}
		return tick
	}
	logIndexNow = func() time.Time { return tick.Add(10 * time.Millisecond) }
	t.Cleanup(func() { logChangeTime, logIndexNow = changeTime, now })
}

func cacheBudget(t *testing.T, n int64) {
	t.Helper()
	old := requestCacheBytes
	requestCacheBytes = n
	t.Cleanup(func() { requestCacheBytes = old })
}

func TestUsageLargeSummaryIsCompleteAndKeptTrimmed(t *testing.T) {
	pageHome(t)
	for i := 0; i < summaryCacheSessions+1; i++ {
		Append(Record{Time: time.Now(), Agent: "codex", Provider: "relay", Model: "m", Session: fmt.Sprint(i), Input: 1 + i})
	}
	worked := func() int {
		summaries.Lock()
		defer summaries.Unlock()
		return summaries.worked
	}
	got := Summarize(All)
	if len(got.Sessions) != summaryCacheSessions+1 || got.Calls != summaryCacheSessions+1 || got.SessionCount != summaryCacheSessions+1 {
		t.Fatalf("large summary lost sessions: %d / %d", len(got.Sessions), got.Calls)
	}
	summaries.Lock()
	kept := summaries.entries[All]
	summaries.Unlock()
	if len(kept.value.Sessions) != summaryCacheSessions || kept.value.SessionCount != summaryCacheSessions+1 {
		t.Fatalf("the cache kept %d sessions of %d", len(kept.value.Sessions), kept.value.SessionCount)
	}
	// the Usage page and the TUI ask for the top few, and a long history's
	// summary is not worked out again for them (Vantiboo1ean, ~750k
	// requests in ~84k sessions); asked for all, it is
	before := worked()
	for _, n := range []int{0, 100, summaryCacheSessions} {
		top := SummarizeTop(All, n)
		if len(top.Sessions) != n || top.SessionCount != summaryCacheSessions+1 || top.Calls != got.Calls || n > 0 && top.Sessions[0] != got.Sessions[0] {
			t.Fatalf("top %d: %d sessions of %d", n, len(top.Sessions), top.SessionCount)
		}
	}
	if s, _ := Summaries(All); len(s.Sessions) != summaryCacheSessions || s.SessionCount != summaryCacheSessions+1 {
		t.Fatalf("Summaries: %d sessions of %d", len(s.Sessions), s.SessionCount)
	}
	if n := worked() - before; n != 0 {
		t.Fatalf("a summary of the top sessions was worked out %d more times", n)
	}
	if len(Summarize(All).Sessions) != summaryCacheSessions+1 || worked() != before+1 {
		t.Fatal("asked for every session, the summary is not worked out afresh")
	}
	// what a caller is handed is its own
	top := SummarizeTop(All, 10)
	top.Sessions[0].Calls, top.Agents[0].Calls = -1, -1
	if again := SummarizeTop(All, 10); again.Sessions[0].Calls == -1 || again.Agents[0].Calls == -1 {
		t.Fatal("a caller's change reached the cache")
	}
}
