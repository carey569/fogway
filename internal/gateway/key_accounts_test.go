package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/access"
	"github.com/carey569/fogway/internal/provider"
)

// A gateway key held to some accounts (#905) counts tokens on those alone:
// the counting candidates — a relay's keys, each an account the key may
// name — are held to the key's accounts as the serving ones are, none but
// the local estimate when it may use none. Gemini's counting, answered by
// the gateway itself, reaches no provider either way.
func TestGatewayKeyAccountsCountTokens(t *testing.T) {
	fresh(t)
	var counted []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counted = append(counted, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"input_tokens":5}`)
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Anthropic: up.URL, Key: "sk-0",
		Keys: []provider.KeyAccount{{Key: "sk-A"}, {Key: "sk-B"}}, Models: []string{"m"}}); err != nil {
		t.Fatal(err)
	}
	keys, secrets := newCaller(t, "Held")
	setAccounts := func(as ...string) {
		t.Helper()
		if _, err := access.Update("accounts-key", access.Change{Key: keys[0].ID, Accounts: as}); err != nil {
			t.Fatal(err)
		}
	}
	h := New().Handler()
	count := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+secrets[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// only the key the key may use counts
	for _, k := range []string{"sk-A", "sk-B"} {
		setAccounts("relay/" + provider.KeyID(k))
		counted = nil
		w := count("/v1/messages/count_tokens", `{"model":"relay/m","messages":[{"role":"user","content":"hi"}]}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "input_tokens") {
			t.Fatalf("counted on %s: %d %s", k, w.Code, w.Body.String())
		}
		if !slices.Equal(counted, []string{k}) {
			t.Fatalf("counted on %s by %v", k, counted)
		}
	}
	// the one key it may use, set not to serve the model: the local
	// estimate, no provider asked
	if err := provider.SetAccountModels("relay", provider.KeyID("sk-A"), []string{"other"}); err != nil {
		t.Fatal(err)
	}
	setAccounts("relay/" + provider.KeyID("sk-A"))
	counted = nil
	w := count("/v1/messages/count_tokens", `{"model":"relay/m","messages":[{"role":"user","content":"hi"}]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "input_tokens") || len(counted) != 0 {
		t.Fatalf("the local estimate: %d %s, asked %v", w.Code, w.Body.String(), counted)
	}
	// Gemini's counting, the model in its URL: the gateway's own estimate
	counted = nil
	w = count("/v1beta/models/relay/m:countTokens", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "totalTokens") || len(counted) != 0 {
		t.Fatalf("gemini counting: %d %s, asked %v", w.Code, w.Body.String(), counted)
	}
}

// A key held to some accounts of one provider uses another's as it always
// did (#905): the list holds it to some accounts of the providers it
// names, not to the providers themselves.
func TestGatewayKeyAccountsOfANamedProvider(t *testing.T) {
	fresh(t)
	plan := &fake{t: t, ctype: "application/json", reply: `{"id":"from-plan","choices":[]}`}
	spare := &fake{t: t, ctype: "application/json", reply: `{"id":"from-spare","choices":[]}`}
	twoProviders(t, plan, spare)
	keys, secrets := newCaller(t, "Held")
	if _, err := access.Update("accounts-key", access.Change{Key: keys[0].ID, Accounts: []string{"plan/" + provider.KeyID("k")}}); err != nil {
		t.Fatal(err)
	}
	h := New().Handler()
	// plan is named: only its key may serve; spare, named nowhere, serves
	// as it always did
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"spare/m2","messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer "+secrets[0])
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "from-spare") {
		t.Fatalf("a provider the key names no account of: %d %s", w.Code, w.Body.String())
	}
	if spare.calls != 1 || plan.calls != 0 {
		t.Fatalf("spare %d, plan %d", spare.calls, plan.calls)
	}
}
