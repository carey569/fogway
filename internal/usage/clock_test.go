package usage

import (
	"sync/atomic"
	"testing"
	"time"
)

// holdClock stops Clock at at for the test and gives at back, for the test
// to stamp its calls by: midnight then never falls between a call and the
// period it is asked for in.
func holdClock(t *testing.T, at time.Time) time.Time {
	t.Helper()
	old := Clock
	Clock = func() time.Time { return at }
	t.Cleanup(func() { Clock = old })
	return at
}

// midnightClock sets Clock to read a tenth of a second before midnight the
// first time and a tenth of a second after it from then on, as when midnight
// falls while an answer is worked out. It is set again for each answer.
func midnightClock(t *testing.T, midnight time.Time) {
	t.Helper()
	var read atomic.Bool
	old := Clock
	Clock = func() time.Time {
		if read.Swap(true) {
			return midnight.Add(100 * time.Millisecond)
		}
		return midnight.Add(-100 * time.Millisecond)
	}
	t.Cleanup(func() { Clock = old })
}
