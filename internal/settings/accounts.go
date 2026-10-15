package settings

import (
	"os"
	"strings"
)

// AccountDiscoveryOff is whether fogway leaves agents' sign-ins unread
// (#1535): NoAccountDiscovery saved, or FOGWAY_NO_ACCOUNTS set to anything
// but "", "0" or "false" for this process alone. The variable only ever
// turns discovery off; it never turns on what the user saved off.
func AccountDiscoveryOff() bool {
	return noAccountsEnv() || Load().NoAccountDiscovery
}

func noAccountsEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FOGWAY_NO_ACCOUNTS")))
	return v != "" && v != "0" && v != "false"
}
