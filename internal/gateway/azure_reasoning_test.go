package gateway

import (
	"encoding/json"
	"io"
	"net/http"
)

// azureStoreless plays Azure OpenAI's Responses as it answers a request
// with store false: an item given by id alone, with nothing sealed in it,
// is looked up among stored items, and there are none, so the whole
// request is turned away with a 400 (#1008's text, word for word; magpie names the provider before it).
type azureStoreless struct{ inputs [][]map[string]any }

func (a *azureStoreless) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var q struct {
		Store *bool            `json:"store"`
		Input []map[string]any `json:"input"`
	}
	json.Unmarshal(body, &q)
	a.inputs = append(a.inputs, q.Input)
	if r.URL.Path != "/openai/v1/responses" || r.Header.Get("api-key") != "azure-key" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	for _, it := range q.Input {
		enc, _ := it["encrypted_content"].(string)
		if it["type"] == "reasoning" && enc == "" && (q.Store == nil || !*q.Store) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"code":null,"message":"Item with id '`+it["id"].(string)+`' not found. Items are not persisted when `+"`store`"+` is set to false. Try again with `+"`store`"+` set to true, or remove this item from your input.","param":"input","type":"invalid_request_error"}}`)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	io.WriteString(w, sse(
		`data: {"type":"response.created","response":{"id":"resp_az","status":"in_progress"}}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"SUMMARY"}]}}`,
		`data: {"type":"response.completed","response":{"id":"resp_az","status":"completed","output":[],"usage":{"input_tokens":5,"output_tokens":2}}}`))
}
