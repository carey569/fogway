package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

// lastRecord is the newest entry of the usage log.
func lastRecord(t *testing.T) usage.Record {
	t.Helper()
	recs := usage.Load(time.Time{})
	if len(recs) == 0 {
		t.Fatal("nothing in the usage log")
	}
	return recs[len(recs)-1]
}

// A request a vendor turns away is in the usage log with the status, what
// the vendor said, what it called the error and the id it gave the request:
// relayed, and translated into another protocol.
func TestFailedRequestKeepsWhatTheVendorSaid(t *testing.T) {
	for _, c := range []struct {
		name     string
		vendor   provider.Protocol
		endpoint string
		want     string // the log's Endpoint
	}{
		{"relayed", provider.Anthropic, "/v1/messages", "/v1/messages"},
		{"translated", provider.Chat, "/v1/messages", "/v1/messages → /v1/chat/completions"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fresh(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Request-Id", "req_011abc")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down, please"}}`))
			}))
			t.Cleanup(up.Close)
			p := provider.Provider{ID: "fake", Name: "Fake", Key: "k", Models: []string{"sol"}}
			if c.vendor == provider.Chat {
				p.Chat = up.URL + "/v1"
			} else {
				p.Anthropic = up.URL
			}
			if err := provider.Save(p); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			body := `{"model":"fake/sol","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
			New().Handler().ServeHTTP(rec, httptest.NewRequest("POST", c.endpoint, strings.NewReader(body)))
			if rec.Code != 429 {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			got := lastRecord(t)
			if got.Status != 429 || !strings.Contains(got.Error, "slow down, please") || got.ErrType != "rate_limit_error" {
				t.Errorf("status %d, error %q, type %q", got.Status, got.Error, got.ErrType)
			}
			if got.RequestID != "req_011abc" || got.Endpoint != c.want {
				t.Errorf("request id %q, endpoint %q; want req_011abc, %q", got.RequestID, got.Endpoint, c.want)
			}
		})
	}
}

// An answered request keeps the vendor's id for it and the path it came in
// on, and nothing of an error.
func TestAnsweredRequestKeepsItsIDAndEndpoint(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "chatcmpl-77")
		servedBy(provider.Chat, "sol")(w, r)
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: up.URL + "/v1", Models: []string{"sol"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := `{"model":"fake/sol","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set(SessionHeader, "magpie-override")
	req.Header.Set("X-Session-Id", "client-session")
	New().Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := lastRecord(t)
	if got.Session != "magpie-override" || got.NativeSession != "client-session" {
		t.Fatalf("session headers lost: %+v", got)
	}
	if got.ResponseID == "" || got.ResponseID != clientResponseID(t, provider.Chat, rec.Body.String(), false) || got.RequestID != "chatcmpl-77" || got.Endpoint != "/v1/chat/completions" || got.Error != "" || got.ErrType != "" {
		t.Errorf("%+v", got)
	}
}

// A request magpie turns away before any vendor is asked — a model it knows
// nothing of — is in the log too, as the failure it was.
func TestTurnedAwayRequestIsLogged(t *testing.T) {
	fresh(t)
	rec := httptest.NewRecorder()
	body := `{"model":"nothing/here","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`
	New().Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
	if rec.Code != 404 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := lastRecord(t)
	if !got.Rejected || got.Status != 404 || got.Error != "unknown model" || got.Requested != "nothing/here" || got.Endpoint != "/v1/messages" {
		t.Errorf("%+v", got)
	}
}

// What a vendor's body calls its error: the type, else the code, in the
// shapes vendors use; nothing for a body that names none.
func TestErrorType(t *testing.T) {
	for body, want := range map[string]string{
		`{"type":"error","error":{"type":"overloaded_error","message":"x"}}`:                "overloaded_error",
		`{"error":{"message":"x","type":"insufficient_quota","code":"insufficient_quota"}}`: "insufficient_quota",
		`{"error":{"message":"x","code":"model_not_found"}}`:                                "model_not_found",
		`{"error":{"message":"x","code":429}}`:                                              "429",
		`{"code":"1302","msg":"rate limited"}`:                                              "1302",
		`{"error":{"type":"usage_limit_reached","resets_in_seconds":7200}}`:                 "usage_limit_reached",
		`{"type":"error"}`:         "",
		`<html>bad gateway</html>`: "",
		``:                         "",
	} {
		if got := provider.ErrorType([]byte(body)); got != want {
			t.Errorf("%s: %q, want %q", body, got, want)
		}
	}
}
