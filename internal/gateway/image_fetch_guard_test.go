package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPublicAddr(t *testing.T) {
	for s, want := range map[string]bool{
		"127.0.0.1": false, "127.9.9.9": false, "10.1.2.3": false, "172.16.0.1": false,
		"192.168.1.1": false, "169.254.169.254": false, "100.64.0.1": false, "100.100.100.100": false,
		"0.0.0.0": false, "224.0.0.1": false, "255.255.255.255": false,
		"::1": false, "::": false, "::ffff:127.0.0.1": false, "::ffff:10.0.0.1": false,
		"::ffff:169.254.169.254": false, "::ffff:100.64.0.1": false, "fe80::1": false, "fc00::1": false, "fd12::1": false,
		"ff02::1": false, "64:ff9b::7f00:1": false, "2002:7f00:1::": false, "::7f00:1": false,
		"8.8.8.8": true, "93.184.216.34": true, "::ffff:8.8.8.8": true, "2606:4700::1111": true,
		"64:ff9b::808:808": true,
	} {
		if got := publicAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("publicAddr(%s) = %v, want %v", s, got, want)
		}
	}
}

// secret is a service only this computer reaches; what it was asked is
// counted.
func secret(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func editFrom(t *testing.T, s *Server, url string, edit func(*http.Request)) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/images/edits", strings.NewReader(`{"model":"art/gpt-image-1","prompt":"make it blue","image":"`+url+`"}`))
	req.Header.Set("Content-Type", "application/json")
	if edit != nil {
		edit(req)
	}
	s.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func resolveTo(t *testing.T, names map[string]string) {
	t.Helper()
	was := lookupIP
	lookupIP = func(_ context.Context, host string) ([]netip.Addr, error) {
		if a, ok := names[host]; ok {
			return []netip.Addr{netip.MustParseAddr(a)}, nil
		}
		return was(context.Background(), host)
	}
	t.Cleanup(func() { lookupIP = was })
}

// The dialler checks the address it connects to, whatever name led there.
func TestPrivateDialerRefusesPrivateAddresses(t *testing.T) {
	srv, hits := secret(t, "")
	for _, addr := range []string{srv.Listener.Addr().String(), "[::1]:9"} {
		c, err := privateDialer.DialContext(context.Background(), "tcp", addr)
		if err == nil {
			c.Close()
		}
		if !errors.Is(err, errPrivateAddress) {
			t.Errorf("%s: %v", addr, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("the private service was asked")
	}
}
