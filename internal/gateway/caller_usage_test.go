package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/usage"
)

func newCaller(t *testing.T, names ...string) ([]access.Key, []string) {
	t.Helper()
	s := settings.Load()
	s.LAN = true
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	var secrets []string
	for _, name := range names {
		secret, err := access.Update("add-key", access.Change{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		secrets = append(secrets, secret)
	}
	keys, err := access.List()
	if err != nil {
		t.Fatal(err)
	}
	return keys[len(keys)-len(names):], secrets
}

func TestRejectedRequestRetainsCallerIdentity(t *testing.T) {
	fresh(t)
	keys, secrets := newCaller(t, "Rejected client")
	r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"nothing/here","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	r.Header.Set("Authorization", "Bearer "+secrets[0])
	w := httptest.NewRecorder()
	New().Handler().ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatal(w.Code, w.Body.String())
	}
	rec := lastUsage(t)
	if !rec.Rejected || rec.CallerKeyID != keys[0].ID || rec.CallerKeyName != "Rejected client" || rec.Endpoint != "/v1/messages" || rec.Error != "unknown model" || rec.ProviderKeyID != "" {
		t.Fatalf("rejected request lost caller or failure metadata: %+v", rec)
	}
	page := usage.QueryPage(usage.All, usage.Filter{CallerKey: keys[0].ID}, 0, 100)
	if page.Total != 1 || page.Sum.Calls != 0 || len(page.CallerKeys) != 0 || usage.Summarize(usage.All).Calls != 0 {
		t.Fatalf("local rejection must stay visible without inflating usage: %+v", page)
	}
	log, err := os.ReadFile(usage.Path())
	if err != nil || strings.Contains(string(log), secrets[0]) {
		t.Fatal("caller credential in rejected request log", err)
	}
}

func TestManagedLANGuard(t *testing.T) {
	fresh(t)
	keys, secrets := newCaller(t, "Remote")
	shared := settings.Load()
	shared.LAN = true
	if err := settings.Save(shared); err != nil {
		t.Fatal(err)
	}
	h := lanGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who := access.Caller(r.Context())
		if who.KeyID != keys[0].ID || r.URL.Query().Get("key") != Token {
			t.Fatal("query key or user not normalized", who)
		}
	}))
	r := httptest.NewRequest("POST", "/v1beta/models/m:generateContent?key="+secrets[0], nil)
	r.RemoteAddr = "192.168.1.9:5000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	access.Update("off-key", access.Change{Key: keys[0].ID})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("disabled remote key", w.Code)
	}
}

func TestCallerIdentitySurvivesStreamingFailover(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer first" {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: "+`{"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`+"\n\n")
		io.WriteString(w, "data: "+`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20}}`+"\n\ndata: [DONE]\n\n")
	}))
	defer up.Close()
	p := provider.Provider{ID: "plan", Name: "Plan", Key: "first", Chat: up.URL + "/v1", Models: []string{"m1"},
		Keys: []provider.KeyAccount{{Key: "backup", Name: "Backup"}}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	keys, secrets := newCaller(t, "Stream")
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"plan/m1","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	r.Header.Set("Authorization", "Bearer "+secrets[0])
	w := httptest.NewRecorder()
	s := New()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatal(w.Code, w.Body.String())
	}
	rec := lastUsage(t)
	if rec.RouteID == 0 || rec.RouteID != lastRoute(s).ID || rec.CallerKeyID != keys[0].ID || rec.CallerKeyName != "Stream" || rec.ProviderKeyID != provider.KeyID("backup") || rec.ProviderKeyName != "Backup" || rec.Input != 100 || rec.Output != 20 {
		t.Fatal(rec)
	}
}

func TestCallerUsageWithRequestArchive(t *testing.T) {
	for _, header := range []string{"Authorization", "x-api-key", "query"} {
		t.Run(header, func(t *testing.T) {
			fresh(t)
			serveOn(t, "fake", "provider-secret", []string{"m1"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer provider-secret" {
					t.Error("caller credential reached archived request upstream")
				}
				archiveVendor{}.ServeHTTP(w, r)
			}))
			keys, secrets := newCaller(t, "Archived client")
			shared := settings.Load()
			shared.RequestArchive = true
			if err := settings.Save(shared); err != nil {
				t.Fatal(err)
			}
			bucket := &memBucket{objs: map[string][]byte{}}
			archiveTo(t, bucket)
			r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fake/m1","messages":[{"role":"user","content":"hi"}]}`))
			r.RemoteAddr = "192.168.1.9:5000"
			switch header {
			case "Authorization":
				r.Header.Set(header, "Bearer "+secrets[0])
			case "query":
				r.URL.RawQuery = "key=" + secrets[0]
			default:
				r.Header.Set(header, secrets[0])
			}
			s := New()
			w := httptest.NewRecorder()
			lanGuard(s.Handler()).ServeHTTP(w, r)
			archivePending.Wait()
			if w.Code != 200 || w.Header().Get(ArchiveHeader) == "" || len(bucket.objs) != 1 {
				t.Fatal("named caller request was not archived", w.Code, w.Body)
			}
			for _, data := range bucket.objs {
				if strings.Contains(string(data), secrets[0]) || strings.Contains(string(data), "provider-secret") {
					t.Fatal("credential leaked into request archive")
				}
			}
			rec := lastUsage(t)
			if rec.RouteID == 0 || rec.RouteID != lastRoute(s).ID || rec.CallerKeyID != keys[0].ID || rec.CallerKeyName != "Archived client" || rec.ProviderKeyID != provider.KeyID("provider-secret") || rec.Input != 3 {
				t.Fatal("archive lost usage attribution", rec)
			}
		})
	}
}
