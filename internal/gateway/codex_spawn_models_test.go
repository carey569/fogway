package gateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// spawnDescription is the description of the spawn_agent tool in a
// Responses request body, "" when it has none.
func spawnDescription(t *testing.T, body []byte) string {
	t.Helper()
	var q struct {
		Tools []struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Tools       []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &q); err != nil {
		t.Fatalf("%v: %.300s", err, body)
	}
	for _, tl := range q.Tools {
		// a translated request has it flat, multi_agent_v1__spawn_agent
		if tl.Name == "spawn_agent" || strings.HasSuffix(tl.Name, "__spawn_agent") {
			return tl.Description
		}
		for _, in := range tl.Tools {
			if in.Name == "spawn_agent" {
				return in.Description
			}
		}
	}
	return ""
}

// codexSpawnRequest is a real Codex 0.162.0 request, its spawn_agent
// naming the five models Codex names at most, on model.
func codexSpawnRequest(t *testing.T, model string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/workdir/codex-0.162.0.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(withModel(b, model))
}

// searchedSpawnDescription is the description of the spawn_agent a
// tool_search_output in a Responses request body hands the model, or of
// the one among its tools when the request was translated (and the
// searched tools put there), "" when it has neither.
func searchedSpawnDescription(t *testing.T, body []byte) string {
	t.Helper()
	var q struct {
		Input []struct {
			Type  string          `json:"type"`
			Tools json.RawMessage `json:"tools"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &q); err != nil {
		t.Fatalf("%v: %.300s", err, body)
	}
	for _, it := range q.Input {
		if it.Type == "tool_search_output" {
			b, _ := json.Marshal(map[string]json.RawMessage{"tools": it.Tools})
			if d := spawnDescription(t, b); d != "" {
				return d
			}
		}
	}
	return spawnDescription(t, body)
}
