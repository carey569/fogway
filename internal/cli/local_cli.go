package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carey569/fogway/internal/gateway"
	"github.com/carey569/fogway/internal/local"
	"github.com/carey569/fogway/internal/provider"
)

const localUsage = `usage: fogway local [status] [--json]
       fogway local check [--json]
       fogway local models
       fogway local setup [<model>|<repo> <file>] [--hf <endpoint>] [--github-mirror <prefix>]
       fogway local use <model>|<repo> <file>
       fogway local on|off
       fogway local mask [on|off]
       fogway local think [on|off]
       fogway local context [<tokens>]
  the model on this computer: llama.cpp's llama-server, downloaded for this
  platform (Metal on a Mac, CUDA with an NVIDIA card, else Vulkan or the CPU),
  running a GGUF model pulled from Hugging Face. It is served as local/<model>,
  and group/auto routes each turn between it and the cloud: the local model
  judges whether it can do the turn itself and whether the turn is sensitive.
  setup downloads llama.cpp and the model now, in the terminal, and turns the
  local model on: the recommended one for this computer's memory unless one is
  named (fogway local models lists them; any repository and .gguf file of it
  can be given). --hf takes a mirror (https://hf-mirror.com); HF_ENDPOINT and
  HF_TOKEN are read too. The app, or fogway serve, runs it while on.
  check measures this computer (about a second) and says which model suits
  it: Spark X2.5 4B where it runs smoothly, else Spark X2.5 1.7B, or that
  none is worth running here. A model that runs is measured too, and what
  it did corrects the estimates.
  mask on has group/auto mask a sensitive turn before it goes to the cloud,
  and keep a sensitive turn the local model takes on this computer even when
  it fails; off routes by what the local model can do alone.
  think on lets the local model reason before it answers, slower; off (the
  default) has it answer at once.
  context sets the window the local model runs at (32k by default, 256k, 1m),
  up to what the model was trained for and what this computer's memory holds;
  with none, it lists the windows and the memory each takes.`

// localAssess is local.Assess; a test stands in for this computer.
var localAssess = local.Assess

func localCmd(args []string) error {
	rest := args[1:]
	sub := ""
	if len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	switch sub {
	case "", "status":
		return localStatus(len(rest) > 0 && (rest[0] == "--json" || rest[0] == "-j"))
	case "check":
		return localCheck(len(rest) > 0 && (rest[0] == "--json" || rest[0] == "-j"))
	case "models":
		return localModels()
	case "setup":
		return localSetup(rest)
	case "use":
		c, err := local.Load()
		if err != nil {
			return err
		}
		m, err := localModelOf(rest)
		if err != nil {
			return err
		}
		c.Model = m
		if err := local.Save(c); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "the local model is", bold.Render(m.ID()), muted.Render("("+m.Repo+" · "+m.File+")"))
		if n := c.ContextOr(); c.Context > n {
			fmt.Println(muted.Render("  it runs at " + local.Tokens(n) + " tokens of context, the most it holds here of the " + local.Tokens(c.Context) + " set"))
		}
		if c.On {
			fmt.Println(muted.Render("  the gateway loads it next; fogway local setup downloads it now"))
		}
		return nil
	case "on", "off":
		c, err := local.Load()
		if err != nil {
			return err
		}
		c.On = sub == "on"
		if c.On && c.Model.Repo == "" {
			// the model recommended now, kept: a later assessment won't
			// swap it for another download
			c.Model = local.Recommend()
		}
		if err := local.Save(c); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "the local model is", sub)
		if c.On {
			if a := local.Assessed(); a.Pick == "" {
				fmt.Println(muted.Render("  fogway local check doesn't recommend a local model here: " + a.Reason()))
			}
		}
		return nil
	case "mask":
		return localMask(rest)
	case "think":
		c, err := local.Load()
		if err != nil {
			return err
		}
		if len(rest) == 0 {
			fmt.Println("thinking is", map[bool]string{true: "on", false: "off"}[c.Think])
			return nil
		}
		if rest[0] != "on" && rest[0] != "off" {
			return fmt.Errorf("fogway local think [on|off]")
		}
		c.Think = rest[0] == "on"
		if err := local.Save(c); err != nil {
			return err
		}
		fmt.Println(green.Render("✓"), "thinking", rest[0], muted.Render("· the local model restarts with it"))
		return nil
	case "context":
		return localContext(rest)
	case "help", "-h", "--help":
		fmt.Println(localUsage)
		return nil
	}
	return fmt.Errorf("%s", localUsage)
}

