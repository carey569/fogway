package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Before the cap is reached, a wrong key still gets the helpful 401, not a
// 429 (the burst does not swallow ordinary mistypes).
func TestAuthThrottleKeepsFirstRefusalHelpful(t *testing.T) {
	fresh(t)
	t.Setenv("FOGWAY_ADDR", "")
	h := lanGuard(New().Handler())
	newCaller(t, "LAN client")
	r := httptest.NewRequest("GET", "/v1/fogway/quotas", nil)
	r.RemoteAddr = "10.0.0.7:5000"
	r.Header.Set("Authorization", "Bearer mgp-typo-key-9z7q")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "ending in 9z7q") {
		t.Fatalf("first wrong key not a helpful 401: %d %s", w.Code, w.Body.String())
	}
}
