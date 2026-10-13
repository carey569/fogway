package gui

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/yetone/magpie/internal/agent"
)

func TestRuntimeUnavailableJSONKeepsMessageAndAction(t *testing.T) {
	for _, action := range []agent.OfflineAction{agent.OfflineStage, agent.OfflineDisconnect, ""} {
		original := &agent.RuntimeUnavailableError{Agent: "aside", Operation: "apply", Offline: action, Cause: fmt.Errorf("fixture unavailable")}
		err := fmt.Errorf("selection: %w", original)
		rec := httptest.NewRecorder()
		fail(rec, err)
		if rec.Code != 400 {
			t.Fatalf("status %d", rec.Code)
		}
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["error"] != err.Error() || out["code"] != "runtime_unavailable" || out["offline"] != string(action) {
			t.Fatalf("shape %+v", out)
		}
	}
	rec := httptest.NewRecorder()
	fail(rec, fmt.Errorf("refusal"))
	var out map[string]string
	json.Unmarshal(rec.Body.Bytes(), &out)
	if _, ok := out["code"]; ok {
		t.Fatal("generic refusal advertised fallback")
	}
}
