package plugin

// Plugin updates, as magpie keeps itself up to date: the community's
// plugins (@magpie-community/*, the ones the built-in subscriptions move
// onto) update by themselves, a little after magpie starts and every hour; anyone else's new version waits for the reader, who sees a dot
// on Plugins and updates it with a click. A plugin pinned to a version
// stays on it. Updating never cuts a reply streaming through a plugin:
// the host it runs on finishes it (see Restart).

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/steady"
	"github.com/yetone/magpie/internal/update"
)

// Updates is what magpie's last look at npm found for the plugins.
type Updates struct {
	Checked time.Time `json:"checked"`
	// Waiting are the plugins with a newer version that magpie leaves to
	// the reader: someone else's, or one pinned to a version.
	Waiting []Waiting `json:"waiting"`
	// Updated are the plugins magpie updated by itself, the latest last.
	Updated []Updated `json:"updated"`
}

// Waiting is a plugin with a newer version on npm than the one installed,
// or, for one from a git repository (Git), whose repository is at another
// commit than the one installed: Version and Latest are then the two
// commits.
type Waiting struct {
	Spec    string `json:"spec"`
	Package string `json:"package"`
	Version string `json:"version"`
	Latest  string `json:"latest"`
	Git     bool   `json:"git,omitempty"`
}

