package provider

import (
	"regexp"
)

var secretHeader = regexp.MustCompile(`(?i)auth|key|token|secret|cookie|session|password`)
