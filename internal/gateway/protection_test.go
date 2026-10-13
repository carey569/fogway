package gateway

import (
	"testing"
)

// A long Codex conversation's compaction, as Codex sends it: the history
// with sealed reasoning and calls, then the trigger.
const protectedCompaction = `{"model":%q,"stream":true,"store":false,"input":[` +
	`{"type":"message","role":"user","content":[{"type":"input_text","text":"refactor the parser"}]},` +
	`{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"gAAAAABo-sealed"},` +
	`{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},` +
	`{"type":"function_call_output","call_id":"call_1","output":"main.go"},` +
	`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]},` +
	`{"type":"compaction_trigger"}]}`

func restingNow(t *testing.T) []string {
	t.Helper()
	restingUntil.Lock()
	defer restingUntil.Unlock()
	var out []string
	for k := range restingUntil.m {
		out = append(out, k)
	}
	return out
}

func TestProtectionBreakOffDoesNotRest(t *testing.T) {
	if lateRests("response protection is unavailable") {
		t.Error("a reply broken off with the backend's refusal of the request rests its account")
	}
	if !lateRests("Unable to reach the model provider") {
		t.Error("a vendor failing mid-reply no longer rests")
	}
}

// brokenOff is the ChatGPT backend's refusal as #1270 (bulai0408, Codex
// 0.159.2 in Lody) met it on a long history: HTTP 200, the reply begun,
// then an error event whose error is nested — the object Codex was shown,
// {"message":"response protection is unavailable","type":"internal_error",
// "param":null,"code":null}, when it came before the reply.
var brokenOff = sse(
	`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-6.1-sol"}}`,
	`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant","content":[]}}`,
	`data: {"type":"response.output_text.delta","output_index":0,"delta":"## Progress so far"}`,
	`event: error`+"\n"+`data: {"type":"error","error":{"message":"response protection is unavailable","type":"internal_error","param":null,"code":null}}`)

// An error event's message nested in its error is the one read, not "the
// model failed".
func TestCompactReplyReadsNestedError(t *testing.T) {
	_, err := compactReply([]byte(brokenOff))
	if err == nil || err.Error() != "response protection is unavailable" {
		t.Errorf("err = %v", err)
	}
}