// localContext shows the windows the local model can run at here, or sets
// one: 256k, 1m or a number of tokens.
func localContext(args []string) error {
	c, err := local.Load()
	if err != nil {
		return err
	}
	m, h := c.Picked(), local.Detect()
	if len(args) == 0 {
		fmt.Printf("%s runs at %s tokens of context\n", cmp.Or(local.Canon(m).Title, m.ID()), local.Tokens(c.ContextOr()))
		for _, w := range local.Windows(m, h) {
			mark := "  "
			if w.Tokens == c.ContextOr() {
				mark = "▸ "
			}
			need := ""
			if w.Need > 0 {
				need = fmt.Sprintf("%.1f GB", float64(w.Need)/(1<<30))
			}
			note := map[local.Grade]string{local.Tight: "tight on memory", local.NoRoom: "too much for this computer"}[w.Room]
			fmt.Printf("%s%-6s %-8s %s\n", mark, local.Tokens(w.Tokens), need, muted.Render(note))
		}
		return nil
	}
	n, err := localTokens(args[0])
	if err != nil {
		return err
	}
	if err := local.FitWindow(m, h, n); err != nil {
		return err
	}
	c.Context = n
	if err := local.Save(c); err != nil {
		return err
	}
	fmt.Println(green.Render("✓"), "context", local.Tokens(n), "tokens", muted.Render("· the local model restarts with it"))
	return nil
}

