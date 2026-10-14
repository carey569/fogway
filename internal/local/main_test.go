package local

import (
	"os"
	"testing"

	"github.com/carey569/fogway/internal/testenv"
)

// The package's tests run in a home of their own, off the network, except
// TestE2E, which downloads llama.cpp and a model when FOGWAY_E2E asks it to.
func TestMain(m *testing.M) {
	os.Exit(testenv.RunIn(m, func(string) {
		if os.Getenv("FOGWAY_E2E") == "" {
			testenv.Offline()
		}
	}))
}
