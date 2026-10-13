package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

const sealedHandoff = `{"type":"agent_message","author":"/root","recipient":"/root/worker","content":[{"type":"input_text","text":"Message Type: NEW_TASK\nPayload:\n"},{"type":"encrypted_content","encrypted_content":"gAAAAATest_ciphertext=="}]}`

func TestCodexSealedAgentHandoffDoesNotAffectOtherMessages(t *testing.T) {
	for _, tc := range []struct {
		name, input string
	}{
		{"plain agent task", `[{"type":"agent_message","content":[{"type":"input_text","text":"Message Type: NEW_TASK\nPayload:\nreply PONG"}]}]`},
		{"non-native encrypted part", `[{"type":"agent_message","content":[{"type":"input_text","text":"Payload:\n"},{"type":"encrypted_content","encrypted_content":"plain task"}]}]`},
		{"other encrypted item", `[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},{"type":"encrypted_content","encrypted_content":"gAAAAATest_ciphertext=="}]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{t: t, reply: sse(`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[]}}`)}
			setup(t, provider.Responses, f)
			body := `{"model":"fake/m1","stream":true,"input":` + tc.input + `}`
			s := New()
			code, response := postTo(t, s, CodexPath+"/responses", body)
			if code != 200 || f.calls != 1 || strings.Contains(response, "sealed subagent task") {
				t.Fatalf("normal request: status=%d upstream=%d response=%s", code, f.calls, response)
			}
			if tc.name == "plain agent task" && !strings.Contains(string(f.got), "reply PONG") {
				t.Fatal("plain task was not delivered")
			}
			for _, route := range s.Trace(context.Background(), 0, 0).Routes {
				if route.SealedTask || route.LeadAccount != "" {
					t.Fatalf("ordinary request marked as sealed: %+v", route)
				}
			}
		})
	}
}
