// Package cli is fogway's command line: every command, and the app, tray
// and TUI it opens (the root's main.go only calls Main).
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/carey569/fogway/internal/agent"
	"github.com/carey569/fogway/internal/appdir"
	"github.com/carey569/fogway/internal/catalog"
	"github.com/carey569/fogway/internal/gateway"
	"github.com/carey569/fogway/internal/netproxy"
	"github.com/carey569/fogway/internal/pluginrt"
	"github.com/carey569/fogway/internal/proc"
	"github.com/carey569/fogway/internal/profile"
	"github.com/carey569/fogway/internal/provider"
	"github.com/carey569/fogway/internal/sessions"
	"github.com/carey569/fogway/internal/settings"
	"github.com/carey569/fogway/internal/tui"
	"github.com/carey569/fogway/internal/update"
)

// version is the root main package's, set at build time
// (-ldflags "-X main.version=…") and handed over by Main.
var version = "dev"

const usage = `fogway — the edge-cloud model gateway for every agent

  fogway                          open the app: a window plus a menu bar icon
  fogway tray                     start in the menu bar only
  fogway panel                    open the menu bar icon's quick panel, or close it
  fogway autostart [on|off]       open fogway (in the menu bar) when you log in, or say whether it does
  fogway tui                      the same thing, in the terminal (serves the gateway while open when no fogway does)
  fogway web [--addr host:port] [--lan] [--no-open] [--gateway]
                                  the app's window in a browser, with the gateway (no desktop needed: WSL, a server over SSH)
                                  a new key each run; FOGWAY_WEB_KEY (16+ letters, digits, - . _ ~) keeps one, signed in for 400 days
                                  behind a reverse proxy, FOGWAY_WEB_URL=https://<the page there> prints the link through it
                                  --gateway: gateway mode, no Agents or Sessions (on by itself with no agents here; Settings › General turns it off)
  fogway ls                       list detected agents and their settings
  fogway <agent>                  show one agent
  fogway <agent> <model>          set an agent's model   e.g. fogway claude deepseek/deepseek-chat
  fogway <agent> <field>          show one field   e.g. fogway codex effort
  fogway <agent> <field> <value>  set another field   e.g. fogway codex effort high
  fogway <agent> default          take fogway out: the agent back on what it had before
  fogway <agent> <field> default  that field back to the agent's own default
  fogway <command> --help         what a command or an agent takes; -h or --help anywhere does nothing else

  fogway local [status]           the model on this computer: llama.cpp running a GGUF model from Hugging Face
  fogway local check              measure this computer: Spark X2.5 4B, 1.7B, or no local model, and how fast each runs
  fogway local setup [<model>]    download llama.cpp and the model now, and turn it on (fogway local models lists them)
  fogway local use|on|off|mask    pick another model, switch it, or mask what group/auto sends the cloud
                                  group/auto: the local model judges each turn, keeps what it can do, sends the rest to the cloud

  fogway save <name>              snapshot every agent's settings as a profile
  fogway use <name>               apply a profile
  fogway profiles                 list profiles
  fogway rm <name>                delete a profile



  fogway providers                list your providers: host, key, models, who uses them
  fogway presets                  the vendors fogway knows: add one with just a key
  fogway provider add <preset> <key>   e.g. fogway provider add deepseek sk-…
  fogway provider add <name> k=v…      a custom vendor (fogway provider for the fields)
  fogway provider key|models|test|rm <id>
  fogway provider fallback <id> <provider/model>…   use these when it's out of quota or down
  fogway import [-y] <link>       add the provider a fogway://import?… link describes
  fogway models [<agent>]         every model agents can pick, as provider/model; an agent's, and why others aren't
  fogway model name <provider/model> <name>|--reset       the name a model goes by, everywhere
  fogway model efforts <provider/model> <l>,<l>|--reset   the reasoning levels a model offers (fogway model help)
  fogway visible [<agent> <family|provider|group>,… | all | --only-picked | --show-new]
                                  which models an agent is shown: families (fogway provider/group set <id> family=…),
                                  or only the models ticked for it, a new one off until it is ticked
  fogway search [add <api> <key>|rm <api>]   Tavily, Brave, Exa, Firecrawl, You.com or SearXNG for web search when no provider can search
  fogway groups                   routing groups: several models agents pick as one, group/<id>
  fogway group add <name> models=<m1>,<m2> [routing=smart|order|rotate|usage|pace] [stays=auto|session|turn|off]
  fogway group <id> | set <id> k=v… | rm <id>   show, change or remove one (fogway group help for more)

  fogway serve                    run the gateway alone (the app runs it too)
  fogway daemon [install|uninstall|start|stop|restart|status]
                                  fogway serve as a service of yours, from boot or login and started again when it stops (systemd --user on Linux, launchd on the Mac)
  fogway healthcheck              exit 0 when the gateway answers (a container's HEALTHCHECK)
  fogway gateway-key list|add <name>|rotate <id>|remove <id>   manage the keys clients use to call a shared gateway
  fogway gateway-key limit <id> [off|reset|day|week|month|<N>d --tokens N --cost USD --cache-reads]   a key's own limit, and what it used
  fogway gateway-key models <id> [all|<provider>/<model>|<provider>/* ...]   the models a key may use, every one unless it names some
  fogway usage [today|7d|30d|all] tokens and cost per agent, model and key (30d)
  fogway usage --csv [--account <name>] [today|7d|30d|all]   every request as CSV (or one account's): the model asked for, sent and served, tokens, cost, time, status, account
  fogway sessions [--model <m>] [--folder <f>] [--json]   the latest Claude Code, Codex, OpenCode and Pi sessions, with what each cost
  fogway sessions --days N|today|all [--model <m>] [--folder <f>] [--json]
                                  what every session spent, day by day, with the top models and folders (7 days)
  fogway quota [<provider>] [--json]  what is left of every plan and key balance
  fogway quota wait <provider|account> [--timeout <d>] [--quiet]
                                  block until that provider or key has allowance again
  fogway quota history [<provider|account>] [--days N] [--json]
                                  each window's readings over time, kept 45 days
  fogway sync                     refresh the model catalog and vendor model lists
  fogway agents                   list every supported agent
  fogway update [check] [--proxy <url>] [--mirror <prefix>]
                                  install the newest release (check: only say if there is one); --proxy: an
                                  http(s):// or socks5:// proxy for it; --mirror: a GitHub download mirror put
                                  before the github.com URL (none unless given; still checked against the update feed's SHA-256)
  fogway update mirror [<prefix>|off]  the mirror every update, the app's own too, is downloaded through
  fogway update auto [on|off] [30m|1h|6h|24h]  whether the app looks for updates by itself, and how often (6h)

agents: claude (cc), codex, gemini, opencode (oc), pi, omp, dsh, goose, cursor, zed, vscode, copilot, crush, kimi, qwen, droid, cline
`