// Updated is a plugin magpie updated by itself.
type Updated struct {
	Package string    `json:"package"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	At      time.Time `json:"at"`
}

const (
	// updateEvery is how often magpie looks for plugin updates: a fix to a
	// community plugin reaches its users within the hour (one look at npm's
	// metadata for the plugins installed; six hours left someone told of a
	// fix installing it by hand, #681)
	updateEvery = time.Hour
	// bunEvery is how often magpie looks for a newer Bun
	bunEvery = 6 * time.Hour
	// keepUpdated is how many updates magpie remembers making
	keepUpdated = 20
)

// Official is whether the package is the community's, magpie's own,
// which magpie keeps up to date by itself.
func Official(pkg string) bool { return strings.HasPrefix(pkg, "@magpie-community/") }

// Pinned is whether spec names a version (or a range or a tag other than
// latest): the user chose it, and it stays. A git one's version is its
// repository's, not npm's: it is never pinned, and never asked of npm.
func Pinned(spec string) bool {
	if IsPath(spec) || IsGit(spec) {
		return false
	}
	v := strings.TrimPrefix(spec, Name(spec))
	return v != "" && v != "@latest"
}

var (
	updatesMu sync.Mutex
	// latestOf is the version npm has of each package, "" for one it
	// didn't tell (tests stub it)
	latestOf = func(ctx context.Context, names []string) map[string]string {
		out := map[string]string{}
		for n, i := range Info(ctx, names) {
			out[n] = i.Version
		}
		return out
	}
	// installNewest installs the version of the package npm has as its
	// newest (installAt; tests stub it)
	installNewest = installAt
)

func updatesPath() string { return filepath.Join(settings.Dir(), "plugin-updates.json") }

func readUpdates() Updates {
	var u Updates
	if b, err := steady.ReadFile(updatesPath()); err == nil {
		_ = json.Unmarshal(b, &u)
	}
	return u
}

// CheckUpdates asks npm for each plugin's newest version, updates the
// community's (unpinned, switched on) to it and notes the others' as
// waiting. The plugins updated are loaded again, a reply streaming
// through the old ones finishing first.
func CheckUpdates(ctx context.Context) (Updates, error) {
	var es, gits []Entry
	var names []string
	for _, e := range Load().Plugins {
		// a git one's package may be on npm too, as someone else's: its
		// repository is asked instead
		if IsGit(e.Spec) && !e.Off {
			gits = append(gits, e)
		} else if !IsPath(e.Spec) && !IsGit(e.Spec) {
			es = append(es, e)
			names = append(names, Name(e.Spec))
		}
	}
	latest := map[string]string{}
	if len(names) > 0 {
		latest = latestOf(ctx, names)
	}
	var waiting []Waiting
	var made []Updated
	var errs []error
	// a git one waits for the reader whoever's it is: its repository's
	// newest commit is nobody's release
	for _, e := range gits {
		if head, err := GitHead(ctx, e.Spec, true); err == nil && GitBehind(e.Spec, head) {
			waiting = append(waiting, Waiting{Spec: e.Spec, Package: Name(e.Spec), Version: short(GitCommit(e.Spec)), Latest: short(head), Git: true})
		}
	}
	for _, e := range es {
		pkg, have, now := Name(e.Spec), Installed(e.Spec), latest[Name(e.Spec)]
		if e.Off || have == "" || now == "" || !update.Newer(now, have) {
			continue
		}
		if !Official(pkg) || Pinned(e.Spec) {
			waiting = append(waiting, Waiting{Spec: e.Spec, Package: pkg, Version: have, Latest: now})
			continue
		}
		if err := installNewest(ctx, pkg, now); err != nil {
			errs = append(errs, err)
			log.Printf("updating the plugin %s: %s", pkg, err)
			waiting = append(waiting, Waiting{Spec: e.Spec, Package: pkg, Version: have, Latest: now})
			continue
		}
		v := Installed(e.Spec)
		if v == "" {
			v = now
		}
		made = append(made, Updated{Package: pkg, From: have, To: v, At: time.Now().UTC().Truncate(time.Second)})
		log.Printf("updated the plugin %s from %s to %s", pkg, have, v)
	}
	updatesMu.Lock()
	u := readUpdates()
	u.Checked = time.Now().UTC().Truncate(time.Second)
	u.Waiting = waiting
	u.Updated = append(u.Updated, made...)
	if n := len(u.Updated); n > keepUpdated {
		u.Updated = u.Updated[n-keepUpdated:]
	}
	if b, err := json.MarshalIndent(u, "", "  "); err == nil {
		if err := os.MkdirAll(settings.Dir(), 0o700); err == nil {
			_ = writeWhole(updatesPath(), b)
		}
	}
	updatesMu.Unlock()
	if len(made) > 0 {
		Restart()
	}
	if len(errs) > 0 {
		return u, errs[0]
	}
	return u, nil
}

// KeepUpdated looks for plugin updates a little after magpie starts, then
// every updateEvery, run by the magpie serving the gateway (one magpie,
// never two at once).
func KeepUpdated(ctx context.Context) {
	t := time.NewTimer(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if len(Load().Plugins) > 0 {
			cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			_, _ = CheckUpdates(cctx)
			cancel()
		}
		t.Reset(updateEvery)
	}
}

// VersionCheck is what asking npm now found of one installed plugin.
type VersionCheck struct {
	Spec    string `json:"spec"`
	Package string `json:"package"`
	Version string `json:"version"`          // installed
	Latest  string `json:"latest,omitempty"` // npm's newest, when it answered
	// Status is "update" (npm has a newer version), "current", "unknown"
	// (npm didn't say: Why and Error say why), or "folder" for one that
	// has no versions to ask about. For a plugin from a git repository it
	// is its repository's answer: "update" when it is at another commit
	// than Commit (Head), "current", "unknown" (it didn't say), or "git"
	// when no commit can be compared (bun named none it installed).
	Status string `json:"status"`
	// Commit and Head are a git one's commit installed and its
	// repository's now, as the page shows them
	Commit string `json:"commit,omitempty"`
	Head   string `json:"head,omitempty"`
	// Why is why npm didn't say: "offline" (not reached), "limited" (too
	// many requests), "registry" (it answered with an error) or "missing"
	// (it has no such package)
	Why   string `json:"why,omitempty"`
	Error string `json:"error,omitempty"`
	// Limited is set when a git one's repository couldn't be asked
	// because GitHub's rate limit was used up, for the page to say so in
	// its own words, as the library's skills do (#1942)
	Limited *GitHubLimit `json:"limited,omitempty"`
	// Auto is whether magpie updates it by itself (the community's,
	// unpinned, switched on), so an update found would come anyway
	Auto bool `json:"auto"`
	Off  bool `json:"off"`
}

// GitHubLimit is GitHub's rate limit used up: until when (RFC 3339, ""
// when GitHub didn't say), and whether the request carried a GitHub token.
type GitHubLimit struct {
	Until string `json:"until,omitempty"`
	Token bool   `json:"token,omitempty"`
}

// VersionChecks is what a check asked now found.
type VersionChecks struct {
	At      time.Time      `json:"at"`
	Plugins []VersionCheck `json:"plugins"`
}

// checkEach is how long npm is waited for, for each package a check asks
var checkEach = 15 * time.Second
