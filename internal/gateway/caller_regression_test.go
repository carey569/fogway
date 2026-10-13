package gateway

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/settings"
)

func TestLoopbackRemainsPermissive(t *testing.T) {
	fresh(t)
	keys, secrets := newCaller(t, "Desk")
	access.Update("off-key", access.Change{Key: keys[0].ID})
	for _, shared := range []bool{false, true} {
		if err := settings.Save(settings.Settings{LAN: shared}); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{"", "anything", Token, TokenFor("alma"), "sk-magpie-stale", secrets[0]} {
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

// A gateway MAGPIE_ADDR puts on the network without sharing it (the Docker
// image) takes another machine's request only with an enabled gateway key,
// however the key is sent; before, it took any token from anyone.
func TestNetworkGatewayNeedsAKey(t *testing.T) {
	fresh(t)
	t.Setenv("MAGPIE_ADDR", "0.0.0.0:3425")
	if _, err := access.Update("add-key", access.Change{Name: "Unused"}); err != nil {
		t.Fatal(err)
	}
	keys, _ := access.List()
	secret, _ := access.Update("copy-key", access.Change{Key: keys[0].ID})
	for _, token := range []string{"", "anything", "magpie", "sk-magpie-stale", secret} {
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
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:0")
	if err := settings.Save(settings.Settings{LAN: true, LANKey: "sk-magpie-old"}); err != nil {
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