var (
	bold  = lipgloss.NewStyle().Bold(true)
	muted = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8B8F98", Dark: "#7C8290"})
	faint = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C4C7CE", Dark: "#4A4F5A"})
	green = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0F9D58", Dark: "#7EE2A8"})
)

// Main runs fogway with os.Args, at the version the build was stamped with.
func Main(v string) {
	version = v
	// before anything reads or writes a file: no home, or a relative one,
	// would put the agents' configs and fogway's keys under the working
	// folder. What needs no file still answers (fogway version in a
	// container or a script without HOME): run makes fogway's folders first.
	ignored, err := appdir.CheckEnv()
	if err != nil {
		if len(os.Args) > 1 {
			switch os.Args[1] {
			case "-v", "--version", "version":
				fmt.Println("fogway", version)
				return
			case "-h", "--help", "help":
				fmt.Print(usage)
				return
			}
		}
		fmt.Fprintln(os.Stderr, "fogway:", err)
		os.Exit(1)
	}
	slices.Sort(ignored)
	for _, v := range ignored {
		fmt.Fprintf(os.Stderr, "fogway: ignoring %s: not an absolute path (the programs fogway starts still get it)\n", v)
	}
	endProbesOnSignal()
	gateway.Version = version
	netproxy.Install()
	update.GUI = hasGUI
	err = run(os.Args[1:])
	proc.EndProbes() // a CLI still being asked something isn't left to init
	sessions.Saved() // the session index kept, for the next run
	if err != nil {
		// a command with exit codes of its own (quota wait) says which
		code := 1
		var e exitError
		if errors.As(err, &e) {
			code = e.code
		}
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, "fogway:", msg)
		}
		os.Exit(code)
	}
}

