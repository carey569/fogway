package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/provider"
)

// An agent shown only the models picked for it (nianlee-official, #1337):
// with its key, /v1/models leaves out a model that came after the switch,
// of a new provider or an old one, while another agent's list has it; and
// the model asked for by name still goes through.
func TestOnlyPickedModelsList(t *testing.T) {
	f := &fake{t: t, reply: sse(`data: {"choices":[{"delta":{"content":"ok"}}]}`, `data: [DONE]`)}
	setup(t, provider.Chat, f)
	list := func(key string) map[string]bool {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		New().Handler().ServeHTTP(rec, req)
		var got struct {
			Data []struct{ ID string } `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &got)
		out := map[string]bool{}
		for _, m := range got.Data {
			out[m.ID] = true
		}
		return out
	}
	if err := provider.SetOnlyPicked("opencode", true); err != nil {
		t.Fatal(err)
	}
	if got := list(TokenFor("opencode")); !got["fake/m1"] {
		t.Fatalf("switched on, what was shown went: %v", got)
	}
	// a new model of the provider it had, and a new provider
	up := httptest.NewServer(f)
	defer up.Close()
	for _, p := range []provider.Provider{
		{ID: "fake", Name: "Fake", Key: "k", Chat: up.URL + "/v1", Models: []string{"m1", "m2"}},
		{ID: "fresh", Name: "Fresh", Key: "k", Chat: up.URL + "/v1", Models: []string{"x1"}},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if got := list(TokenFor("opencode")); !got["fake/m1"] || got["fake/m2"] || got["fresh/x1"] {
		t.Errorf("only picked: %v", got)
	}
	if got := list(TokenFor("cursor")); !got["fake/m2"] || !got["fresh/x1"] {
		t.Errorf("another agent's list: %v", got)
	}
	// asked for by name, one it isn't shown still answers
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fresh/x1","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+TokenFor("opencode"))
	New().Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(string(f.got), `"x1"`) {
		t.Errorf("a model not picked, by name: %d %s; upstream got %s", rec.Code, rec.Body, f.got)
	}
	// ticked, it is listed
	if err := provider.SetPickedModels("opencode", []string{"fake/m1", "fresh/x1"}); err != nil {
		t.Fatal(err)
	}
	if got := list(TokenFor("opencode")); !got["fresh/x1"] || got["fake/m2"] {
		t.Errorf("fresh/x1 ticked: %v", got)
	}
}
