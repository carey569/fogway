package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/provider"
)

// sub2api's refusals, as v0.1.149 sends them on the OpenAI and Responses
// paths: a key out of the 5-hour, day or 7-day limit its owner gave it is
// out of quota till that window starts again, though it says
// rate_limit_exceeded; the relay out of accounts for everyone, or its
// upstream rate limiting, is not the key's quota.
func TestSub2APIKeyLimitIsQuota(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		want   string
	}{
		{429, `{"error":{"message":"api key 7天限额已用完","type":"rate_limit_exceeded"}}`, failQuota},
		{429, `{"error":{"message":"api key 日限额已用完","type":"rate_limit_exceeded"}}`, failQuota},
		{429, `{"error":{"message":"api key 5小时限额已用完","type":"rate_limit_exceeded"}}`, failQuota},
		// the key's own total quota (its middleware's words)
		{429, `{"code":"API_KEY_QUOTA_EXHAUSTED","message":"API key 额度已用完"}`, failQuota},
		// the pool: v0.1.149's 503, later versions' 429
		{503, `{"error":{"message":"No available accounts","type":"api_error"}}`, failOther},
		{429, `{"error":{"message":"All available accounts are currently rate-limited. Please retry later.","type":"rate_limit_error"}}`, failRate},
		{429, `{"error":{"message":"Upstream rate limit exceeded, please retry later","type":"rate_limit_error"}}`, failRate},
		// the wallet
		{403, `{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`, failCredit},
	} {
		if got := failure(c.status, []byte(c.body)); got != c.want {
			t.Errorf("%d %s: %s, want %s", c.status, c.body, got, c.want)
		}
	}
}

// keyWindows swaps in each key's windows, by the key, for a test.
func keyWindows(t *testing.T, of map[string]provider.Allowance) *[]string {
	t.Helper()
	old, oldStale := keyAllowance, staleKeyAllowance
	var mu sync.Mutex
	var staled []string
	keyAllowance = func(p provider.Provider) (provider.Allowance, bool) {
		a, ok := of[p.Key]
		return a, ok
	}
	staleKeyAllowance = func(p provider.Provider) {
		mu.Lock()
		staled = append(staled, p.Key)
		mu.Unlock()
	}
	t.Cleanup(func() { keyAllowance, staleKeyAllowance = old, oldStale })
	return &staled
}

func week(used float64, renews time.Time) provider.Allowance {
	return provider.Allowance{{Used: used, Resets: renews, Span: 7 * 24 * time.Hour, Amount: used * 8, Of: 800, Unit: "USD"}}
}

// A key refused for its own limit rests until the window it filled starts
// again, as an account does, and its windows are read again; when they
// were last read short of full, it rests a quarter of an hour.
func TestKeyOutOfItsWindowRestsUntilReset(t *testing.T) {
	now := time.Now()
	of := map[string]provider.Allowance{
		"sk-full":  week(100, now.Add(3*24*time.Hour)),
		"sk-stale": week(40, now.Add(3*24*time.Hour)),
	}
	staled := keyWindows(t, of)
	key := func(k string) candidate {
		return candidate{p: provider.Provider{ID: "kr", Key: k}, model: "gpt-6-astra", rest: "kr#" + k}
	}
	s := &Server{}
	body := []byte(`{"error":{"message":"api key 7天限额已用完","type":"rate_limit_exceeded"}}`)
	near := func(d, want time.Duration) bool { return d > want-time.Minute && d <= want }

	r := s.restAfter(key("sk-full"), 429, http.Header{}, body)
	if r.Why != failQuota || r.By != "window" || !near(time.Until(r.Until), 3*24*time.Hour) {
		t.Fatalf("full: rests %v by %s (%s)", time.Until(r.Until), r.By, r.Why)
	}
	r = s.restAfter(key("sk-stale"), 429, http.Header{}, body)
	if r.Why != failQuota || r.By != "quota" || !near(time.Until(r.Until), quotaRest) {
		t.Fatalf("stale: rests %v by %s (%s)", time.Until(r.Until), r.By, r.Why)
	}
	if len(*staled) != 2 || (*staled)[0] != "sk-full" || (*staled)[1] != "sk-stale" {
		t.Fatalf("read again: %v", *staled)
	}
}

// The relay out of accounts for everyone (sub2api's 503) says nothing of
// the key's own windows: it backs off a minute, full or not, and they
// aren't read again for it.
func TestKeyPoolErrorDoesNotRestForWindow(t *testing.T) {
	of := map[string]provider.Allowance{"sk-pool": week(100, time.Now().Add(3*24*time.Hour))}
	staled := keyWindows(t, of)
	c := candidate{p: provider.Provider{ID: "kp", Key: "sk-pool"}, model: "gpt-6-astra", rest: "kp#sk-pool"}
	forget := func() { routed.Lock(); delete(routed.failures, c.restKey()); routed.Unlock() }
	forget() // its first failure
	t.Cleanup(forget)
	r := (&Server{}).restAfter(c, 503, http.Header{}, []byte(`{"error":{"message":"No available accounts","type":"api_error"}}`))
	if r.Why != failOther || r.By != "backoff" || time.Until(r.Until) > fallbackCooldown {
		t.Fatalf("pool empty: rests %v by %s (%s)", time.Until(r.Until), r.By, r.Why)
	}
	if len(*staled) != 0 {
		t.Fatalf("read again: %v", *staled)
	}
}