// runTUI runs the TUI, which quits on Ctrl+C and SIGTERM itself once it
// has started; it asks CLIs first, and a signal then ends those.
func runTUI() error {
	return tuiRun(ownSignals)
}

// tuiRun is tui.Run; a var so tests can stand in for it.
var tuiRun = tui.Run

func run(args []string) error {
	// before anything is read, moved or started: --help on any command
	// prints its usage and does nothing else
	if helpAsked(args) {
		fmt.Println(strings.TrimRight(helpFor(args), "\n"))
		return nil
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck() // every few seconds in a container: nothing else
	}
	if len(args) == 2 && args[0] == agent.DryRunArg {
		// an agent disconnected on a copy of its files under a temporary
		// home, for the Agents page to show what disconnecting changes
		// (agent.DisconnectPreview)
		return agent.DryRun(args[1])
	}
	// started by an update, the fogway it replaces goes first: the moves
	// below write the files it may still be writing
	update.AwaitPredecessor()
	makeDirs()
	settings.Migrate()
	// a new install starts with the Essentials preset of built-in plugins
	// (plugins.yml), one that ran before with all of them, told by no file
	// of a fogway run before: so before anything below writes one
	if _, err := pluginrt.Settle(!settings.RanBefore()); err != nil {
		fmt.Fprintf(os.Stderr, "fogway: plugins.yml: %v\n", err)
	}
	if _, err := pluginrt.Refresh(); err != nil {
		fmt.Fprintf(os.Stderr, "fogway: plugins.yml: %v\n", err)
	}
	// the providers and settings read once for every agent's fields, which
	// the moves below look at (a write among them reads them again)
	release := provider.Hold()
	agent.RenameLegacy()
	agent.MoveOffAccountIDs()
	release()
	// a provider added, edited or removed, or a list fetched anew, reaches
	// the model lists agents keep in files of their own
	catalog.Changed = agent.SyncCatalog
	// a model Claude Code names that fogway doesn't serve goes to the one
	// it is set to use for that tier
	gateway.StandIn = agent.StandIn
	// and dsh's patch lists, which dsh reads live: a route left behind by
	// something else writing the file fails every session there until
	// fogway writes its own list again
	gateway.WhileServing = append(gateway.WhileServing, agent.KeepDshWired)
	if len(args) == 0 {
		if hasGUI {
			return runGUI(true, "")
		}
		return runTUI()
	}
	// a fogway:// link the system handed over (Windows, Linux): the app
	// opens it for the user to confirm
	if strings.HasPrefix(strings.ToLower(args[0]), "fogway:") {
		if !hasGUI {
			return importCmd(args)
		}
		return runGUI(false, args[0])
	}
	switch args[0] {
	case "tui":
		return runTUI()
	case "web":
		return webCmd(args[1:])
	case "app", "gui":
		// `fogway gui settings`: the window on that tab, as a restart to
		// update from it comes back (update.RelaunchArgs)
		if len(args) > 1 {
			return runWindow(args[1])
		}
		return runGUI(true, "")
	case "tray":
		return runGUI(false, "")
	case "-Embedding":
		// Windows starting fogway for a click on one of its notifications
		// (a usage alert, #368) left in the Action Center after it quit:
		// the window, on the Usage page
		return runWindow("usage")
	case "panel":
		return runPanel()
	case "autostart":
		return autostartCmd(args[1:])
	case "daemon":
		return daemonCmd(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	case "-v", "--version", "version":
		fmt.Println("fogway", version)
		return nil
	case "ls", "list":
		// in the order the app lists them; those hidden there come last, dimmed
		shown, hidden := settings.Arrange(settings.Load(), agent.Detected(), func(a *agent.Agent) string { return a.ID })
		return list(append(shown, hidden...), true, len(shown))
	case "agents":
		return list(agent.All(), false, -1)
	case "sync":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := catalog.Sync(ctx); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "catalog saved to", catalog.CachePath())
		refreshLive(ctx)
		return nil
	case "save", "use", "rm", "profiles":
		return profiles(args)
	case "import":
		return importCmd(args[1:])
	case "providers":
		return providers()
	case "presets":
		return presets()
	case "provider":
		return providerCmd(args)
	case "models":
		return models(args[1:])
	case "model":
		return modelCmd(args[1:])
	case "visible":
		return visibleCmd(args[1:])
	case "search":
		return searchCmd(args[1:])
	case "groups":
		return groups()
	case "group":
		return groupCmd(args)
	case "serve":
		return serve()
	case "gateway-key":
		return gatewayKeys(args)
	case "usage":
		return usageCmd(args)
	case "sessions":
		return sessionsCmd(args)
	case "quota", "quotas":
		return quotaCmd(args)
	case "update":
		return updateCmd(args)
	case "plugin", "plugins":
		return pluginCmd(args)
	case "local":
		return localCmd(args)
	}

	a, err := agent.Find(args[0])
	if err != nil {
		return err
	}
	// no agent form takes a flag: one is never set as a model or a value
	// (`fogway snow --verbose` would take fogway out of Snow, #1457)
	if f := flagArg(args[1:]); f != "" {
		return fmt.Errorf("unknown flag %s: nothing was changed\n\n%s", f, strings.TrimRight(agentHelp(a), "\n"))
	}
	if len(a.Fields) == 0 && a.Import != nil {
		// `fogway cindy`: it takes fogway through its own link, confirmed there
		if len(args) > 1 {
			link := a.Import()
			openInBrowser(link)
			fmt.Println(green.Render("✓"), bold.Render(a.Name), muted.Render("opened to add fogway — confirm it there"))
			fmt.Println(muted.Render("  " + link))
			return nil
		}
	}
	switch len(args) {
	case 1:
		return list([]*agent.Agent{a}, true, -1)
	case 2:
		// default is fogway's word in any case, as an agent's name and a
		// field's are: fogway codex DEFAULT wrote model = "DEFAULT"
		if strings.EqualFold(args[1], "default") {
			if a.Wired() {
				return disconnect(a)
			}
			return set(a, a.Fields[0].Key, "")
		}
		// `fogway codex model`: a field's name alone asks what it is set to,
		// where it was set as the model (model = "model"). Claude Code's
		// opus is a model's name as well as a tier's, and stays the model
		if f := a.Field(args[1]); f != nil && !modelAlias(a, args[1]) {
			return showField(a, f)
		}
		// `fogway codex xhigh`: a bare value that belongs to a non-model field
		// (effort levels, for instance) is routed there; anything else is a model.
		if f := fieldForValue(a, args[1]); f != nil {
			return set(a, f.Key, args[1])
		}
		return set(a, a.Fields[0].Key, args[1])
	case 3:
		value := args[2]
		if strings.EqualFold(value, "default") {
			value = ""
		}
		return set(a, args[1], value)
	}
	return fmt.Errorf("too many arguments\n\n%s", usage)
}

