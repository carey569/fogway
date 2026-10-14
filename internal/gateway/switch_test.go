package gateway

import (
	"fmt"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// A key added while conversations run doesn't take them over (#63): each
// stays with the one key that answered it so far, mid-turn.
func TestAddedKeyLeavesRunningConversations(t *testing.T) {
	fresh(t)
	v := &keyed{}
	serveOn(t, "aff", "k1", []string{"m"}, v)
	provider.SetRouting("aff", provider.Rotate)
	s := New()
	for i := range 4 {
		postAs(t, s, fmt.Sprint("s", i), `{"model":"aff/m","messages":[{"role":"user","content":"hi"}]}`)
	}
	serveOn(t, "aff", "k1", []string{"m"}, v, "k2") // the new key
	provider.SetRouting("aff", provider.Rotate)
	v.tried = nil
	for i := range 4 {
		postAs(t, s, fmt.Sprint("s", i), `{"model":"aff/m","messages":[{"role":"user","content":"hi"},`+
			`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c1","content":"a.go"}]}`)
	}
	if strings.Join(v.tried, ",") != "k1,k1,k1,k1" {
		t.Fatalf("running conversations went to %v", v.tried)
	}
	// a new one may go to the new key
	v.tried = nil
	for i := range 2 {
		postAs(t, s, fmt.Sprint("new", i), `{"model":"aff/m","messages":[{"role":"user","content":"hi"}]}`)
	}
	if !strings.Contains(strings.Join(v.tried, ","), "k2") {
		t.Fatalf("new conversations went to %v", v.tried)
	}
}
