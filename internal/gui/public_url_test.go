package gui

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

// A gateway MAGPIE_ADDR puts on the network (the Docker image) asks other
// machines for a gateway key whether or not it is shared, so the Gateway
// page shows its keys and offers no key-less "this computer" then.
func TestGatewayOnNetworkInConsole(t *testing.T) {
	h := t.TempDir()
	testenv.SetHome(t, h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, ".local", "share"))
	t.Setenv("MAGPIE_ADDR", "0.0.0.0:3425")
	t.Setenv("MAGPIE_PUBLIC_URL", "")
	if err := settings.Save(settings.Settings{LAN: false}); err != nil {
		t.Fatal(err)
	}
	if g := providersState().Gateway; !g.OnNetwork || !g.LAN {
		t.Errorf("gateway on the network, not shared, shown without its keys: %+v", g)
	}
	if err := settings.Save(settings.Settings{LAN: true}); err != nil {
		t.Fatal(err)
	}
	if g := providersState().Gateway; !g.OnNetwork || !g.LAN {
		t.Errorf("gateway on the network, shared: %+v", g)
	}
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:3425")
	if err := settings.Save(settings.Settings{LAN: false}); err != nil {
		t.Fatal(err)
	}
	if g := providersState().Gateway; g.OnNetwork || g.LAN {
		t.Errorf("gateway on loopback, not shared: %+v", g)
	}
}

// #1551: MAGPIE_PUBLIC_URL is offered where other machines are let in,
// and nowhere else: on loopback, not shared, nothing reaches the gateway
// through it without a key it can't take.
func TestPublicURLOnlyWhereOffered(t *testing.T) {
	h := t.TempDir()
	testenv.SetHome(t, h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(h, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(h, ".local", "share"))
	t.Setenv("MAGPIE_PUBLIC_URL", "https://magpie.example.com")
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:3425")
	if err := settings.Save(settings.Settings{LAN: false}); err != nil {
		t.Fatal(err)
	}
	if g := providersState().Gateway; g.PublicURL != "" {
		t.Errorf("loopback, not shared: Gateway page offers %q", g.PublicURL)
	}
	if u := settingsState().PublicURL; u != "" {
		t.Errorf("loopback, not shared: Settings shows %q", u)
	}
	if err := settings.Save(settings.Settings{LAN: true}); err != nil {
		t.Fatal(err)
	}
	if g := providersState().Gateway; g.PublicURL != "https://magpie.example.com" {
		t.Errorf("shared: Gateway page's public URL %q", g.PublicURL)
	}
	t.Setenv("MAGPIE_PUBLIC_URL", "")
	if g := providersState().Gateway; g.PublicURL != "" {
		t.Errorf("no MAGPIE_PUBLIC_URL: Gateway page's public URL %q", g.PublicURL)
	}
	if u := settingsState().PublicURL; u != "" {
		t.Errorf("no MAGPIE_PUBLIC_URL: Settings shows %q", u)
	}
}
