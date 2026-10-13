// Package claudecode finds the Claude Code magpie's Claude subscription
// runs, and downloads Anthropic's own build of it when the user asks, for a
// machine with none: a server or a container (Jorben on Discord).
//
// The build comes from where Anthropic's installer (claude.ai/install.sh)
// takes it: the release's manifest.json names each platform's SHA-256 and
// size, and the binary is checked against both before it is kept. It is
// kept in magpie's cache folder, not on PATH, so it stands in for no agent:
// it is only what the bridge and the sign-in run.
package claudecode

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/proc"
)

// Base is where Anthropic publishes Claude Code's builds, as its installer
// downloads them; a var for tests.
var Base = "https://downloads.claude.ai/claude-code-releases"

// Root is the folder magpie keeps the Claude Code it downloaded in, one
// folder per version.
func Root() string { return filepath.Join(appdir.Cache(), "claude-code") }

func exeName(goos string) string {
	if goos == "windows" {
		return "claude.exe"
	}
	return "claude"
}

var versionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// Downloaded is the Claude Code magpie downloaded and its version, or ""
// when there is none: the newest of Root's versions that has the binary.
func Downloaded() (path, version string) {
	ents, err := os.ReadDir(Root())
	if err != nil {
		return "", ""
	}
	for _, e := range ents {
		v := e.Name()
		if !e.IsDir() || !versionRe.MatchString(v) {
			continue
		}
		p := filepath.Join(Root(), v, exeName(runtime.GOOS))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		if version == "" || newer(v, version) {
			path, version = p, v
		}
	}
	return path, version
}

// newer is whether version a is after b (numbers compared one by one).
func newer(a, b string) bool {
	pa, pb := strings.SplitN(strings.SplitN(a, "-", 2)[0], ".", 3), strings.SplitN(strings.SplitN(b, "-", 2)[0], ".", 3)
	for i := range 3 {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x > y
		}
	}
	return a > b
}

// Find is the Claude Code to run on this machine: the user's own (on PATH
// or where its installers put it, proc.FindTool), else the one magpie
// downloaded; "" when there is neither.
func Find() string {
	if p := proc.FindTool("claude"); p != "" {
		return p
	}
	p, _ := Downloaded()
	return p
}

// InstallHint is what an error about a missing Claude Code adds: how to
// have magpie download it.
const InstallHint = "or run `magpie claude-code install` to download Anthropic's own build for magpie"

// Release is a build of Claude Code for this machine, as the manifest
// names it.
type Release struct {
	Version  string
	Platform string
	URL      string
	Checksum string // SHA-256, hex
	Size     int64
}

type progressFunc func(int64)

func (f progressFunc) Write(p []byte) (int, error) {
	f(int64(len(p)))
	return len(p), nil
}

// Remove deletes every Claude Code magpie downloaded.
func Remove() error { return os.RemoveAll(Root()) }
