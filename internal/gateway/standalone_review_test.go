package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

func TestStandalonePlainEncoding(t *testing.T) {
	for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
		for _, body := range []string{`{ "z": "<keep>&", "input": [ { "role": "user", "content": "hi" } ], "a": 1 }`, fmt.Sprintf(`{ "input": [{"type":%q,"call_id":"A","output":"<paired>"}], "instructions":"<keep>&" }`, kind)} {
			if got := orphanedToolOutputs([]byte(body)); string(got) != body {
				t.Fatalf("no-op bytes changed: %s", got)
			}
		}
		body := []byte(fmt.Sprintf(`{ "z": 1, "instructions": "<keep>&", "input": [{"type":%q,"output":"<notice>&"}], "a": 2 }`, kind))
		got := orphanedToolOutputs(body)
		for _, literal := range []string{"<keep>&", "<notice>&"} {
			if !bytes.Contains(got, []byte(literal)) {
				t.Fatalf("escaped %s: %s", literal, got)
			}
		}
		if !bytes.Equal(got, orphanedToolOutputs(got)) {
			t.Fatal("not idempotent")
		}
	}
}

func TestStandaloneChatOrdering(t *testing.T) {
	for _, endpoint := range []string{"/v1/responses", CodexPath + "/responses"} {
		for _, kind := range []string{"function", "custom_tool"} {
			call := func(id string) string {
				if kind == "function" {
					return fmt.Sprintf(`{"type":"function_call","call_id":%q,"name":"echo","arguments":"{}"}`, id)
				}
				return fmt.Sprintf(`{"type":"custom_tool_call","call_id":%q,"name":"echo","input":"echo"}`, id)
			}
			output := func(id, text string) string {
				return fmt.Sprintf(`{"type":%q,"call_id":%q,"output":%q}`, kind+"_call_output", id, text)
			}
			notice := fmt.Sprintf(`{"type":%q,"output":"notice"}`, kind+"_call_output")
			for _, tc := range []struct{ name, input, roles string }{
				{"before_calls", notice + "," + call("A") + "," + output("A", "result A"), "user,assistant,tool"},
				{"before_result", call("A") + "," + notice + "," + output("A", "result A"), "assistant,tool,user"},
				{"between_parallel_results", call("A") + "," + call("B") + "," + output("A", "result A") + "," + notice + "," + output("B", "result B"), "assistant,tool,tool,user"},
				{"empty_id", call("A") + "," + output("", "notice") + "," + output("A", "result A"), "assistant,tool,user"},
			} {
				t.Run(endpoint+"/"+kind+"/"+tc.name, func(t *testing.T) {
					f := &fake{t: t, reply: sse(`data: {"id":"ok","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `data: [DONE]`)}
					var roles []string
					f.refuse = func(body []byte) (int, string) {
						var q struct {
							Messages []struct {
								Role  string `json:"role"`
								Calls []struct {
									ID string `json:"id"`
								} `json:"tool_calls"`
								ID string `json:"tool_call_id"`
							} `json:"messages"`
						}
						if err := json.Unmarshal(body, &q); err != nil {
							t.Error(err)
							return 400, `{"error":{"message":"bad JSON"}}`
						}
						pending := map[string]bool{}
						for _, m := range q.Messages {
							roles = append(roles, m.Role)
							if len(pending) > 0 && m.Role != "tool" {
								return 400, `{"error":{"message":"interrupted tool results"}}`
							}
							if m.Role == "tool" {
								if !pending[m.ID] {
									return 400, `{"error":{"message":"unpaired result"}}`
								}
								delete(pending, m.ID)
							}
							for _, c := range m.Calls {
								pending[c.ID] = true
							}
						}
						if len(pending) > 0 {
							return 400, `{"error":{"message":"missing result"}}`
						}
						return 0, ""
					}
					setup(t, provider.Chat, f)
					body := `{"model":"fake/m1","input":[` + tc.input + `]}`
					var code int
					var response string
					if endpoint == "/v1/responses" {
						code, response = post(t, endpoint, body)
					} else {
						code, response = codexPost(t, body)
					}
					if code != 200 {
						t.Fatalf("%d: %s", code, response)
					}
					if got := strings.Join(roles, ","); got != tc.roles {
						t.Fatalf("roles %s, want %s: %s", got, tc.roles, f.got)
					}
					for _, text := range []string{"notice", "result A"} {
						if !bytes.Contains(f.got, []byte(text)) {
							t.Fatalf("lost %s: %s", text, f.got)
						}
					}
				})
			}
		}
	}
}
