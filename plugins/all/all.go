// Package all registers every built-in plugin. Only fogway's main imports
// it: core never imports a plugin, and a plugin never imports another.
package all

import (
	_ "github.com/fogway/fogway/plugins/groups"
	_ "github.com/fogway/fogway/plugins/sessions"
)
