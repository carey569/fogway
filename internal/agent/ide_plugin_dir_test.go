package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAt(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// v5tech's ~/.copilot was written by GitHub Copilot's JetBrains plugin
// (ide/, instructions/, prompts/), with no Copilot CLI installed (#1495).
// That folder is no sign of the CLI; its settings.json or its command is.
func TestCopilotFolderFromAnIDEPluginIsNotTheCLI(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	home := t.TempDir()
	a := copilot(home)
	for _, f := range []string{"ide/.registry/1.jsonl", "instructions/a.md", "prompts/b.md", "agents/c.agent.md", "hooks/h.json"} {
		writeAt(t, filepath.Join(a.Dir, f), "{}")
	}
	if copilot(home).Detected() {
		t.Errorf("a ~/.copilot holding only the JetBrains plugin's folders is taken for the Copilot CLI")
	}
	writeAt(t, filepath.Join(bin, "copilot"), "#!/bin/sh\n")
	os.Chmod(filepath.Join(bin, "copilot"), 0o755)
	if !copilot(home).Detected() {
		t.Error("the copilot command on PATH no longer finds the Copilot CLI")
	}
	os.Remove(filepath.Join(bin, "copilot"))
	writeAt(t, a.Path, `{"model":"auto"}`)
	if !copilot(home).Detected() {
		t.Error("~/.copilot/settings.json no longer finds the Copilot CLI")
	}
}
