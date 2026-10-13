package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/fogway/fogway/internal/pluginrt"
)

// builtinCmd: `fogway plugin builtin [on|off <id>]` — fogway's built-in
// plugins (internal/pluginrt, plugins/), as Plugins › Built-in shows them.
// One in use is refused off, as the window refuses it.
func builtinCmd(rest []string) error {
	if len(rest) == 0 || rest[0] == "list" {
		cs, err := pluginrt.Refresh()
		if err != nil {
			return err
		}
		for _, m := range pluginrt.Registered() {
			i := slices.IndexFunc(cs, func(c pluginrt.Composed) bool { return c.ID == m.ID })
			c := cs[i]
			mark, how := muted.Render("○"), "off"
			if c.Enabled {
				mark, how = green.Render("●"), "on"
			}
			if c.Source == pluginrt.FromInUse {
				how = "on, in use"
			}
			fmt.Printf("%s %-10s %-12s %s\n", mark, m.ID, how, muted.Render(m.Name))
		}
		fmt.Println(muted.Render("in " + tildePath(pluginrt.Path())))
		return nil
	}
	if len(rest) != 2 || (rest[0] != "on" && rest[0] != "off") {
		return errors.New(pluginUsage)
	}
	on, id := rest[0] == "on", rest[1]
	cs, err := pluginrt.Refresh()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(cs, func(c pluginrt.Composed) bool { return c.ID == id })
	if i < 0 {
		return fmt.Errorf("no built-in plugin %q", id)
	}
	if !on && cs[i].InUse > 0 {
		return fmt.Errorf("%s is in use and stays on", id)
	}
	if err := pluginrt.Set(id, on); err != nil {
		return err
	}
	fmt.Println(green.Render("✓"), id, "is", rest[0])
	return nil
}

const pluginUsage = `usage: fogway plugin [builtin] [on|off <id>]
       list fogway's built-in plugins (Sessions, routing groups…), or turn one on or off`

// pluginCmd: `fogway plugin …` is the built-in plugins, with or without
// the word builtin.
func pluginCmd(args []string) error {
	rest := args[1:]
	if len(rest) > 0 && (rest[0] == "builtin" || rest[0] == "builtins" || rest[0] == "built-in") {
		rest = rest[1:]
	}
	return builtinCmd(rest)
}