// A key resting out of its own window has it read again as it is
// planned, so a limit raised brings it back where nothing weighs it by
// its windows: an ordered group weighs each member's keys alone, and a
// member with one key not at all. No card is read for it.
func TestRestingKeyReadAgainInAnOrderedGroup(t *testing.T) {
	fresh(t)
	provider.ForgetBalances()
	t.Cleanup(provider.ForgetBalances)
	old := keyAllowance
	keyAllowance = provider.KeyAllowance // the key's windows as read, here
	t.Cleanup(func() { keyAllowance = old })
	week := time.Now().Add(72 * time.Hour).Truncate(time.Second).Format(time.RFC3339)
	var mu sync.Mutex
	limit, reads, raised := 800, 0, 0
	// the owner raises the limit after the key refused, before fogway
	// reads its windows again: a reading that reaches the relay after the
	// refusal waits for it. Else the reading ee2588c2 asks again at once,
	// once one still out as the key refused is back, could reach the
	// relay first (a loaded CI runner), see the week still used up and be
	// trusted for its minute, past this test's wait (d7bdb076's CI)
	var refused bool
	raise := make(chan struct{})
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		held := refused
		mu.Unlock()
		if held {
			<-raise
		}
		mu.Lock()
		l := limit
		reads++
		if l > 800 {
			raised++
		}
		mu.Unlock()
		fmt.Fprintf(w, `{"isValid":true,"mode":"quota_limited","status":"active","rate_limits":[{"window":"7d","limit":%d,"used":800,"remaining":%d,"reset_at":%q}]}`, l, l-800, week)
	}))
	defer relay.Close()
	for _, p := range []provider.Provider{
		{ID: "ko", Name: "KO", Key: "sk-one", Models: []string{"gpt-6-astra"}, Responses: relay.URL + "/v1", BalanceURL: relay.URL + "/v1/usage"},
		{ID: "kx", Name: "KX", Key: "sk-x", Models: []string{"gpt-6-astra"}, Responses: "http://127.0.0.1:1/v1"},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := provider.SaveGroup(provider.Group{ID: "in-order", Name: "In order", Members: []string{"ko/gpt-6-astra", "kx/gpt-6-astra"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	g, ms, ok := provider.FindGroup(provider.GroupPrefix + "in-order")
	if !ok {
		t.Fatal("no group")
	}
	// its card read first: no reading of the plan's own is out when the
	// key refuses, whose reading again after it would come before the
	// limit is raised, and read it unraised
	provider.KeyBalances(context.Background()) // its card: its week used up
	s := &Server{}
	first := func() (string, *Rest) {
		out, pl := s.planGroup(g, ms, provider.Responses)
		for i, c := range out {
			if c.p.ID == "ko" {
				return out[0].p.ID, pl.order[i].Rest
			}
		}
		t.Fatalf("ko not planned: %d planned", len(out))
		return "", nil
	}
	cs, _ := s.planGroup(g, ms, provider.Responses)
	if len(cs) != 2 || cs[0].p.ID != "ko" {
		t.Fatalf("planned %d, ko first: %v", len(cs), len(cs) > 0 && cs[0].p.ID == "ko")
	}
	a := cs[0]
	t.Cleanup(func() { clearRest(a.restKey()) })
	quota := []byte(`{"error":{"message":"api key 7天限额已用完","type":"rate_limit_exceeded"}}`)
	mu.Lock()
	refused = true
	mu.Unlock()
	defer func() {
		select {
		case <-raise:
		default:
			close(raise) // failed before the limit was raised
		}
	}()
	if r := s.restAfter(a, 429, http.Header{}, quota); r.Why != failQuota || r.By != "window" {
		t.Fatalf("out of its week: rests by %s (%s)", r.By, r.Why)
	}

	// given $1,000 a week, and no card read again
	mu.Lock()
	limit = 1000
	mu.Unlock()
	close(raise)
	// a reading out as the key refused is followed by another at once,
	// which may have seen the limit raised and brought it back already
	at, r := first()
	mu.Lock()
	seen := raised
	mu.Unlock()
	if (at != "kx" || r == nil) && (at != "ko" || r != nil || seen == 0) {
		t.Fatalf("resting: %s first, its rest %v, the raised limit read %d times", at, r, seen)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		r, ok := restOf(a.restKey())
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			n := reads
			mu.Unlock()
			t.Fatalf("its limit raised: still rests by %s till %v, its windows read %d times", r.By, r.Until.Format(time.RFC3339), n)
		}
	}
	if at, r := first(); at != "ko" || r != nil {
		t.Fatalf("back: %s first, its rest %v", at, r)
	}
}
