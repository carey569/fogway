package gateway

import (
	"github.com/carey569/fogway/internal/provider"
)

// Tests don't ask vendors how much of an allowance is used: the gateway
// would, behind a request, of whatever upstream a test serves — nor how
// much of a key's own windows.
func init() {
	allowances = func(string) map[string]provider.Allowance { return nil }
	keyAllowance = func(provider.Provider) (provider.Allowance, bool) { return nil, false }
}

func restsOf(cs []candidate) string {
	s := ""
	for _, c := range cs {
		s += c.rest + " "
	}
	return s
}
