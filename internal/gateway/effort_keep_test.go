package gateway

import (
	"testing"
	"time"
)

// keepsEffort keeps a turn before's level only against a lower pick, while
// the cache is warm and the conversation wasn't compacted.
func TestKeepsEffort(t *testing.T) {
	at := time.Date(2026, 10, 3, 19, 13, 0, 0, time.Local)
	tr := turnRule{effort: "xhigh", at: at, size: 230_000}
	for _, c := range []struct {
		name   string
		tr     turnRule
		pick   string
		tokens int
		now    time.Time
		want   bool
	}{
		{"lower, warm", tr, "high", 234_000, at.Add(33 * time.Second), true},
		{"higher", turnRule{effort: "medium", at: at}, "high", 1000, at.Add(time.Second), false},
		{"same", tr, "xhigh", 234_000, at.Add(time.Second), false},
		{"cold", tr, "high", 234_000, at.Add(cacheCold + time.Second), false},
		{"answered since: warm", turnRule{effort: "xhigh", at: at, answered: at.Add(4 * time.Minute)}, "low", 1000, at.Add(6 * time.Minute), true},
		{"compacted", tr, "high", 40_000, at.Add(time.Second), false},
		{"nothing before", turnRule{at: at}, "high", 1000, at.Add(time.Second), false},
		{"no pick", tr, "", 234_000, at.Add(time.Second), false},
	} {
		if got := keepsEffort(c.tr, c.pick, c.tokens, c.now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
