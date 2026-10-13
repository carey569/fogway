package gateway

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// busyThenAnswers holds its first request for hold and answers it 503, as
// OpenCode Go did under a long context (#1532), and streams the next.
type busyThenAnswers struct {
	hold time.Duration
	n    atomic.Int32
}

func (b *busyThenAnswers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.ReadAll(r.Body)
	if b.n.Add(1) == 1 {
		time.Sleep(b.hold)
		http.Error(w, `{"error":{"message":"Service Unavailable"}}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	io.WriteString(w, chatStreamOK)
}

// What of Sent was whose: the try that failed first is not fogway's, nor
// a wait for a slot, nor connecting and sending the body.
func TestSplitSent(t *testing.T) {
	start := time.Unix(1000, 0)
	at := func(ms int64) time.Time { return start.Add(time.Duration(ms) * time.Millisecond) }
	for _, c := range []struct {
		name                 string
		sent                 int64
		first, began         int64
		asked                int64 // ms from start; -1 unknown
		queued               int64
		own, retried, waited int64
	}{
		// #1532: 104 s before the answering try's body went, ~100 s of it
		// the try OpenCode Go held before its 503
		{"failed try first", 104000, 30, 103000, 103900, 0, 930, 102970, 0},
		{"one try", 200, 30, 30, 150, 0, 150, 0, 0},
		{"slot waited for", 5200, 30, 30, 5100, 5000, 100, 0, 5000},
		{"upload unknown", 900, 30, 30, -1, 0, 900, 0, 0},
	} {
		asked := int64(0)
		if c.asked >= 0 {
			asked = at(c.asked).UnixNano()
		}
		own, retried, waited := splitSent(c.sent, start, at(c.first), at(c.began), asked, c.queued)
		if own != c.own || retried != c.retried || waited != c.waited {
			t.Errorf("%s: own %d retried %d waited %d, want %d %d %d", c.name, own, retried, waited, c.own, c.retried, c.waited)
		}
	}
}
