package sessions

import (
	"testing"
	"time"
)

func TestCodexUsageLargerFirstRequestAfterLegacyReset(t *testing.T) {
	before := cxUse(91814, 46976, 0, 396, 123)
	after := cxUse(92225, 91008, 0, 885, 516)
	lines := append(cxTurnLines(0, "t1", "gpt-6-astra", "high"), tokenCountLine(1, before, before))
	lines = append(lines, cxTurnLines(2, "t2", "gpt-6-astra", "high")...)
	lines = append(lines, tokenCountLine(3, after, after), tokenCountLine(4, after, after))
	codexUsageLog(t, lines)
	if cs := Calls(time.Time{}); len(cs) != 2 {
		t.Fatalf("new runtime's first request lost or replay counted: %+v", cs)
	}
	if ss := List(0); len(ss) != 1 || ss[0].Tokens != (Tokens{46055, 1281, 137984, 0, 0}) {
		t.Fatalf("summary=%+v", ss)
	}
}
