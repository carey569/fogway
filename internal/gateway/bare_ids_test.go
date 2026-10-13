package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// listedIDs is /v1/models' ids, in order, as the request r is answered.
func listedIDs(t *testing.T, r *http.Request) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, r)
	var l struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var ids []string
	for _, m := range l.Data {
		ids = append(ids, m.ID)
	}
	return ids
}
