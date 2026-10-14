package cli

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/carey569/fogway/internal/testenv"
)

// The tests run in a home of their own, never the user's real files, and
// off the network (testenv.Offline): the plugin tests start plugin hosts,
// and a host started in a home without a models.dev catalog fetched the
// real one for up to 30s (#1922). Nor do they reach a gateway running on
// this computer: the default address is the app's, and fogway local check
// measured the model the user's app was serving and graded this computer
// by it. A test that wants a gateway sets FOGWAY_ADDR to its own.
func TestMain(m *testing.M) {
	os.Exit(testenv.RunIn(m, func(string) {
		testenv.Offline()
		os.Setenv("FOGWAY_ADDR", noGateway)
	}))
}

// noGateway is an address nothing answers on.
const noGateway = "127.0.0.1:1"

// No test reaches the gateway at the default address, which is the app's
// when it runs here.
func TestNoGatewayByDefault(t *testing.T) {
	if got := gatewayLocal(); got != nil || os.Getenv("FOGWAY_ADDR") != noGateway {
		t.Fatalf("the tests reach a gateway at %q: %+v", os.Getenv("FOGWAY_ADDR"), got)
	}
}

// A host start's models.dev fetch is refused at once, never sent (#1922).
func TestStaysOffTheNetwork(t *testing.T) {
	resp, err := http.Get("https://models.dev/api.json")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("models.dev answered %s: the package's tests reach the network", resp.Status)
	}
	if !strings.Contains(err.Error(), "tests stay off the network") {
		t.Fatalf("models.dev: %v, not testenv's refusal", err)
	}
}
