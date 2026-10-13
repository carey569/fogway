package sessions

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func snapshotMeta() string {
	return fmt.Sprintf(`{"timestamp":"%s","type":"session_meta","payload":{"id":"child","session_id":"parent","history_mode":"paginated","subagent_history_start_ordinal":219}}`, stamp(0))
}

func snapshotCompaction() string {
	return strings.Replace(cxCompactionLine(0, "snapshot", cxUse(1000, 500, 0, 100, 0), cxUse(30, 10, 0, 2, 0)), `"response_id":"snapshot"`, `"thread_id":"child","response_id":"snapshot"`, 1)
}

func TestCodexUsageSnapshotNeedsSameBoundary(t *testing.T) {
	u := cxUse(100, 50, 0, 10, 0)
	for _, kind := range []string{"different_time", "different_thread", "missing_field", "not_paginated", "new_response"} {
		t.Run(kind, func(t *testing.T) {
			meta, compacted := snapshotMeta(), snapshotCompaction()
			zero := tokenCountLine(0, cxUse(900, 450, 0, 90, 0), cxUse(0, 0, 0, 0, 0))
			switch kind {
			case "different_time":
				zero = strings.Replace(zero, stamp(0), stamp(1), 1)
			case "different_thread":
				compacted = strings.Replace(compacted, `"thread_id":"child"`, `"thread_id":"another"`, 1)
			case "missing_field":
				zero = strings.ReplaceAll(zero, `"reasoning_output_tokens":0,`, "")
			case "not_paginated":
				meta = strings.Replace(meta, `"paginated"`, `"full"`, 1)
			}
			lines := []string{meta, compacted}
			if kind == "new_response" {
				lines = append(lines, cxRecordLine(0, "intervening", cxUse(1010, 505, 0, 101, 0), cxUse(10, 5, 0, 1, 0)))
			}
			lines = append(lines, zero, cxRecordLine(2, "a", cxUse(1100, 550, 0, 110, 0), u), tokenCountLine(2, cxUse(1000, 500, 0, 100, 0), u))
			codexUsageLog(t, lines)
			want := 3
			if kind == "new_response" {
				want++
			}
			if cs := Calls(time.Time{}); len(cs) != want {
				t.Fatalf("unproven offset accepted: %+v", cs)
			}
		})
	}
}
