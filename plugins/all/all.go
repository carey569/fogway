// Package all registers every built-in plugin. Only fogway's main imports
// it: core never imports a plugin, and a plugin never imports another.
package all

import (
	_ "github.com/carey569/fogway/plugins/groups"
	_ "github.com/carey569/fogway/plugins/sessions"
)
