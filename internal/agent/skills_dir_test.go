package agent

import (
	"path/filepath"
	"testing"
)

// ownAppData puts APPDATA and LOCALAPPDATA in home, where agents keep
// their folders on Windows (Hermes in %LOCALAPPDATA%\hermes): left at the
// package's home, another test's files there are taken for the agent.
func ownAppData(t *testing.T, home string) {
	t.Helper()
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}
