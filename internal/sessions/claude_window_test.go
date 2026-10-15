package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestClaudeWindowBoundsOneLongMessage(t *testing.T) {
	s := &state{}
	at := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < claudeRevisionEntries*2; i++ {
		content := fmt.Sprintf(`[{"type":"tool_use","id":%q,"name":"Read","input":{}}]`, fmt.Sprint(i))
		claudeLine(s, claudeUsageLine(at.Add(time.Duration(i)*time.Second), "a", "r", "m", Tokens{Output: i + 1}, content, true), true)
		if s.Claude.weight() > claudeRevisionEntries {
			t.Fatal("single message grew beyond window")
		}
	}
	want := Tokens{Output: claudeRevisionEntries * 2}
	if s.Models["m"] != want || claudeUsageReplies(s) != 1 {
		t.Fatal("window reset lost the current contribution")
	}
	copy := s.clone()
	claudeLine(copy, claudeUsageLine(at.Add(time.Hour), "b", "r-b", "m", Tokens{Output: 1}, "", true), true)
	if reflect.DeepEqual(s.Models, copy.Models) || len(s.Claude.Messages) != 1 {
		t.Fatal("clone shares mutable state")
	}
}

func TestClaudeCallsReuseRowsBeyondWindow(t *testing.T) {
	st := &callFile{Agent: "claude"}
	at := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < claudeRevisionEntries; i++ {
		claudeCallLine(st, claudeUsageLine(at.Add(time.Duration(i)*time.Second), fmt.Sprint(i), "req", "m", Tokens{Output: 1}, "", true))
	}
	// The old message is outside the window, but its materialized row is still
	// available. A missing request ID must not duplicate it or erase its ID.
	claudeCallLine(st, claudeUsageLine(at.Add(time.Hour), "0", "", "m", Tokens{Output: 3}, "", true))
	if len(st.Calls) != claudeRevisionEntries || st.Calls[0].Output != 3 || st.Calls[0].RequestID != "req" {
		t.Fatal("old row was duplicated or lost identity")
	}
}

func TestLargeClaudeFileKeepsAppendContinuation(t *testing.T) {
	d := setupCalls(t)
	path := filepath.Join(d.claude, "projects", "fixture", "session.jsonl")
	const count = 24000
	lines := make([]string, count)
	at := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for i := range lines {
		lines[i] = string(claudeUsageLine(at.Add(time.Duration(i)*time.Second), fmt.Sprint(i), "", "m", Tokens{Input: 1, Output: 2}, "", true))
	}
	writeLines(t, path, lines...)
	read := func() []Call {
		t.Helper()
		for _, source := range CallSources() {
			if source.Path == path {
				return ReadCallSource(source)
			}
		}
		t.Fatal("source missing")
		return nil
	}
	live := true
	check := func(n int) {
		t.Helper()
		ss := List(0)
		if len(ss) != 1 || ss[0].Input != n || ss[0].Output != 2*n {
			t.Fatalf("summary after append: %+v", ss)
		}
		cs := read()
		var total Tokens
		for _, c := range cs {
			total.add(c.Tokens)
		}
		if len(cs) != n || total.Input != n || total.Output != 2*n {
			t.Fatalf("calls=%d tokens=%+v", len(cs), total)
		}
		if live && (cache[path].Claude == nil || callContinuations[path].state.Claude == nil) {
			t.Fatal("large active file lost continuation")
		}
		if cache[path].Claude.weight() > claudeRevisionEntries || callContinuations[path].state.Claude.weight() > claudeRevisionEntries {
			t.Fatal("recent window exceeded budget")
		}
	}
	check(count)
	before, err := os.Stat(callCachePath(path))
	if err != nil {
		t.Fatal(err)
	}
	// A cold reparse cannot preserve this in-memory marker. This proves List
	// clones its old summary and resumes at Off instead of replaying the prefix.
	cache[path].Named = "append continuation marker"
	old := cache[path].Claude.clone()
	for i := 0; i < 3; i++ {
		line := string(claudeUsageLine(at.Add(time.Duration(count+i)*time.Second), fmt.Sprint(count+i), "", "m", Tokens{Input: 1, Output: 2}, "", true))
		appendText(t, path, line+"\n")
		check(count + i + 1)
		if cache[path].Named != "append continuation marker" {
			t.Fatal("summary reparsed the prefix")
		}
		after, err := os.Stat(callCachePath(path))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) {
			t.Fatal("call append rebuilt the shard")
		}
	}
	// Recent A/B/A replays still reconcile after the file has outgrown the
	// budget. Replaying the previous final message must not add a call.
	appendText(t, path, lines[count-1]+"\n")
	check(count + 3)
	if old.Messages["message:23999"][""].Usage.Tokens.Output != 2 {
		t.Fatal("mutated published revision snapshot")
	}
	Reset()
	live = false
	check(count + 3)
}
