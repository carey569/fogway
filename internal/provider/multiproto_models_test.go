package provider

import (
	"github.com/yetone/magpie/internal/catalog"
)

// idsOf are the ids of a list, in its order.
func idsOf(ms []catalog.Model) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}
