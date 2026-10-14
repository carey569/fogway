package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// sealedWorkerMessage is what Codex (codex-rs protocol.rs,
// InterAgentCommunication::to_model_input_item) puts in its lead's input
// when a subagent on one of Codex's own models sends it a message with
// send_message: the ChatGPT backend sealed the call's arguments, so the
// message goes as encrypted_content, under a plain header.
const sealedWorkerMessage = `{"type":"agent_message","author":"/root/worker","recipient":"/root","content":[{"type":"input_text","text":"Message Type: MESSAGE\nTask name: /root\nSender: /root/worker\nPayload:\n"},{"type":"encrypted_content","encrypted_content":"gAAAAATest_worker_message=="}]}`

// plainTask is a lead's spawn_agent message as Codex hands it to a subagent
// when fogway served the lead: plain text, encrypted_function_args [].
const plainTask = `{"type":"agent_message","author":"/root","recipient":"/root/helper","content":[{"type":"input_text","text":"read the README"}]}`

// chatRecorded is a Chat Completions provider (an API key on an
// OpenAI-compatible endpoint, laacmilan's "token"), keeping every request.
type chatRecorded struct {
	mu  sync.Mutex
	got []string
}

func (f *chatRecorded) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

func chatRecordedOn(t *testing.T, id, model string) *chatRecorded {
	t.Helper()
	f := &chatRecorded{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.got = append(f.got, string(b))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n"+
			"data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: id, Name: id, Key: "k", Chat: up.URL + "/v1", Models: []string{model}}); err != nil {
		t.Fatal(err)
	}
	return f
}
