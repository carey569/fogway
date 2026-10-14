package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/carey569/fogway/internal/netproxy"
	"github.com/carey569/fogway/internal/settings"
)

// Via is ctx for requests made on the provider's behalf: they go through
// its own proxy (ProxyChoice), or the global one when it has none.
func (p Provider) Via(ctx context.Context) context.Context {
	return netproxy.With(ctx, p.ProxyChoice())
}

// ProxyChoice is the proxy requests made on the provider's behalf go
// through, as netproxy.With takes it: for one account of a subscription
// that holds several, or one key of a provider that holds several
// (Beyfish_Wang on X: 不同 key 走不同的代理), its own (AccountProxies),
// else the provider's (Proxy); "" follows the global one. An account in
// use beside the agent's own is made afresh by some agents (AlsoOn),
// without the provider's settings: those are read from providers.json.
func (p Provider) ProxyChoice() string {

	if v := p.AccountProxies[keyID(p.Key)]; p.Key != "" && v != "" {
		return v
	}
	return p.Proxy

}

// accountKey is how an account is known in AccountProxies: its name as the
// accounts list shows it, in lower case, as accounts are told apart.
func accountKey(user string) string { return strings.ToLower(strings.TrimSpace(user)) }

// normalAccountProxies is m as it is kept: keys in lower case, addresses
// trimmed, accounts following the provider's proxy left out; nil for none.
func normalAccountProxies(m map[string]string) map[string]string {
	var out map[string]string
	for u, v := range m {
		u, v = accountKey(u), strings.TrimSpace(v)
		if u == "" || v == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[u] = v
	}
	return out
}

// keyProxies is a provider of keys' AccountProxies as it is kept: those
// of the keys it still holds, by their KeyID; nil for none.
func keyProxies(p Provider) map[string]string {
	var out map[string]string
	for _, k := range p.KeyList() {
		if v := p.AccountProxies[k.ID]; v != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[k.ID] = v
		}
	}
	return out
}

// checkAccountProxies refuses an account's proxy Settings would refuse.
func checkAccountProxies(m map[string]string) error {
	for u, v := range m {
		if err := settings.CheckProxy(v); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
	}
	return nil
}
