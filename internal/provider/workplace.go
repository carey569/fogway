package provider

import (
	"regexp"
	"strings"
)

// gitSaidRe is whether the agent's folder is in a git repository, as the
// agent says it beside the folder: Claude Code's " - Is a git repository:
// false" (2.1.296; "Is directory a git repo: Yes" before), OpenCode's
// "  Is directory a git repo: yes".
var gitSaidRe = regexp.MustCompile(`(?mi)^\s*(?:- )?Is (?:a git repository|directory a git repo): (true|false|yes|no)\s*$`)

// GitSaid is the last "yes" or "no" an agent's text says of its folder
// being in a git repository, "" when it says neither.
func GitSaid(text string) string {
	m := gitSaidRe.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return ""
	}
	switch strings.ToLower(m[len(m)-1][1]) {
	case "true", "yes":
		return "yes"
	}
	return "no"
}
