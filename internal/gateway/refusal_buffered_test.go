package gateway

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The ChatGPT backend holding a gpt-6.x reply back for extra safety checks,
// as it did for Koohoko (#248): it says so in a response.metadata event,
// reasons a while, and then fails the response with bio_policy.
var bufferedStart = []string{
	`event: response.created` + "\n" + `data: {"type":"response.created","response":{"id":"r0","status":"in_progress"}}`,
	`event: response.metadata` + "\n" + `data: {"type":"response.metadata","response_id":"r0","metadata":{"type":"safety_buffering","use_cases":["bio"],"reasons":["user_risk"]}}`,
	`event: response.in_progress` + "\n" + `data: {"type":"response.in_progress","response":{"id":"r0","status":"in_progress"}}`,
}

const bioFailed = `event: response.failed` + "\n" + `data: {"type":"response.failed","response":{"id":"r0","status":"failed","error":{"code":"bio_policy","message":"This content was flagged for possible biological risk."}}}`

// A stream the backend said it holds back for safety checks is held past
// the 15s a stream of empty frames is otherwise held for: gpt-6.1-sol at
// xhigh said nothing for 35s before its bio_policy (#248). One that said
// nothing of it is let through as before.
func TestSafetyBufferedStreamHeldLonger(t *testing.T) {
	held := func(lead []string) (*holdWriter, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		h := newHoldWriter(rec, true)
		h.Header().Set("Content-Type", "text/event-stream")
		h.WriteHeader(200)
		io.WriteString(h, sse(lead[:len(lead)-1]...))
		h.since = time.Now().Add(-(holdLongest + 5*time.Second))
		io.WriteString(h, sse(lead[len(lead)-1]))
		return h, rec
	}
	h, rec := held(bufferedStart)
	if h.passing || rec.Body.Len() != 0 {
		t.Fatalf("let through after %s: %q", holdLongest, rec.Body.String())
	}
	io.WriteString(h, sse(bioFailed))
	if !h.failed() || !h.refused || !strings.Contains(h.failMsg, "bio_policy") || rec.Body.Len() != 0 {
		t.Fatalf("failed %v refused %v %q; sent %q", h.failed(), h.refused, h.failMsg, rec.Body.String())
	}

	h, rec = held([]string{bufferedStart[0], bufferedStart[2]})
	if !h.passing || !strings.Contains(rec.Body.String(), "response.in_progress") {
		t.Fatalf("a stream without safety buffering held past %s", holdLongest)
	}
}
