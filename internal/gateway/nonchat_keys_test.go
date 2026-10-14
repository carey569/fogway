package gateway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// keyedVendor answers images and embeddings on any key, and keeps which
// key each path was asked on.
type keyedVendor struct {
	mu  sync.Mutex
	got map[string][]string
}

func (v *keyedVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	v.mu.Lock()
	if v.got == nil {
		v.got = map[string][]string{}
	}
	v.got[r.URL.Path] = append(v.got[r.URL.Path], strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	v.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v1/images/generations":
		io.WriteString(w, `{"created":1,"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(pngBytes)+`"}]}`)
	case "/v1/embeddings":
		io.WriteString(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`)
	default:
		w.WriteHeader(404)
	}
}

func (v *keyedVendor) keys(path string) []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.got[path]...)
}

// keyedRelay is a provider of two keys: the first set to serve the chat
// model only, the second the image and embedding models only (#474).
func keyedRelay(t *testing.T) (*Server, *keyedVendor) {
	t.Helper()
	fresh(t)
	v := &keyedVendor{}
	up := httptest.NewServer(v)
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Chat: up.URL + "/v1", Key: "key-a", Keys: []provider.KeyAccount{{Key: "key-b"}},
		Models: []string{"text", "gpt-image-2", "text-embedding-3-small"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetAccountModels("relay", provider.KeyID("key-a"), []string{"text"}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetAccountModels("relay", provider.KeyID("key-b"), []string{"gpt-image-2", "text-embedding-3-small"}); err != nil {
		t.Fatal(err)
	}
	return New(), v
}

func postJSON(t *testing.T, s *Server, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// An image or an embedding goes to the key whose own list of models has
// it, as a chat does, never the first key, set to serve another (#1657,
// wangru8080); with every key set not to serve it, it is refused with why.
func TestNonChatGoesToTheKeyThatServesIt(t *testing.T) {
	s, v := keyedRelay(t)
	if code, raw := postJSON(t, s, "/v1/embeddings", `{"model":"relay/text-embedding-3-small","input":"a fogway"}`); code != 200 {
		t.Fatalf("embed: %d %s", code, raw)
	}
	if got := v.keys("/v1/embeddings"); len(got) != 1 || got[0] != "key-b" {
		t.Fatalf("embedded on %v, want key-b alone", got)
	}

	// key-b taken off the embedding model: no key serves it, and nothing is sent
	if err := provider.SetAccountModels("relay", provider.KeyID("key-b"), []string{"gpt-image-2"}); err != nil {
		t.Fatal(err)
	}
	if code, raw := postJSON(t, s, "/v1/embeddings", `{"model":"relay/text-embedding-3-small","input":"a fogway"}`); code != 403 || !strings.Contains(raw, "text-embedding-3-small") {
		t.Fatalf("embed on no key: %d %s", code, raw)
	}
	if m := len(v.keys("/v1/embeddings")); m != 1 {
		t.Fatalf("a key set not to serve the model was asked: %d embeddings", m)
	}
}

// relayByKey draws on the key that serves the model and answers any other
// as wangru8080's relay did, 404 "Model … is not supported by any
// configured account in this group"; bare, a plain 404 with no words.
type relayByKey struct {
	mu     sync.Mutex
	serves map[string]string
	bare   bool
	got    []string
}

func (v *relayByKey) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Model string `json:"model"`
	}
	json.NewDecoder(r.Body).Decode(&q)
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	v.mu.Lock()
	v.got = append(v.got, key+" "+r.URL.Path)
	bare := v.bare
	v.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if bare {
		w.WriteHeader(404)
		io.WriteString(w, `404 page not found`)
		return
	}
	if v.serves[key] != q.Model {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":{"message":"Model \"`+q.Model+`\" is not supported by any configured account in this group","type":"invalid_request_error"}}`)
		return
	}
	if r.URL.Path != "/v1/images/generations" {
		w.WriteHeader(404)
		return
	}
	io.WriteString(w, `{"created":1,"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(pngBytes)+`"}]}`)
}

func (v *relayByKey) asked() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.got...)
}
