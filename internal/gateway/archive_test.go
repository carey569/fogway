package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

// memBucket is a bucket in memory, or one that refuses every write.
type memBucket struct {
	mu   sync.Mutex
	objs map[string][]byte
	err  error
}

func (b *memBucket) Put(_ context.Context, name string, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	b.objs[name] = data
	return nil
}

func archiveTo(t *testing.T, b *memBucket) {
	t.Helper()
	was := ArchiveBucket
	ArchiveBucket = func() (Putter, bool) { return b, true }
	t.Cleanup(func() { ArchiveBucket = was })
}

// archiveVendor answers a chat request, with a cookie of its own, and
// keeps nothing of what it was sent.
type archiveVendor struct{}

func (archiveVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Set-Cookie", "vendor_session=abc123; HttpOnly")
	io.WriteString(w, `{"id":"c1","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
}

const archiveKey = "sk-proj-abcdefghijklmnopqrstuvwxyz0123456789"

func archivePost(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"fake/m1","messages":[{"role":"user","content":"deploy with ` + archiveKey + `"}],"api_key":"named-secret"}`
	req := httptest.NewRequest("POST", "/v1/chat/completions?key=query-secret&beta=true", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer magpie-gateway-key")
	req.Header.Set("x-api-key", "another-secret")
	req.Header.Set("Cookie", "sid=cookie-secret")
	req.Header.Set("User-Agent", "claude-cli/2.1.0")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	archivePending.Wait()
	return rec
}

// Off, as it is unless turned on, nothing is archived: no upload, no id on
// the call or its response — even with a bucket there to take it.
func TestArchiveOff(t *testing.T) {
	fresh(t)
	serveOn(t, "fake", "k", []string{"m1"}, archiveVendor{})
	b := &memBucket{objs: map[string][]byte{}}
	archiveTo(t, b)
	s := New()
	rec := archivePost(t, s)
	if len(b.objs) != 0 || rec.Header().Get(ArchiveHeader) != "" || s.Recent()[0].Archive != "" {
		t.Fatalf("archived while off: %v %q %+v", keys(b.objs), rec.Header().Get(ArchiveHeader), s.Recent()[0].Archive)
	}
	// on, with no bucket (sync isn't to S3): nothing either
	settings.Save(settings.Settings{RequestArchive: true})
	ArchiveBucket = func() (Putter, bool) { return nil, false }
	rec = archivePost(t, s)
	if len(b.objs) != 0 || rec.Header().Get(ArchiveHeader) != "" || s.Recent()[0].Archive != "" {
		t.Fatalf("archived with no bucket: %v", keys(b.objs))
	}
}

// longVendor answers with a reply far longer than the 256 KB Recent calls
// keeps, ending in RES-TAIL: one JSON body, or, asked to stream, a stream
// of many small events.
type longVendor struct{}

func (longVendor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	// Words that don't repeat: the same few words over and over with no
	// break is a loop the loop guard ends (#1489).
	var sb strings.Builder
	for i := 0; sb.Len() < 450_000; i++ {
		fmt.Fprintf(&sb, "word%d ", i)
	}
	words := sb.String() // 450 KB
	if !strings.Contains(string(b), `"stream":true`) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"c1","object":"chat.completion","model":"m1","choices":[{"index":0,"message":{"role":"assistant","content":"`+words+`RES-TAIL"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for i := range 1000 {
		io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"m1","choices":[{"index":0,"delta":{"content":"`+words[i*450:(i+1)*450]+`"}}]}`+"\n\n")
	}
	io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"m1","choices":[{"index":0,"delta":{"content":"RES-TAIL"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`+"\n\ndata: [DONE]\n\n")
}

// The archive keeps each body whole, not the first 256 KB Recent calls
// shows, a stream's too (#447): its files are gone once it is uploaded,
// and the call's row in the usage log names where it is kept.
func TestArchiveWholeBodies(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			fresh(t)
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			serveOn(t, "fake", "k", []string{"m1"}, longVendor{})
			settings.Save(settings.Settings{RequestArchive: true})
			b := &memBucket{objs: map[string][]byte{}}
			archiveTo(t, b)
			s := New()
			body := `{"model":"fake/m1","stream":` + fmt.Sprint(stream) + `,"messages":[{"role":"user","content":"` + strings.Repeat("sit amet ", 40000) + `REQ-TAIL"}]}`
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body)))
			archivePending.Wait()
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "RES-TAIL") {
				t.Fatalf("%d %.300s", rec.Code, rec.Body)
			}
			c := s.Recent()[0]
			if !c.RequestTruncated || !c.ResponseTruncated || len(c.RequestBody) != callBodyLimit {
				t.Errorf("Recent calls keeps %d, cut %v %v", len(c.RequestBody), c.RequestTruncated, c.ResponseTruncated)
			}
			data := b.objs["archive/"+c.Archive+".json"]
			var a Archived
			if err := json.Unmarshal(data, &a); err != nil {
				t.Fatalf("%v: %v", keys(b.objs), err)
			}
			if a.Request.Body != body || a.Request.Size != int64(len(body)) || a.Request.Truncated {
				t.Errorf("request kept %d of %d, size %d, cut %v", len(a.Request.Body), len(body), a.Request.Size, a.Request.Truncated)
			}
			if a.Response.Body != rec.Body.String() || a.Response.Size != int64(rec.Body.Len()) || a.Response.Truncated {
				t.Errorf("response kept %d of %d, size %d, cut %v", len(a.Response.Body), rec.Body.Len(), a.Response.Size, a.Response.Truncated)
			}
			if u := lastUsage(t); u.Archive == "" || u.Archive != c.Archive {
				t.Errorf("usage row names archive %q, the call %q", u.Archive, c.Archive)
			}
			if left, _ := os.ReadDir(tmp); len(left) != 0 {
				t.Errorf("files left behind: %v", left)
			}
		})
	}
}

// Past RequestArchiveMaxMB a body is cut, said so, and its whole size kept.
func TestArchiveLimit(t *testing.T) {
	fresh(t)
	t.Setenv("TMPDIR", t.TempDir())
	serveOn(t, "fake", "k", []string{"m1"}, archiveVendor{})
	settings.Save(settings.Settings{RequestArchive: true, RequestArchiveMaxMB: 1})
	b := &memBucket{objs: map[string][]byte{}}
	archiveTo(t, b)
	s := New()
	body := `{"model":"fake/m1","messages":[{"role":"user","content":"` + strings.Repeat("sit amet ", 150000) + `"}]}`
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body)))
	archivePending.Wait()
	var a Archived
	if err := json.Unmarshal(b.objs["archive/"+s.Recent()[0].Archive+".json"], &a); err != nil {
		t.Fatal(err)
	}
	if !a.Request.Truncated || a.Request.Size != int64(len(body)) || a.Request.Body != body[:1<<20] {
		t.Errorf("kept %d of %d, size %d, cut %v", len(a.Request.Body), len(body), a.Request.Size, a.Request.Truncated)
	}
	if a.Response.Truncated || !strings.Contains(a.Response.Body, `"content":"done"`) {
		t.Errorf("response %+v", a.Response)
	}
}

// A call with the archive off names no archive in the usage log.
func TestArchiveOffUsage(t *testing.T) {
	fresh(t)
	serveOn(t, "fake", "k", []string{"m1"}, archiveVendor{})
	archiveTo(t, &memBucket{objs: map[string][]byte{}})
	archivePost(t, New())
	if u := lastUsage(t); u.Archive != "" {
		t.Errorf("archive %q while off", u.Archive)
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
