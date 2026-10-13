package sessions

import (
	"testing"
	"time"
)

// workbuddyCalls are the WorkBuddy calls Calls gives, by message id. Calls
// with no id (WorkBuddy's older history) are keyed by their time.
func workbuddyCalls(t *testing.T) map[string]Call {
	t.Helper()
	out := map[string]Call{}
	for _, c := range Calls(time.Time{}) {
		if c.Agent != "workbuddy" {
			continue
		}
		k := c.Msg
		if k == "" {
			k = c.Time.Format("15:04:05.000")
		}
		if _, dup := out[k]; dup {
			t.Fatalf("message %s counted twice", c.Msg)
		}
		out[k] = c
	}
	return out
}

// workbuddyOld are the older history's calls, which name no messageId: each
// reply is a call of its own, as the Sessions page counts them (workbuddyLine
// replaces the call before it only when the id is the same non-empty one).
func workbuddyOld(cs map[string]Call) []Call {
	var out []Call
	for _, c := range cs {
		if c.Model == "auto" && c.Msg == "" {
			out = append(out, c)
		}
	}
	return out
}

// WorkBuddy's calls reach the usage ledger too: the Sessions page read its
// session files all along, and the Usage page showed none of them.
func TestWorkBuddyCalls(t *testing.T) {
	setupAgents(t)
	cs := workbuddyCalls(t)
	// m1 (its assistant message and its last function call both carry the
	// usage, so it must count once), m2, the reply the user's own custom
	// provider answered, and three brought over from the older history. The
	// one fogway's gateway answered is the gateway's to count
	// (TestWorkBuddyCallsSkipsFogwaysGateway), not read here as well.
	if len(cs) != 6 {
		t.Fatalf("want 6 WorkBuddy calls, got %d: %+v", len(cs), cs)
	}
	m, ok := cs["m1"]
	if !ok {
		t.Fatalf("no call for WorkBuddy's reply: %+v", cs)
	}
	// input with the cache read taken out of it, as its summary counts it:
	// 1200 - 1000
	if m.Model != "hy3" || m.Tokens != (Tokens{200, 80, 1000, 0, 0}) || !m.Time.Equal(ms(1790503205000)) {
		t.Fatalf("the reply: %+v", m)
	}
	// the second reply's own total is its last line's: 9999 − 7777 cached
	if n := cs["m2"]; n.Tokens != (Tokens{2222, 888, 7777, 0, 0}) {
		t.Fatalf("the second reply: %+v", n)
	}
	// three brought over from its older history: input_tokens, no cache, and
	// no messageId — each one a call of its own
	old := workbuddyOld(cs)
	if len(old) != 3 {
		t.Fatalf("want 3 older-history calls, got %d: %+v", len(old), old)
	}
	var in, out int
	for _, c := range old {
		in, out = in+c.Tokens.Input, out+c.Tokens.Output
		if c.Session != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("an older-history reply's session: %+v", c)
		}
	}
	if in != 5500 || out != 130 {
		t.Fatalf("the older history's tokens: in %d out %d, want 5500 130 (%+v)", in, out, old)
	}
}

// A call WorkBuddy made to fogway's gateway is the gateway's record to keep,
// not this one's: the gateway logs it with the provider and account that
// really answered, and nothing here can pair the two, so reading it would
// count the call twice — once as the gateway's and once as a nameless local
// session's. A model the user pointed at another address in the same picker
// ("custom-local:…" as well) is still read: only a routing group's name, or
// fogway's own entry in models.json, says a call went to the gateway.
func TestWorkBuddyCallsSkipsFogwaysGateway(t *testing.T) {
	setupAgents(t)
	cs := workbuddyCalls(t)
	if _, dup := cs["wm1"]; dup {
		t.Fatalf("a call made to fogway's gateway was read from the file too: %+v", cs["wm1"])
	}
	m, ok := cs["wm2"]
	if !ok {
		t.Fatalf("no call for the user's own custom provider: %+v", cs)
	}
	if m.Model != "glm-5" || m.Tokens != (Tokens{1200, 80, 0, 0, 0}) {
		t.Fatalf("the user's own: %+v", m)
	}
}