func set(a *agent.Agent, key, value string) error {
	f := a.Field(key)
	if f == nil {
		var keys []string
		for _, f := range a.Fields {
			keys = append(keys, f.Key)
		}
		return fmt.Errorf("%s has no field %q (fields: %s)", a.Name, key, strings.Join(keys, ", "))
	}
	// a field's name is no value: fogway codex subagent effort ran Codex's
	// subagents on a model called effort. Claude Code's opus is a model's
	// name as well as a tier's
	if g := a.Field(value); value != "" && g != nil && !modelAlias(a, value) {
		return fieldAsValue(a, f, g, value)
	}
	value, err := a.Spell(f.Key, value)
	if err != nil {
		return err
	}
	before := f.Get()
	if err := a.Apply(f.Key, value); err != nil {
		return err
	}
	// what the config reads now, not what was asked: an agent may name the
	// model under a provider of its own (OpenCode's fogway-relay/…), and a
	// value it had already is said to be so
	now := f.Get()
	shown := now
	if value == "" || now == "" {
		shown = muted.Render("default")
	}
	if now == before {
		shown += " " + muted.Render("(unchanged)")
	}
	fmt.Println(green.Render("✓"), bold.Render(a.Name), muted.Render(f.Label), shown)
	if a.Notice != nil {
		if n := a.Notice(); n != "" {
			fmt.Println(muted.Render("  ↻ " + n))
		}
	}
	return nil
}

