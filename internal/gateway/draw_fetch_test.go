package gateway

import (
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// bigPNG is as many bytes as #1576's 4K PNG (10.6 MB), a PNG by its
// signature.
func bigPNG() []byte {
	b := make([]byte, 10_600_000)
	r := rand.New(rand.NewPCG(1576, 2))
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	copy(b, "\x89PNG\r\n\x1a\n")
	return b
}

// fileProxy is the proxy a provider names, standing in front of the host
// its vendor keeps the images it draws on: it serves them, and keeps the
// URLs it was asked for.
type fileProxy struct {
	mu    sync.Mutex
	asked []string
	files map[string][]byte
}

func (f *fileProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.asked = append(f.asked, r.URL.String())
	f.mu.Unlock()
	b, ok := f.files[r.URL.Path]
	if r.URL.Host != "files.test" || !ok {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(b)
}

func (f *fileProxy) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// relayed is a gateway whose provider "relay" answers images with URLs on
// files.test, a host reached only through the provider's own proxy.
func relayed(t *testing.T, vendor http.HandlerFunc) (*Server, *fileProxy) {
	t.Helper()
	fresh(t)
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	up := httptest.NewServer(vendor)
	t.Cleanup(up.Close)
	px := &fileProxy{files: map[string][]byte{}}
	proxy := httptest.NewServer(px)
	t.Cleanup(proxy.Close)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Chat: up.URL + "/v1", Key: "key", Proxy: proxy.URL, Models: []string{"gpt-image-2.5-sunburst"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("relay", up.URL+"/v1", []catalog.Model{{ID: "gpt-image-2.5-sunburst"}}); err != nil {
		t.Fatal(err)
	}
	return New(), px
}

type fetchedAnswer struct {
	Data []struct {
		B64  string `json:"b64_json"`
		URL  string `json:"url"`
		Mime string `json:"mime_type"`
	} `json:"data"`
}
