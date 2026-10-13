// Package iconproxy names where fogway downloads a remote picture from. An
// import link's icon sits on a host its author picked, and fetching it
// straight from there hands that author the user's IP. FOGWAY_ICON_PROXY
// names a proxy that asks the host itself and passes on only an image
// (?url=<the picture>); without one, the picture is asked for where it is.
package iconproxy

import (
	"net/url"
	"os"
)

// URL is the address to fetch the picture at u through: the proxy
// FOGWAY_ICON_PROXY names, else u itself.
func URL(u string) string {
	base := os.Getenv("FOGWAY_ICON_PROXY")
	if base == "" {
		return u
	}
	return base + "?url=" + url.QueryEscape(u)
}