// modelAlias says whether a word names the agent's model, in any case,
// though a field goes by it too: Claude Code's opus, a tier's name as well
func modelAlias(a *agent.Agent, w string) bool {
	return slices.ContainsFunc(a.ModelAliases, func(m string) bool { return strings.EqualFold(m, w) })
}

// fieldAsValue refuses field g's name given as field f's value, for what
// was meant: two words that are one field's label (fogway codex subagent
// effort), a switch, which is turned on (Claude Code's ultracode), or the
// field, which its name alone shows.
func fieldAsValue(a *agent.Agent, f, g *agent.Field, value string) error {
	for _, name := range []string{f.Key + " " + value, f.Key + "_" + value} {
		if h := a.Field(name); h != nil {
			return fmt.Errorf("%q is one field of %s's: fogway %s %s shows it", h.Label, a.Name, a.ID, h.Key)
		}
	}
	if g.Options != nil {
		if o := g.Options(a.Values()); len(o) == 1 && o[0].Value == "on" {
			return fmt.Errorf("%q is a switch of %s's, not a value for its %s: fogway %s %s on", value, a.Name, f.Label, a.ID, g.Key)
		}
	}
	return fmt.Errorf("%q is a field of %s's, not a value for its %s: fogway %s %s shows it", value, a.Name, f.Label, a.ID, g.Key)
}

// showField is `fogway <agent> <field>`: what the field is set to, as set
// prints it
func showField(a *agent.Agent, f *agent.Field) error {
	v := f.Get()
	if v == "" {
		v = muted.Render("default")
	}
	fmt.Println(bold.Render(a.Name), muted.Render(f.Label), v)
	return nil
}

// disconnect is `fogway <agent> default` on an agent fogway is wired into:
// the Agents page's Disconnect, which puts back what the user had before
// fogway — Claude Code's own model, its endpoint — where a field's default
// leaves the agent as installed (__jingling on X: fogway claude default
// took the model they had set away with fogway's)
func disconnect(a *agent.Agent) error {
	before := a.Values()
	if err := a.Disconnect(); err != nil {
		return err
	}
	now := a.Values()
	for _, f := range a.Fields {
		if before[f.Key] == now[f.Key] {
			continue
		}
		shown := now[f.Key]
		if shown == "" {
			shown = muted.Render("default")
		}
		fmt.Println(green.Render("✓"), bold.Render(a.Name), muted.Render(f.Label), shown)
	}
	fmt.Println(muted.Render("  disconnected from fogway, back to what it had before"))
	if a.Notice != nil {
		if n := a.Notice(); n != "" {
			fmt.Println(muted.Render("  ↻ " + n))
		}
	}
	return nil
}

