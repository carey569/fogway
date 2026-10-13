package sessions

import (
	"fmt"
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
