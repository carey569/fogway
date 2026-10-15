package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// Gemini's thought signatures as its thought-signatures guide gives them:
// on the first function call of a step (parallel calls after it go
// unsigned), and on the last text part of a reply with no call, which a
// stream may send as an empty text part of its own (#1445, XYenon).
const (
	gSigCall = "CiQB0e2Kb7Zk8n1sX3qS4Q2iQh3VJb9K1Z0yR5mT6uW7xY8zA9BCsQEKrgEB0e2Kb2w=="
	gSigText = "CiUB0e2Kb3Xn8pQ4rT6vW8yA0cE2gI4kM6oQ8sU0wY2aC4eG6iK8mO0q"
)

// geminiCallStream is a Code Assist stream answering with a signed call
// and an unsigned one after it.
var geminiCallStream = []string{
	`{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"check_flight","args":{"flight":"AA100"}},"thoughtSignature":"` + gSigCall + `"}]}}],"modelVersion":"gemini-3-pro-preview","responseId":"r1"}}`,
	`{"response":{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"location":"Paris"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30},"modelVersion":"gemini-3-pro-preview","responseId":"r1"}}`,
}

// geminiTextStream is one answering in text, its signature in an empty
// text part of its own at the end.
var geminiTextStream = []string{
	`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"AA100 is on time."}]}}],"modelVersion":"gemini-3-pro-preview","responseId":"r2"}}`,
	`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"","thoughtSignature":"` + gSigText + `"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15},"modelVersion":"gemini-3-pro-preview","responseId":"r2"}}`,
}

// geminiReplyParts is every part a Gemini client was given, streamed or
// whole.
func geminiReplyParts(msgs ...map[string]any) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		cands, _ := m["candidates"].([]any)
		for _, c := range cands {
			content, _ := c.(map[string]any)["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, p := range parts {
				out = append(out, p.(map[string]any))
			}
		}
	}
	return out
}

// A Gemini client is given the signatures where Gemini gives them: on the
// signed call, with the call's own id, and on the text. The old decoder
// dropped both.
func TestGeminiReplyKeepsSignatures(t *testing.T) {
	for _, whole := range []bool{false, true} {
		var calls, texts []map[string]any
		var parts []map[string]any
		if whole {
			parts = geminiReplyParts(translateWhole(t, provider.Gemini, provider.Gemini, geminiCallStream...))
			parts = append(parts, geminiReplyParts(translateWhole(t, provider.Gemini, provider.Gemini, geminiTextStream...))...)
		} else {
			parts = geminiReplyParts(translateStream(t, provider.Gemini, provider.Gemini, geminiCallStream...)...)
			parts = append(parts, geminiReplyParts(translateStream(t, provider.Gemini, provider.Gemini, geminiTextStream...)...)...)
		}
		for _, p := range parts {
			if p["functionCall"] != nil {
				calls = append(calls, p)
			} else if p["text"] != nil {
				texts = append(texts, p)
			}
		}
		if len(calls) != 2 || calls[0]["thoughtSignature"] != gSigCall || calls[1]["thoughtSignature"] != nil {
			t.Fatalf("whole=%v: calls %v", whole, calls)
		}
		if id, _ := calls[0]["functionCall"].(map[string]any)["id"].(string); strings.Contains(id, sigMark) {
			t.Fatalf("whole=%v: the signature left in the call's id %q", whole, id)
		}
		signed := false
		for _, p := range texts {
			if p["thoughtSignature"] == gSigText {
				signed = true
			}
		}
		if !signed {
			t.Fatalf("whole=%v: text's signature lost: %v", whole, texts)
		}
	}
}

// A Gemini request's signatures go back to Gemini as the client gave
// them: on the signed call, not on the call after it, and on the text.
// The old build put skip_thought_signature_validator on every call and
// dropped the text's.
func TestGeminiSignaturesGoBackAsTheyCame(t *testing.T) {
	body := `{"model":"gemini-3-pro-preview","contents":[
		{"role":"user","parts":[{"text":"Check flight AA100 and the weather in Paris."}]},
		{"role":"model","parts":[
			{"functionCall":{"name":"check_flight","args":{"flight":"AA100"}},"thoughtSignature":"` + gSigCall + `"},
			{"functionCall":{"name":"get_weather","args":{"location":"Paris"}}}]},
		{"role":"user","parts":[
			{"functionResponse":{"name":"check_flight","response":{"status":"on time"}}},
			{"functionResponse":{"name":"get_weather","response":{"temp":"15C"}}}]},
		{"role":"model","parts":[{"text":"AA100 is on time, and it's 15C in Paris.","thoughtSignature":"` + gSigText + `"}]},
		{"role":"user","parts":[{"text":"Thanks!"}]}]}`
	req, err := parse(provider.Gemini, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	type gContents struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	gemini, err := build(provider.Gemini, req, "gemini-3-pro-preview", "generativelanguage.googleapis.com", false)
	if err != nil {
		t.Fatal(err)
	}
	var ca struct {
		Request gContents `json:"request"`
	}
	if err := json.Unmarshal(buildCodeAssist(req, "gemini-3-pro-preview", "gemini"), &ca); err != nil {
		t.Fatal(err)
	}
	var g gContents
	if err := json.Unmarshal(gemini, &g); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]gContents{"Gemini API": g, "Code Assist": ca.Request} {
		if len(c.Contents) != 5 {
			t.Fatalf("%s: %d contents", name, len(c.Contents))
		}
		calls, text := c.Contents[1].Parts, c.Contents[3].Parts
		if len(calls) != 2 || calls[0]["thoughtSignature"] != gSigCall || calls[1]["thoughtSignature"] != nil {
			t.Fatalf("%s: calls %v", name, calls)
		}
		if len(text) != 1 || text[0]["thoughtSignature"] != gSigText {
			t.Fatalf("%s: text %v", name, text)
		}
	}
}
