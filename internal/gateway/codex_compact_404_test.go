package gateway

import (
	"encoding/base64"
	"strings"
	"testing"
)

// compactedText is the summary in a compaction Codex was answered with.
func compactedText(t *testing.T, body string) string {
	t.Helper()
	for _, e := range events(body) {
		if e["type"] != "response.output_item.done" {
			continue
		}
		item := e["item"].(map[string]any)
		enc, _ := item["encrypted_content"].(string)
		if item["type"] != "compaction" || !strings.HasPrefix(enc, magpieCompaction) {
			t.Fatalf("not magpie's compaction: %s", body)
		}
		b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, magpieCompaction))
		return string(b)
	}
	t.Fatalf("no compaction: %s", body)
	return ""
}

const compact404Request = `{"model":"fake/m1","stream":true,"store":false,"include":["reasoning.encrypted_content"],
  "input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the login bug"}]},
  {"type":"reasoning","id":"rs_1","summary":[]},
  {"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},
  {"type":"function_call_output","call_id":"call_1","output":"main.go"},
  {"type":"message","role":"assistant","content":[{"type":"output_text","text":"I fixed main.go"}]},
  {"type":"message","role":"user","content":[{"type":"input_text","text":"now add a test"}]},
  {"type":"compaction_trigger"}]}`

const relay404 = `{"error":{"message":"Upstream request failed","type":"upstream_error"}}`

// #1511 (TooPub): a relay answered a magpie model's summary request
// 400 invalid_request_error "apex: Upstream request failed", and Codex's
// compaction stopped there, while Codex asking the same relay itself
// compacted. The reporter's body, as it came.
const relayApex400 = `{"error":{"code":null,"message":"apex: Upstream request failed","param":null,"type":"invalid_request_error"}}`
