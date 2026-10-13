package provider

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/netproxy"
	"github.com/fogway/fogway/internal/settings"
)

// dialsOut has fogway's own requests go as netproxy.Install has them, and
// tells every host dialed that isn't this machine's, never dialing it.
func dialsOut(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var out []string
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = netproxy.Func
	dial := (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if host, _, _ := net.SplitHostPort(addr); host != "127.0.0.1" && host != "::1" && host != "localhost" {
			mu.Lock()
			out = append(out, addr)
			mu.Unlock()
			return nil, &net.OpError{Op: "dial", Net: network, Err: net.UnknownNetworkError("test: no network")}
		}
		return dial(ctx, network, addr)
	}
	oldT, oldC := http.DefaultTransport, http.DefaultClient.Transport
	http.DefaultTransport, http.DefaultClient.Transport = base, netproxy.Dispatch(base)
	t.Cleanup(func() {
		http.DefaultTransport, http.DefaultClient.Transport = oldT, oldC
		base.CloseIdleConnections()
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), out...)
	}
}

func setQuotaReads(t *testing.T, v string) {
	t.Helper()
	s := settings.Load()
	s.QuotaReads = v
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
}

// The same for a key's windows and balance: routing (KeyAllowance) and
// the cards' timed reads (KeyBalances, PlanQuotas) ask nothing while
// allowances are read only when asked; a key that said it is out
// (StaleKeyAllowance) is read, and so is one the user asks for.
func TestKeyReadsOnlyWhenAsked(t *testing.T) {
	keyLimitsHome(t)
	srv, asked := sub2apiServer(t, func() ([]byte, int) { return sub2apiUsage(t, true), 200 })
	p := sub2apiKey(t, srv)
	if err := Save(p); err != nil {
		t.Fatal(err)
	}
	setQuotaReads(t, "asked")
	t.Cleanup(func() { setQuotaReads(t, "") })
	dialsOut(t) // fogway's own transport, which holds what nobody asked for
	bg := context.Background()
	KeyBalances(bg)
	PlanQuotas(bg)
	KeyAllowance(p)
	time.Sleep(100 * time.Millisecond)
	if n := asked.Load(); n != 0 {
		t.Fatalf("nobody asked, and the key was read %d times", n)
	}
	StaleKeyAllowance(p)
	KeyAllowance(p)
	for i := 0; i < 100 && asked.Load() == 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("a key that said it is out was read %d times, want once", n)
	}
	ForgetBalances()
	KeyBalances(Asked(bg))
	if n := asked.Load(); n < 2 {
		t.Fatal("the user asked, and the key's balance wasn't read")
	}
}
