package gateway

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

const verifyLinked = `{"error":{"code":403,"message":"Verify your account to continue.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/signin/continue?sarp=1&x=2"}}]}}`

// Without a link, the agent is told to verify it in the vendor's app; with
// another to ask, the request goes on to it and the refused one rests.
func TestVerifyRefusalFallsBack(t *testing.T) {
	fresh(t)
	a := &scripted{replies: []reply{{403, "application/json", `{"error":{"code":403,"message":"Verify your account to continue.","status":"PERMISSION_DENIED"}}`}}}
	b := &scripted{replies: []reply{{200, "", chatOK}}}
	scriptedOn(t, "a", provider.Chat, a)
	scriptedOn(t, "b", provider.Chat, b)
	provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered})
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || a.n != 1 || b.n != 1 {
		t.Fatalf("%d (a %d, b %d): %s", code, a.n, b.n, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if rest := r.Tries[0].Rest; r.Tries[0].Fail != failVerify || rest == nil || rest.By != "verify" || rest.Link != "" ||
		!strings.Contains(rest.said, "open the Antigravity app") {
		t.Fatalf("tries %+v", r.Tries)
	}
	if failure(403, []byte(`{"error":{"message":"The caller does not have permission"}}`)) == failVerify {
		t.Fatal("any 403 read as a verification")
	}
}
