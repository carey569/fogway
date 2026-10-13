package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// hangUpAt is an agent that hangs up once the stream's last event has
// reached it, as the OpenAI SDK does on [DONE].
type hangUpAt struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	hungUp bool
}

func (h *hangUpAt) Write(b []byte) (int, error) {
	n, err := h.ResponseRecorder.Write(b)
	if s := h.Body.String(); strings.Contains(s, "[DONE]") || strings.Contains(s, "message_stop\"}") {
		h.hungUp = true
		h.cancel()
	}
	return n, err
}

// A stream's end is seen in each spelling, split across two writes too, and
// never in a reply's text that only talks about one.
func TestHoldWriterSeesEveryStreamEnd(t *testing.T) {
	chunk := `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}` + "\n\n"
	for _, c := range []struct {
		name, stream string
		ended        bool
	}{
		{"chat spaced", chunk + "data: [DONE]\n\n", true},
		{"chat unspaced", chunk + "data:[DONE]\n\n", true},
		{"chat unspaced CRLF", chunk + "data:[DONE]\r\n\r\n", true},
		{"anthropic event unspaced", "event:message_stop\ndata:{}\n\n", true},
		{"anthropic JSON spaced", "data: {\"type\": \"message_stop\"}\n\n", true},
		{"responses JSON spaced", "data: {\"type\": \"response.completed\",\"response\":{}}\n\n", true},
		{"responses incomplete spaced", "data: {\"type\": \"response.incomplete\",\"response\":{}}\n\n", true},
		{"text about DONE", `data: {"choices":[{"index":0,"delta":{"content":"a stream ends with data:[DONE] or data: [DONE]"},"finish_reason":null}]}` + "\n\n", false},
		{"text about message_stop", `data: {"choices":[{"index":0,"delta":{"content":"then event:message_stop and {\"type\": \"message_stop\"}"},"finish_reason":null}]}` + "\n\n", false},
	} {
		for cut := 1; cut < len(c.stream); cut++ {
			h := &holdWriter{}
			h.see([]byte(c.stream[:cut]))
			h.see([]byte(c.stream[cut:]))
			if h.ended != c.ended {
				t.Fatalf("%s cut at %d: ended = %v, want %v", c.name, cut, h.ended, c.ended)
			}
		}
	}
}
