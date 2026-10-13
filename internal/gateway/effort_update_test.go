package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// effortTurns are a thread's requests as an agent sends them: the
// history so far, each turn adding the reply and the next user message.
func effortTurn(key, effort string, users ...string) string {
	var items []string
	for i, u := range users {
		if i > 0 {
			items = append(items, fmt.Sprintf(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"reply %d"}]}`, i))
		}
		items = append(items, fmt.Sprintf(`{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}`, u))
	}
	return fmt.Sprintf(`{"model":"codex/gpt-6-luna","stream":true,"prompt_cache_key":%q,"reasoning":{"effort":%q},"input":[%s]}`, key, effort, strings.Join(items, ","))
}

// sentShape is what the upstream got: the top-level effort, the
// prompt_cache_key, and the input as roles and updates ("u:high").
func sentShape(t *testing.T, b []byte) (effort, key string, input []string) {
	t.Helper()
	var q struct {
		Key       string `json:"prompt_cache_key"`
		Reasoning struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		Input []struct {
			Type      string `json:"type"`
			Role      string `json:"role"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		} `json:"input"`
	}
	if err := json.Unmarshal(b, &q); err != nil {
		t.Fatalf("upstream body: %v: %s", err, b)
	}
	for _, it := range q.Input {
		if it.Type == "configuration_update" {
			input = append(input, "u:"+it.Reasoning.Effort)
		} else {
			input = append(input, it.Role)
		}
	}
	return q.Reasoning.Effort, q.Key, input
}

var effortReply = sse(
	`data: {"type":"response.created","response":{"id":"r1","model":"gpt-6-luna"}}`,
	`data: {"type":"response.output_text.delta","delta":"pong"}`,
	`data: {"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":7,"output_tokens":1}}}`)

// #617: a Responses provider not OpenAI's gets a thread's effort change as
// the agent sent it.
func TestEffortChangeOtherVendor(t *testing.T) {
	t.Setenv("MAGPIE_EFFORT_UPDATES", "on")
	effortThreads.Lock()
	effortThreads.m, effortThreads.no = map[string]*effortThread{}, map[string]time.Time{}
	effortThreads.Unlock()
	f := &fake{t: t, reply: effortReply}
	setup(t, provider.Responses, f)
	for _, b := range []string{effortTurn("t1", "low", "a"), effortTurn("t1", "high", "a", "b")} {
		b = strings.Replace(b, "codex/gpt-6-luna", "fake/gpt-6-luna", 1)
		if code, body := post(t, "/v1/responses", b); code != 200 {
			t.Fatalf("status %d: %s", code, body)
		}
	}
	effort, _, input := sentShape(t, f.got)
	if effort != "high" || strings.Join(input, ",") != "user,assistant,user" {
		t.Fatalf("sent %q %v", effort, input)
	}
}