func fieldForValue(a *agent.Agent, v string) *agent.Field {
	vals := a.Values()
	// the agent's suffix after a model (omp's ":max") aside: a role on the
	// same model at that level offers it as typed, and would take it. A list
	// of models no picker offers: it is for the model
	if a.SplitSuffix != nil {
		m, _, one := a.SplitSuffix(v)
		if !one {
			return nil
		}
		v = m
	}
	// a model stays with the model, even where other fields offer it too
	// (Claude Code's opus/sonnet/haiku/fable)
	for _, o := range a.Fields[0].Options(vals) {
		if o.Value == v {
			return nil
		}
	}
	for i := 1; i < len(a.Fields); i++ {
		for _, o := range a.Fields[i].Options(vals) {
			if o.Value == v {
				return &a.Fields[i]
			}
		}
	}
	return nil
}

// list prints the agents; those from dimFrom on (when not -1) are the ones
// hidden in the app, and are dimmed.
func list(agents []*agent.Agent, detectedOnly bool, dimFrom int) error {
	if len(agents) == 0 {
		return fmt.Errorf("no supported agents found on this machine")
	}
	type row struct{ name, vals, path string }
	var rows []row
	nameW, valW := 0, 0
	for i, a := range agents {
		r := row{name: a.Name, path: tilde(a.Path)}
		if !detectedOnly && !a.Detected() {
			r.name = faint.Render(a.Name)
			r.vals = faint.Render("not detected")
			r.path = ""
		} else {
			vals := a.Values()
			dim := dimFrom >= 0 && i >= dimFrom
			label, value := muted, lipgloss.NewStyle()
			if dim {
				label, value = faint, faint
			}
			var parts []string
			for _, f := range a.Fields {
				v := vals[f.Key]
				if v == "" && f.Quiet {
					continue
				}
				if v == "" {
					v = faint.Render("—")
				} else {
					v = value.Render(v)
				}
				if f.Label == "model" {
					parts = append(parts, v)
				} else {
					parts = append(parts, label.Render(f.Label)+" "+v)
				}
			}
			r.name = bold.Render(a.Name)
			if dim {
				r.name = faint.Render(a.Name) + " " + faint.Render("hidden")
			}
			r.vals = strings.Join(parts, label.Render("  ·  "))
			if a.Import != nil {
				if a.Added != nil && a.Added() {
					r.vals = value.Render("fogway added")
				} else {
					r.vals = label.Render("fogway "+a.ID+" add") + faint.Render("  to add fogway")
				}
			}
		}
		nameW = max(nameW, lipgloss.Width(r.name))
		valW = max(valW, lipgloss.Width(r.vals))
		rows = append(rows, r)
	}
	for _, r := range rows {
		fmt.Printf("  %s  %s  %s\n", pad(r.name, nameW), pad(r.vals, valW), faint.Render(r.path))
	}
	return nil
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func profiles(args []string) error {
	switch args[0] {
	case "profiles":
		ps, err := profile.Load()
		if err != nil {
			return err
		}
		if len(ps) == 0 {
			fmt.Println(muted.Render("no profiles yet · fogway save <name>"))
			return nil
		}
		detected := agent.Detected()
		for _, n := range profile.Names(ps) {
			// ✓ the one the setup is now (superhtoooth on X)
			mark := " "
			if profile.InUse(ps[n], detected, nil) {
				mark = green.Render("✓")
			}
			fmt.Printf("%s %s  %s\n", mark, bold.Render(n), muted.Render(profile.Summary(ps[n])))
		}
		return nil
	case "save":
		if len(args) < 2 {
			return fmt.Errorf("usage: fogway save <name>")
		}
		p, err := profile.Snapshot()
		if err != nil {
			return err
		}
		if err := profile.Save(args[1], p); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "saved profile", bold.Render(args[1]))
		return nil
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: fogway use <name>")
		}
		ps, err := profile.Load()
		if err != nil {
			return err
		}
		p, ok := ps[args[1]]
		if !ok {
			return fmt.Errorf("no profile named %q", args[1])
		}
		a, err := profile.Apply(p)
		if err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "applied", bold.Render(args[1]), muted.Render(fmt.Sprintf("(%d changed)", a.Changed)))
		return nil
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: fogway rm <name>")
		}
		if err := profile.Delete(args[1]); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "deleted profile", bold.Render(args[1]))
		return nil
	}
	return nil
}

func pad(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
