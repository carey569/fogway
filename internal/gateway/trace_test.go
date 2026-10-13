package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/provider"
)

// A vendor's Retry-After reaches routing through a failed request: the
// account rests as long as it says, and the trace says so.
func TestRetryAfterRests(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	restingUntil.Lock()
	restingUntil.m = map[string]time.Time{}
	restingUntil.Unlock()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bearer k-slow" {
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"Too many requests"}}`)
			return
		}
		io.WriteString(w, `{"id":"x","choices":[]}`)
	}))
	defer srv.Close()
	p := provider.Provider{ID: "ra", Name: "RA", Chat: srv.URL + "/v1", Models: []string{"m1"},
		Key: "k-slow", KeyName: "Slow", Keys: []provider.KeyAccount{{Name: "Fast", Key: "k-fast"}}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	s := New()
	s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"ra/m1","messages":[{"role":"user","content":"hi"}]}`)))
	rs := s.Trace(context.Background(), 0, 0).Routes
	if len(rs) != 1 || len(rs[0].Tries) != 2 || rs[0].Tries[0].Rest == nil {
		t.Fatalf("%+v", rs)
	}
	r := rs[0].Tries[0].Rest
	if r.By != "retry-after" || time.Until(r.Until).Round(time.Minute) != 5*time.Minute {
		t.Fatalf("rest %+v", r)
	}
}

// A route weighed with no seats tells its order as [], not null: the
// Routing page reads it as a list, and a null one left a refused request
// that couldn't be opened (Discord, mythfish on v0.1.810).
func TestTraceOrderIsAList(t *testing.T) {
	s := New()
	r := s.trace.begin(Route{Model: "workbuddy-ai/deepseek-v4.1-flash"})
	s.trace.update(r, func(r *Route) {
		r.Tries = append(r.Tries, Try{ID: "workbuddy-ai", Status: 400, Error: "WorkBuddy AI: Invalid request parameters"})
		r.Status, r.Error, r.Done = 400, "WorkBuddy AI: Invalid request parameters", true
	})
	st := s.Trace(context.Background(), 0, 0)
	b, err := json.Marshal(st.Routes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"order":[]`) || strings.Contains(string(b), `"order":null`) {
		t.Fatalf("trace routes = %s, want order []", b)
	}
	if _, c := s.trace.sessionLatest(context.Background(), "", "", 0, 0); c == nil || c.Order == nil {
		t.Fatalf("session's latest route = %+v, want a non-nil order", c)
	}
}
