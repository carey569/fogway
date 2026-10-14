package gui

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/access"
	"github.com/carey569/fogway/internal/gateway"
	"github.com/carey569/fogway/internal/settings"
)

func TestSettingsMigratesOldLANCredentialToManagedKey(t *testing.T) {
	sandboxHome(t)
	old := "sk-fogway-0123456789abcdef0123456789abcdef01234567"
	s := settings.Load()
	s.LAN, s.LANKey = true, old
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	handler := Handler(nil, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), old) || strings.Contains(w.Body.String(), `"lanKey"`) {
		t.Fatal("old credential leaked", w.Code, w.Body)
	}
	if settings.Load().LANKey != old {
		t.Fatal("migration did not retain the old credential for older Fogway")
	}
	keys, _ := access.List()
	if len(keys) != 1 || keys[0].Name != "Fogway" || settings.Load().LANKeyID != keys[0].ID {
		t.Fatal(keys)
	}
	who, ok := access.Authenticate(old)
	if !ok || who.KeyID != keys[0].ID {
		t.Fatal(who, ok)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/caller-keys", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), old) {
		t.Fatal("list exposed credential", w.Code, w.Body)
	}
}

func TestGatewayConnectionAddressesFollowLANSharing(t *testing.T) {
	sandboxHome(t)
	s := settings.Load()
	for _, on := range []bool{false, true, false} {
		s.LAN = on
		if err := settings.Save(s); err != nil {
			t.Fatal(err)
		}
		g := providersState().Gateway
		if g.LAN != on {
			t.Fatal("key block visibility does not follow sharing", g.LAN)
		}
		if !on && len(g.LANURLs) != 0 {
			t.Fatal("shared addresses remain after sharing is off", g.LANURLs)
		}
		if on && !slices.Equal(g.LANURLs, gateway.LANURLs()) {
			t.Fatal("Connect does not list the gateway's network addresses", g.LANURLs)
		}
	}
}
