package sessions

import (
	"testing"
	"time"
)

func TestCodexRecordPreservesMetadataSession(t *testing.T) {
	lines := append(cxTurnLines(0, "t1", "gpt-6-astra", "high"), cxRecordLine(1, "a", cxUse(100, 0, 0, 10, 0), cxUse(100, 0, 0, 10, 0)))
	lines[0] = `{"timestamp":"` + stamp(0) + `","type":"session_meta","payload":{"id":"leaf-thread","session_id":"leaf-session"}}`
	codexUsageLog(t, lines)
	if cs := Calls(time.Time{}); len(cs) != 1 || cs[0].Session != "leaf-session" {
		t.Fatalf("record changed session grouping: %+v", cs)
	}
}
