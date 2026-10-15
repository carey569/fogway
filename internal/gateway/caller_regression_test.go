package gateway

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carey569/fogway/internal/access"
	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/settings"
	"github.com/carey569/fogway/internal/usage"
)

func TestLoopbackRemainsPermissive(t *testing.T) {
	fresh(t)
	keys, secrets := newCaller(t, "Desk")
	access.Update("off-key", access.Change{Key: keys[0].ID})
	for _, shared := range []bool{false, true} {
		if err := settings.Save(settings.Settings{LAN: shared}); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{"", "anything", Token, TokenFor("alma"), "sk-fogway-stale", secrets[0]} {
			for _, addr := range []string{"127.0.0.1:5000", "[::1]:5000"} {
				r := httptest.NewRequest("GET", "/v1/models", nil)
				r.RemoteAddr = addr
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				lanGuard(New().Handler()).ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatalf("shared=%v %s %q: %d", shared, addr, token, w.Code)
				}
			}
		}
	}
}

// A gateway FOGWAY_ADDR puts on the network without sharing it (the Docker
// image) takes another machine's request only with an enabled gateway key,
// however the key is sent; before, it took any token from anyone.
func TestNetworkGatewayNeedsAKey(t *testing.T) {
	fresh(t)
	t.Setenv("FOGWAY_ADDR", "0.0.0.0:3725")
	if _, err := access.Update("add-key", access.Change{Name: "Unused"}); err != nil {
		t.Fatal(err)
	}
	keys, _ := access.List()
	secret, _ := access.Update("copy-key", access.Change{Key: keys[0].ID})
	for _, token := range []string{"", "anything", "fogway", "sk-fogway-stale", secret} {
		for _, header := range []string{"Authorization", "x-api-key", "query"} {
			r := httptest.NewRequest("GET", "/v1/models", nil)
			r.RemoteAddr = "192.168.1.9:5000"
			if header == "query" {
				r.URL.RawQuery = "key=" + token
			} else if header == "Authorization" {
				r.Header.Set(header, "Bearer "+token)
			} else {
				r.Header.Set(header, token)
			}
			w := httptest.NewRecorder()
			lanGuard(New().Handler()).ServeHTTP(w, r)
			if want := map[bool]int{true: 200, false: http.StatusUnauthorized}[token == secret]; w.Code != want {
				t.Fatalf("gateway on the network, not shared, %s %q: %d, want %d", header, token, w.Code, want)
			}
		}
	}
}

func TestMigrationFailureDoesNotStopGateway(t *testing.T) {
	fresh(t)
	t.Setenv("FOGWAY_ADDR", "127.0.0.1:0")
	if err := settings.Save(settings.Settings{LAN: true, LANKey: "sk-fogway-old"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(settings.Path(), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(settings.Path(), 0o600) })
	if err := settings.Save(settings.Load()); err == nil {
		t.Skip("settings.json remains writable on this platform")
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	s := New()
	if err := s.Relisten(); err != nil {
		t.Fatal("migration stopped Relisten", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if !strings.Contains(logs.String(), "migrat") {
			t.Error("migration failure was not logged")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.lnMu.Lock()
		ln := s.ln
		s.lnMu.Unlock()
		if ln != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway did not bind after failed migration")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestConcurrentCallerAttribution(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream" {
			t.Error("caller credential leaked upstream")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":5}}`)
	}))
	defer up.Close()
	if err := provider.Save(provider.Provider{ID: "plan", Name: "Plan", Key: "upstream", Chat: up.URL + "/v1", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	keys, secrets := newCaller(t, "Desk", "Server")
	if err := settings.Save(settings.Settings{LAN: true}); err != nil {
		t.Fatal(err)
	}
	h := lanGuard(New().Handler())
	var wg sync.WaitGroup
	for i := range 120 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(chatReq))
			r.RemoteAddr = "192.168.1.9:5000"
			r.Header.Set("Authorization", "Bearer "+secrets[i%len(secrets)])
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Error(w.Code, w.Body)
			}
		}()
	}
	wg.Wait()
	counts := map[string]int{}
	for _, rec := range usage.Load(time.Time{}) {
		counts[rec.CallerKeyID]++
		if rec.CallerKeyName != keys[0].Name && rec.CallerKeyName != keys[1].Name {
			t.Error("lost caller name", rec)
		}
	}
	if len(counts) != 2 || counts[keys[0].ID] != 60 || counts[keys[1].ID] != 60 {
		t.Fatal("concurrent calls mixed identities", counts)
	}
}
