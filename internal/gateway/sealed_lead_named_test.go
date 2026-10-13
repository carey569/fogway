package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fogway/fogway/internal/provider"
)

// chatOnlyOn is a provider serving Chat Completions only, as the Qoder
// plugin does: it neither seals a subagent's task nor opens one. It
// counts the requests it is sent.
func chatOnlyOn(t *testing.T, id string) *int {
	t.Helper()
	n := new(int)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		*n++
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n"+
			"data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: id, Name: id, Key: "k", Chat: up.URL + "/v1", Models: []string{"dfmodel"}}); err != nil {
		t.Fatal(err)
	}
	return n
}
