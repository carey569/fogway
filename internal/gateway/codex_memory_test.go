package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// codexCall is a request Codex with fogway as its provider sends to
// /v1/responses: a turn of its own (source "") or a call of a kind of its.
func codexCall(t *testing.T, s *Server, model, source string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"`+model+`","stream":true,"input":"hi"}`))
	req.Header.Set("Authorization", "Bearer "+TokenFor("codex"))
	if source != "" {
		req.Header.Set("x-codex-turn-metadata", `{"thread_source":"`+source+`"}`)
	}
	s.Handler().ServeHTTP(rec, req)
	return rec
}
