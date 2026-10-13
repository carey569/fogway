package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// zenDeprecated is OpenCode Zen's answer to a request for exo-free, as it
// was on 2026-10-10 (HTTP/2 410, content-type: application/json), while
// its /zen/v1/models still listed exo-free (MOMO on Discord).
const zenDeprecated = `{"type":"error","error":{"type":"ModelDeprecated","message":"Model exo-free has been deprecated."},"metadata":{"model":"exo-free"}}`

// zenUnsupported is OpenCode Zen's answer to a request for glm-5-free, a
// free model it had stopped serving, as it was on 2026-10-10 (HTTP 401,
// application/json), with the public key (Jeremy.Zhou on Discord: models
// no longer free should leave the list).
const zenUnsupported = `{"type":"error","error":{"type":"ModelError","message":"Model glm-5-free is not supported"}}`

// zen answers exo-free as OpenCode Zen does, and every other model.
type zen struct{ tried []string }

func (z *zen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	model := modelOf(body)
	z.tried = append(z.tried, model)
	w.Header().Set("Content-Type", "application/json")
	if model == "glm-5-free" {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, zenUnsupported)
		return
	}
	if model == "exo-free" {
		w.WriteHeader(http.StatusGone)
		io.WriteString(w, zenDeprecated)
		return
	}
	io.WriteString(w, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"from zen `+model+`"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
}

func listedModels(t *testing.T, s *Server) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/models", nil))
	var got struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("/v1/models: %v %s", err, rec.Body.String())
	}
	var ids []string
	for _, m := range got.Data {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestRetiredWords(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
		want   bool
	}{
		{410, zenDeprecated, true},
		{404, `{"error":{"message":"The model text-davinci-003 has been deprecated","type":"invalid_request_error","code":"model_not_found"}}`, true},
		{400, `{"error":{"message":"max_tokens is deprecated, use max_completion_tokens"}}`, false},
		{410, `{"error":{"message":"gone"}}`, false},
		{429, zenDeprecated, false},
		{401, zenUnsupported, true},
		// Zen's own words for a bad key, and for a model served on another API
		{401, `{"type":"error","error":{"type":"AuthError","message":"Invalid API key."}}`, false},
		{401, `{"type":"error","error":{"type":"ModelError","message":"Model grok-4.7 is not supported for format anthropic"}}`, false},
		{403, zenUnsupported, false},
		{401, `Model glm-5-free is not supported`, true},
		// as the gateway holds it, put in its own words
		{401, `{"error":{"code":null,"message":"OpenCode Zen Free: Model glm-5-free is not supported","param":null,"type":"authentication_error"}}`, true},
		{401, `Model grok-4.7 is not supported for format anthropic`, false},
		{401, `Invalid API key.`, false},
	} {
		if got := modelRetired(c.status, []byte(c.body)); got != c.want {
			t.Errorf("modelRetired(%d, %s) = %v", c.status, c.body, got)
		}
	}
}
