package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/access"
	"github.com/fogway/fogway/internal/provider"
)

// limitedUpstream is a provider whose every call uses 300 input and 50
// output tokens, counting the calls it is sent.
func limitedUpstream(t *testing.T, delay time.Duration) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":300,"completion_tokens":50}}`)
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "plan", Name: "Plan", Key: "upstream-secret", Chat: up.URL + "/v1", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	return &calls
}

func setLimit(t *testing.T, id string, l *access.Limit) {
	t.Helper()
	if _, err := access.Update("limit-key", access.Change{Key: id, Limit: l}); err != nil {
		t.Fatal(err)
	}
}

// pinLimitClock holds the gateway's limit checks at a fixed past noon, so
// a test can't see its calls split across a midnight, a Monday or a 1st.
// The calls are still recorded at the real time, later than that, and a
// window counts every call from its start, so they all count in its window.
func pinLimitClock(t *testing.T) time.Time {
	t.Helper()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	limitClock = func() time.Time { return at }
	t.Cleanup(func() { limitClock = time.Now })
	return at
}
