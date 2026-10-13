package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeDesktopCopyOfMessageIsCountedOnce(t *testing.T) {
	d := setupCalls(t)
	line := claudeMsg("shared-message", "claude-opus-5-5", 10, 2, 30, 4, 1)
	writeLines(t, filepath.Join(d.claude, "projects", "fixture", "session.jsonl"), line)
	writeLines(t, filepath.Join(d.desktop, "local-agent-mode-sessions", "org", "workspace", "local_fixture", ".claude", "projects", "fixture", "session.jsonl"), line)
	if cs := Calls(time.Time{}); len(cs) != 1 {
		t.Fatalf("CLI/Desktop copies both counted: %+v", cs)
	}
}

func TestEvictedClaudeRevisionsReleasedAndRebuilt(t *testing.T) {
	d := setupCalls(t)
	data, err := os.ReadFile("testdata/claude-native-replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	path := filepath.Join(d.claude, "projects", "fixture", "session.jsonl")
	writeLines(t, path, lines[:2]...)
	old := time.Now().Add(-time.Hour)
	if err = os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	// Eight larger sources displace this window even when it is idle. Size,
	// not an idle timer, determines which bounded windows are worth keeping.
	for i := 0; i < revisionFiles; i++ {
		p := filepath.Join(filepath.Dir(path), fmt.Sprintf("larger-%d.jsonl", i))
		writeLines(t, p, string(claudeUsageLine(time.Now(), fmt.Sprint(i), "", "m", Tokens{Output: 1}, `[{"type":"text","text":"`+strings.Repeat("x", 8192)+`"}]`, true)))
	}

	List(0)
	Calls(time.Time{})
	if cache[path].Claude != nil || callCache[path].Claude != nil {
		t.Fatal("evicted Claude retained message indexes")
	}
	for _, source := range CallSources() {
		ReadCallSource(source)
	}
	if callContinuations[path].state.Claude != nil {
		t.Fatal("evicted Claude retained continuation")
	}
	appendText(t, path, lines[2]+"\n")
	want := Tokens{10338, 37363, 281066, 63398, 0}
	List(0)
	var got Tokens
	for _, v := range cache[path].Models {
		got.add(v)
	}
	if got != want {
		t.Fatal("reopened summary counted replay", got)
	}
	n := 0
	for _, c := range Calls(time.Time{}) {
		if c.File == path {
			n++
		}
	}
	if n != 2 {
		t.Fatal("reopened calls counted replay", n)
	}
	mu.Lock()
	trimSummaryRevisions(time.Now().Add(revisionIdle + time.Second))
	mu.Unlock()
	callsMu.Lock()
	trimCallRevisions(time.Now().Add(revisionIdle + time.Second))
	callsMu.Unlock()
	if cache[path].Claude != nil || callCache[path].Claude != nil {
		t.Fatal("cheaper source displaced larger windows")
	}
}
