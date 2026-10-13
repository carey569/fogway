package agent

import (
	"os"
	"strings"
)

// kimiToolUse is whether a model table in the file says it calls tools.
func kimiToolUse(path string) bool {
	b, _ := os.ReadFile(path)
	return strings.Contains(string(b), `"tool_use"`)
}
