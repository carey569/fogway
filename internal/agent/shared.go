package agent

import (
	"net"
	"net/url"
	"slices"
)

// keptEffort is the default effort a sync leaves on one of fogway's models:
// the one the user set (in the agent's own settings, or by hand) while the
// model still offers it, else fogway's def. fogway's default never
// overrides the user's own.
func keptEffort(was string, levels []string, def string) string {
	if was != "" && slices.Contains(levels, was) {
		return was
	}
	return def
}

// onAnotherMachine says base is an http(s) address whose host isn't this
// machine's loopback.
func onAnotherMachine(base string) bool {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	h := u.Hostname()
	if h == "localhost" {
		return false
	}
	ip := net.ParseIP(h)
	return ip == nil || !ip.IsLoopback() && !ip.IsUnspecified()
}
