package sessions

import (
	"fmt"
	"testing"
	"time"
)

func TestTraceSessionIndexSharesMissesAndDoesNotQueueRequests(t *testing.T) {
	x := new(TraceSessionIndex)
	empty := x.update(nil)
	for i := 0; i < 100; i++ {
		if x.Visible("codex", fmt.Sprintf("remote-%d", i)) {
			t.Fatal("unknown session visible")
		}
	}
	if x.snapshot.Load() != empty {
		t.Fatal("each unknown session repeated discovery")
	}
	x.snapshot.Store(&traceIdentitySnapshot{at: time.Now().Add(-time.Minute), sessions: map[TraceSession]bool{{"codex", "known"}: true}})
	x.refresh.Lock()
	done := make(chan bool, 1)
	go func() { done <- x.Visible("codex", "known") }()
	select {
	case visible := <-done:
		x.refresh.Unlock()
		if !visible {
			t.Fatal("concurrent refresh discarded existing readiness")
		}
	case <-time.After(time.Second):
		x.refresh.Unlock()
		<-done
		t.Fatal("request queued behind directory discovery")
	}
}

func TestTraceClaudeChildrenCannotEvictMainStores(t *testing.T) {
	now := time.Now()
	files := []file{{agent: "codex", path: "codex", main: true, mod: now.Add(-time.Hour)}, {agent: "pi", path: "pi", main: true, mod: now.Add(-time.Hour)}}
	for i := 0; i < Limit+30; i++ {
		files = append(files, file{agent: "claude", path: fmt.Sprint(i), mod: now.Add(time.Duration(i) * time.Second)})
	}
	recent := traceRecentFiles(files)
	if len(recent) != Limit+2 {
		t.Fatalf("quota: %d", len(recent))
	}
	if recent[len(recent)-2].agent != "codex" && recent[len(recent)-1].agent != "codex" {
		t.Fatal("Claude children evicted Codex")
	}
	if recent[len(recent)-2].agent != "pi" && recent[len(recent)-1].agent != "pi" {
		t.Fatal("Claude children evicted Pi")
	}
	for i := 1; i < len(recent); i++ {
		if recent[i].mod.After(recent[i-1].mod) {
			t.Fatal("recency order lost")
		}
	}
}
