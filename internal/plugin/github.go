package plugin

import (
	"sync"
	"time"
)

// Plugins on GitHub that nobody listed: a repository its author tagged
// with the magpie-plugin topic. The market shows them apart from the
// community's, as not reviewed by anyone: installed from npm when its
// author published it there from the repository, else from the repository
// itself (github:owner/repo).

// Topic is the GitHub topic that puts a repository on the Plugins page.
const Topic = "magpie-plugin"

// Tagged is a repository tagged with Topic.
type Tagged struct {
	Spec        string    `json:"spec"`  // what Install adds: its npm name when npm has it from this repository, else github:owner/repo
	Repo        string    `json:"repo"`  // owner/repo
	URL         string    `json:"url"`   // its page on GitHub
	Owner       string    `json:"owner"` // its owner's login
	OwnerAvatar string    `json:"ownerAvatar,omitempty"`
	Description string    `json:"description,omitempty"`
	Stars       int       `json:"stars"`
	License     string    `json:"license,omitempty"` // SPDX id
	Pushed      time.Time `json:"pushed"`
	// from its package.json on the default branch: what bun will install
	// it as, its version, and whether it is gateway middleware or an agent
	Package string `json:"package,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"` // "middleware" or "agent" when that is all it is
	// its package.json's magpie.icon, as an installed plugin gives its
	// provider one: an https URL or a data:image URI, as it is written
	Icon string `json:"icon,omitempty"`
}

// Where GitHub is asked, moved by tests.
var (
	githubAPI = "https://api.github.com"
	githubRaw = "https://raw.githubusercontent.com"
)

// taggedTTL is how long what GitHub said stands: short, so a repository
// tagged a moment ago shows within minutes (lee04052822 on X tagged theirs
// and, asked six hours apart, didn't see it). A search is one request of
// the ten a minute; what each repository's package.json and npm said is
// kept while its last push stays the same, so asking again costs no more.
const taggedTTL = 10 * time.Minute

// taggedWait is how long the page waits for GitHub; taggedEach bounds a
// repository's reads (package.json, npm, its entry file), which go on past
// that wait and are kept for the next ask; taggedAtOnce repositories are
// read at once; and a list that left a repository out for want of an
// answer is asked again after taggedRetry. Through a proxy far away each
// read takes a few hundred milliseconds (larchsis on X: a list cut short
// at the wait was kept ten minutes).
var (
	taggedWait   = 8 * time.Second
	taggedEach   = 20 * time.Second
	taggedAtOnce = 12
	taggedRetry  = 20 * time.Second
)

var (
	taggedMu sync.Mutex
	taggedL  []Tagged
	taggedAt time.Time
	// owner/repo@pushed → what its package.json and npm made of it, nil
	// when it was left out
	readMu   sync.Mutex
	readRepo = map[string]*Tagged{}
)
