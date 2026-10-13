package provider

import (
	"os"
	"testing"

	"github.com/fogway/fogway/internal/testenv"
)

// TestMain gives the package's tests a home of their own (testenv), so what
// a test leaves behind it never lands in the runner's fogway, nor in APPDATA
// on Windows. And it keeps them off the network (testenv.Offline): a test
// of a key at a vendor's real address asks only what it serves on loopback.
func TestMain(m *testing.M) {
	os.Exit(testenv.RunIn(m, func(string) { testenv.Offline() }))
}