// localTokens reads a window as the windows are named: 256k is 256×1024
// tokens, 1m 1024×1024; a plain number is tokens.
func localTokens(v string) (int, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	mul := 1
	switch {
	case strings.HasSuffix(s, "k"):
		mul, s = 1<<10, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mul, s = 1<<20, strings.TrimSuffix(s, "m")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 64<<20/mul {
		return 0, fmt.Errorf("%q is not a window: give 32k, 256k, 1m or a number of tokens", v)
	}
	if n*mul < 2048 {
		return 0, fmt.Errorf("a window of %d tokens is too small to hold a turn", n*mul)
	}
	return n * mul, nil
}

// localModelOf is the model args name: a catalog model by its name, or a
// repository and a file of it.
func localModelOf(args []string) (local.Model, error) {
	switch len(args) {
	case 1:
		for _, m := range local.Catalog {
			if strings.EqualFold(m.ID(), args[0]) {
				return m, nil
			}
		}
		return local.Model{}, fmt.Errorf("no model %q in fogway local models; give a repository and a .gguf file of it", args[0])
	case 2:
		if !strings.Contains(args[0], "/") || !strings.HasSuffix(strings.ToLower(args[1]), ".gguf") {
			return local.Model{}, fmt.Errorf("give a Hugging Face repository (owner/name) and a .gguf file in it")
		}
		return local.Canon(local.Model{Repo: args[0], File: args[1]}), nil
	}
	return local.Model{}, fmt.Errorf("fogway local use <model>   or   fogway local use <repo> <file.gguf>")
}

func localStatus(asJSON bool) error {
	c, err := local.Load()
	if err != nil {
		return err
	}
	h := local.Detect()
	if asJSON {
		b, _ := json.MarshalIndent(map[string]any{"config": c, "hardware": h, "model": c.Picked(), "status": gatewayLocal(), "assessment": local.Assessed()}, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	state := "off"
	if c.On {
		state = "on"
	}
	m := c.Picked()
	fmt.Println(bold.Render("local model"), state, muted.Render("· "+local.Ref(m)))
	fmt.Println(muted.Render("  model    "), m.Repo, muted.Render(m.File))
	fmt.Println(muted.Render("  computer "), fmt.Sprintf("%s/%s, %s memory", h.OS, h.Arch, gib(h.RAM)), gpuWords(h), muted.Render("· llama.cpp "+cmpOr(c.Backend, h.Backend())))
	if st := gatewayLocal(); st != nil {
		line := string(st.Phase)
		if st.URL != "" && st.Phase == local.Ready {
			line += " at " + st.URL
			if st.Context > 0 {
				line += " · " + local.Tokens(st.Context) + " tokens of context"
			}
		}
		if st.Progress != nil && st.Progress.Total > 0 {
			line += fmt.Sprintf(" · %s %d%%", st.Progress.What, st.Progress.Done*100/st.Progress.Total)
		}
		if st.Error != "" {
			line += " · " + st.Error
		}
		fmt.Println(muted.Render("  now      "), line)
	} else if c.On {
		fmt.Println(muted.Render("  now      "), "the gateway isn't running: the app or fogway serve runs it")
	}
	if g, _, ok := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID); ok && g.Hybrid != nil {
		mask := "off"
		if g.Hybrid.Mask {
			mask = "on"
		}
		fmt.Println(muted.Render("  route    "), "group/"+provider.HybridGroupID, muted.Render("· "+strings.Join(g.Members, ", ")+" · masking "+mask))
	}
	return nil
}

func gib(n uint64) string {
	if n == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.0f GB", float64(n)/(1<<30))
}

func gpuWords(h local.Hardware) string {
	switch {
	case h.GPU != "":
		return muted.Render("· " + h.GPU + " " + gib(h.VRAM))
	case h.Unified:
		return muted.Render("· Apple silicon, memory shared with the GPU")
	}
	return ""
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func localModels() error {
	a := local.Assessed()
	for _, m := range local.Catalog {
		mark := "  "
		if m.ID() == a.Pick {
			mark = green.Render("★ ")
		}
		fmt.Printf("%s%-16s %-30s %.1f GB  %s\n", mark, m.ID(), m.Repo, float64(m.Size)/(1<<30), muted.Render(m.Note))
	}
	if a.Pick == "" {
		fmt.Println(muted.Render("none is recommended for this computer: " + a.Reason()))
	} else {
		fmt.Println(muted.Render("★ recommended for this computer · fogway local check says why · fogway local setup <name> · or any repo and .gguf file"))
	}
	return nil
}

// localCheck measures this computer and says which model suits it, and
// measures the model the gateway runs, if one does.
func localCheck(asJSON bool) error {
	a := localAssess()
	if st := gatewayLocal(); st != nil && st.Phase == local.Ready && st.URL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		s, err := local.Measure(ctx, st.URL)
		cancel()
		if err == nil {
			s.Backend, s.Build = st.Backend, st.Build
			a = local.RecordSpeed(local.Canon(st.Model).ID(), s)
		} else if !asJSON {
			fmt.Fprintln(os.Stderr, muted.Render("measuring "+st.Model.ID()+": "+err.Error()))
		}
	}
	if asJSON {
		b, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	h := a.Hardware
	comp := fmt.Sprintf("%s/%s", h.OS, h.Arch)
	if h.CPU != "" {
		comp = h.CPU
	}
	comp += fmt.Sprintf(" · %d cores · %s memory", h.Cores, gib(h.RAM))
	switch {
	case h.GPU != "":
		comp += " · " + h.GPU + " " + gib(h.VRAM)
	case h.Unified && h.GPUCores > 0:
		comp += fmt.Sprintf(" · GPU %d cores, memory shared", h.GPUCores)
	}
	if a.Disk > 0 {
		comp += " · " + gib(a.Disk) + " free"
	}
	fmt.Println(muted.Render("this computer "), comp)
	fmt.Println(muted.Render("measured      "), fmt.Sprintf("memory read at %.0f GB/s · %.0f billion multiply-adds a second", a.Bench.Bandwidth, a.Bench.Compute))
	fmt.Println()
	fmt.Println(muted.Render(fmt.Sprintf("  %-16s %-8s %-15s %-12s %-6s %s", "", "memory", "judges a turn", "writes", "on", "")))
	for _, e := range a.Models {
		mark := "  "
		if e.Model == a.Pick {
			mark = green.Render("★ ")
		}
		how := ""
		switch {
		case e.Measured:
			how = "measured"
		case e.Corrected:
			how = "estimated, set by a measurement"
		default:
			how = "estimated"
		}
		fmt.Printf("%s%-16s %-8s %-15s %-12s %-6s %s %s\n", mark, e.Model, fmt.Sprintf("%.1f GB", float64(e.Need)/(1<<30)), fmt.Sprintf("%.1f s", e.Judge),
			fmt.Sprintf("%.0f tok/s", e.Gen), strings.ToUpper(e.Where), gradeWord(e.Grade), muted.Render(how))
	}
	fmt.Println()
	if m, ok := local.CatalogModel(a.Pick); ok {
		fmt.Println(green.Render("✓"), "recommended:", bold.Render(m.Title), muted.Render("— "+a.Reason()))
		fmt.Println(muted.Render("  fogway local setup " + m.ID() + " installs it"))
	} else {
		fmt.Println(bold.Render("a local model isn't recommended here"), muted.Render("— "+a.Reason()))
		if a.Why == "runtime" {
			fmt.Println(muted.Render("  group/auto needs one; the cloud models serve agents without it"))
		} else {
			fmt.Println(muted.Render("  group/auto needs one; the cloud models serve agents without it. To try anyway: fogway local setup " + local.Spark17.ID()))
		}
	}
	return nil
}

func gradeWord(g local.Grade) string {
	switch g {
	case local.Smooth:
		return green.Render("smooth")
	case local.Usable:
		return "usable"
	case local.Slow:
		return "too slow"
	case local.Tight:
		return "too much memory"
	case local.NoRoom:
		return "doesn't fit"
	case local.NoDisk:
		return "no disk room"
	case local.NoRuntime:
		return "llama.cpp can't run"
	}
	return string(g)
}

// localSetup downloads llama.cpp and the model in the terminal, then turns
// the local model on.
func localSetup(args []string) error {
	c, err := local.Load()
	if err != nil {
		return err
	}
	var names []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hf", "--hf-endpoint":
			if i+1 >= len(args) {
				return fmt.Errorf("--hf takes an address, such as https://hf-mirror.com")
			}
			c.HFEndpoint, i = args[i+1], i+1
		case "--github-mirror", "--mirror":
			if i+1 >= len(args) {
				return fmt.Errorf("--github-mirror takes a prefix")
			}
			c.GitHubMirror, i = args[i+1], i+1
		default:
			names = append(names, args[i])
		}
	}
	if len(names) > 0 {
		m, err := localModelOf(names)
		if err != nil {
			return err
		}
		c.Model = m
	} else if c.Model.Repo == "" {
		// none named or set: the one this computer suits, measured now
		a := localAssess()
		m, ok := local.CatalogModel(a.Pick)
		if !ok {
			if a.Why == "runtime" {
				return fmt.Errorf("a local model can't run on this computer: %s", a.Reason())
			}
			return fmt.Errorf("a local model isn't recommended for this computer: %s (fogway local check). To install one anyway, name it: fogway local setup %s", a.Reason(), local.Spark17.ID())
		}
		fmt.Println(green.Render("✓"), "this computer suits", bold.Render(m.Title), muted.Render("— "+a.Reason()))
		c.Model = m
	}
	ctx, stop := interruptContext()
	defer stop()
	last := time.Time{}
	tell := func(p local.Progress) {
		if time.Since(last) < 200*time.Millisecond && p.Done != p.Total {
			return
		}
		last = time.Now()
		if p.Total > 0 {
			fmt.Fprintf(os.Stderr, "\r  %s  %s / %s  %3d%%   ", p.What, gib2(p.Done), gib2(p.Total), p.Done*100/p.Total)
		} else {
			fmt.Fprintf(os.Stderr, "\r  %s  %s   ", p.What, gib2(p.Done))
		}
	}
	h := local.Detect()
	m := c.Picked()
	fmt.Println("setting up", bold.Render(m.ID()), muted.Render("("+m.Repo+")"), "for", h.OS+"/"+h.Arch, muted.Render("· llama.cpp "+cmpOr(c.Backend, h.Backend())))
	if _, err := local.Prepare(ctx, c, tell); err != nil {
		fmt.Fprintln(os.Stderr)
		return err
	}
	fmt.Fprintln(os.Stderr)
	c.On = true
	if err := local.Save(c); err != nil {
		return err
	}
	fmt.Println(green.Render("✓"), "ready:", bold.Render(local.Ref(m)), muted.Render("· the app, or fogway serve, runs it; group/"+provider.HybridGroupID+" routes between it and the cloud"))
	return nil
}

func gib2(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}

func localMask(args []string) error {
	g, _, ok := provider.FindGroup(provider.GroupPrefix + provider.HybridGroupID)
	if !ok || g.Hybrid == nil {
		return fmt.Errorf("there is no edge-cloud group yet: fogway local setup makes group/%s once the local model runs", provider.HybridGroupID)
	}
	if len(args) == 0 {
		state := "off"
		if g.Hybrid.Mask {
			state = "on"
		}
		fmt.Println("masking is", state)
		return nil
	}
	if args[0] != "on" && args[0] != "off" {
		return fmt.Errorf("fogway local mask [on|off]")
	}
	h := *g.Hybrid
	h.Mask = args[0] == "on"
	g.Hybrid = &h
	if err := provider.SaveGroup(g); err != nil {
		return err
	}
	fmt.Println(green.Render("✓"), "masking", args[0], muted.Render("· group/"+provider.HybridGroupID))
	return nil
}

// gatewayLocal is the local model's status as the running gateway has it,
// nil when no gateway answers.
func gatewayLocal() *local.Status {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gateway.URL()+"/v1/fogway/local", nil)
	if err != nil {
		return nil
	}
	res, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var s local.Status
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&s) != nil {
		return nil
	}
	return &s
}
