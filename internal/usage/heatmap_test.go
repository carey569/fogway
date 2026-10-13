package usage

import (
	"reflect"
	"testing"
	"time"
)

// HeatmapOf reads the log itself: a call appended today is today's, and the
// filter given narrows it.
func TestHeatmapOf(t *testing.T) {
	pageHome(t)
	now := holdClock(t, time.Date(2026, 10, 9, 15, 0, 0, 0, time.Local))
	for i, at := range []time.Time{now.AddDate(0, 0, -400), now.AddDate(0, -3, 0), now.Add(-time.Hour), now.Add(-time.Minute)} {
		Append(Record{Time: at, Agent: "claude", Provider: "openai", Model: "m", Input: 10 + i, Status: 200})
	}
	Append(Record{Time: now.Add(-time.Minute), Agent: "codex", Provider: "openai", Model: "m", Input: 100, Status: 200})
	h := HeatmapOf(Filter{Agent: "claude", Day: "2026-10-09"})
	want := []Day{{Date: now.AddDate(0, -3, 0).Format(time.DateOnly), Calls: 1, Tokens: 11}, {Date: "2026-10-09", Calls: 2, Tokens: 25}}
	for i := range h.Days {
		h.Days[i].Cost = 0
	}
	if h.From != "2025-10-06" || h.To != "2026-10-09" || !reflect.DeepEqual(h.Days, want) {
		t.Fatalf("heatmap: %+v", h)
	}
}
