package gateway

import (
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// An embeddings model the user disabled is refused before its provider is
// asked.
func TestDisabledEmbeddingsModelRefused(t *testing.T) {
	s, l := shelved(t)
	if err := provider.DisableModels("lib/embed-1"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"lib/embed-1", "embed-1"} {
		code, raw := postRetrieval(t, s, "/v1/embeddings", `{"model":"`+model+`","input":"hi"}`)
		if code != 403 || !strings.Contains(raw, "is disabled in fogway") {
			t.Fatalf("%s: %d %s", model, code, raw)
		}
	}
	if sent := l.got("/v1/embeddings"); len(sent) != 0 {
		t.Fatal("a disabled embeddings model was asked", sent)
	}
}
