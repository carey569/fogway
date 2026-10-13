package cli

import "testing"

// fogway web on loopback behind a reverse proxy (#1479) prints the proxy's
// link with the key when told the proxy's address, so nobody joins
// "/?k=…" onto it by hand: FOGWAY_WEB_URL, else FOGWAY_PUBLIC_URL with no
// port (one with a port isn't a proxy). None is printed without either.
func TestWebProxyLink(t *testing.T) {
	key := "/?k=0123456789abcdef"
	for _, c := range []struct{ env, want string }{
		{"https://fogway.example.com", "https://fogway.example.com/?k=0123456789abcdef"},
		{"https://fogway.example.com/", "https://fogway.example.com/?k=0123456789abcdef"},
		{"fogway.example.com", "http://fogway.example.com/?k=0123456789abcdef"},
		{"https://example.com/fogway/", "https://example.com/fogway/?k=0123456789abcdef"},
		{"http://192.168.1.20:3725", ""},
		{"", ""},
	} {
		t.Setenv("FOGWAY_PUBLIC_URL", c.env)
		if got := proxyLink(key); got != c.want {
			t.Errorf("FOGWAY_PUBLIC_URL=%q: %q, want %q", c.env, got, c.want)
		}
	}
	// the page's own proxy address, when the gateway's is elsewhere
	t.Setenv("FOGWAY_PUBLIC_URL", "https://api.example.com")
	for _, c := range []struct{ env, want string }{
		{"https://fogway.example.com/", "https://fogway.example.com/?k=0123456789abcdef"},
		{"fogway.example.com:8443", "https://fogway.example.com:8443/?k=0123456789abcdef"},
		{"https://fogway.example.com/?x=1", ""},
		{"ftp://fogway.example.com", ""},
	} {
		t.Setenv("FOGWAY_WEB_URL", c.env)
		if got := proxyLink(key); got != c.want {
			t.Errorf("FOGWAY_WEB_URL=%q: %q, want %q", c.env, got, c.want)
		}
	}
}
