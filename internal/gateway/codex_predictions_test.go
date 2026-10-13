package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fogway/fogway/internal/provider"
	"github.com/fogway/fogway/internal/settings"
)

// predictionSchema is the schema ChatGPT.app's composer predictions ask
// for, as its main bundle builds it ({suggestion: string|null}).
const predictionSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"suggestion":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["suggestion"],"additionalProperties":false}`

// codexPredictionRequest is a composer prediction as ChatGPT.app's Codex
// sends one through its ChatGPT sign-in (#1970, pluo): a hidden turn on a
// fork of the thread, on the thread's own model, its thread_source
// composer_predictions, its prompt ending in the schema.
func codexPredictionRequest(t *testing.T, s *Server, path, model string) *httptest.ResponseRecorder {
	t.Helper()
	prompt := "Predict the user's next message.\n\nReturn only JSON matching this schema: " + predictionSchema + "."
	in, _ := json.Marshal(prompt)
	body := `{"model":"` + model + `","stream":true,"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the login bug"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Fixed it; the tests pass."}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":` + string(in) + `}]}],` +
		`"text":{"format":{"type":"json_schema","name":"codex_output_schema","strict":true,"schema":` + predictionSchema + `}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("session_id", "prediction-thread")
	req.Header.Set("x-codex-turn-metadata", `{"thread_source":"composer_predictions","forked_from_thread_id":"main-chat"}`)
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func predictionReply(text string) string {
	b, _ := json.Marshal(text)
	return sse(
		`data: {"id":"c1","choices":[{"index":0,"delta":{"content":`+string(b)+`}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":8}}`,
		`data: [DONE]`)
}

func saveCodexPredictions(t *testing.T, v string) {
	t.Helper()
	st := settings.Load()
	st.CodexPredictions = v
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
}

// Codex on a fogway model (an "OpenAI" provider table pointed at fogway,
// #1301), with nothing set: the model asked for writes the prediction,
// its answer wrapped as Codex reads it.
func TestCodexPredictionOnFogway(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/backend-api/codex/responses"} {
		t.Run(path, func(t *testing.T) {
			f := &fake{t: t, reply: predictionReply("```json\n{\"suggestion\": \"  Commit it  \"}\n```")}
			setup(t, provider.Chat, f)
			s := New()
			rec := codexPredictionRequest(t, s, path, "fake/m1")
			if text, _ := outputText(t, rec.Body.String()); rec.Code != 200 || text != `{"suggestion":"Commit it"}` || f.calls != 1 {
				t.Fatalf("on fogway: %d %q, calls %d", rec.Code, rec.Body.String(), f.calls)
			}
		})
	}
}

func TestPredictionJSON(t *testing.T) {
	for in, want := range map[string]struct {
		out string
		ok  bool
	}{
		`{"suggestion":"Run it"}`:                         {`{"suggestion":"Run it"}`, true},
		`{"suggestion":null}`:                             {`{"suggestion":null}`, true},
		`{"suggestion":"  "}`:                             {`{"suggestion":null}`, true},
		"<think>hm</think>\n{\"suggestion\":\"Ship it\"}": {`{"suggestion":"Ship it"}`, true},
		"Run the tests":                                   {`{"suggestion":null}`, false},
		`{"description":"x"}`:                             {`{"suggestion":null}`, false},
		`{"suggestion":3}`:                                {`{"suggestion":null}`, false},
		"":                                                {`{"suggestion":null}`, false},
	} {
		if got, ok := predictionJSON(in); got != want.out || ok != want.ok {
			t.Errorf("predictionJSON(%q) = %s %v, want %s %v", in, got, ok, want.out, want.ok)
		}
	}
}
