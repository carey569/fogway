package provider

// The cards another magpie is told (CachedCards) as fresh as this magpie
// knows them, not only as the Usage page last read them (jorben, #1313).
// A magpie that serves only other magpies (`magpie serve` in a container)
// has nobody on its Usage page, so the page's cards were never read
// there, while the same accounts' allowances are read all the time
// behind the requests: the account switching and the Codex warm-up
// (LoginUsage), routing (Allowances, KeyAllowance). Those readings now
// stand on the cards in place of an older one, an account or key with no
// card yet gets one from them, and after a restart, before anything is
// read, the cards are the ones kept on disk (quotas.json, as of when they
// were read). Nothing here asks a vendor.

import (
	"strings"
	"time"

	"github.com/yetone/magpie/internal/plugin"
)

// cardFace is what a card is named and drawn with.
type cardFace struct{ name, icon string }

// accountCard is the face of the card the Usage page gives each account
// of agent, as its usage is read (LoginUsage): a plugin's ("plugin:<id>")
// as the plugin's cards are.
func accountCard(agent string) cardFace {
	if pp, ok := pluginOfAgent(agent); ok && strings.HasPrefix(agent, "plugin:") {
		return pluginCardFace(pp)
	}
	switch agent {
	case "claude":
		return cardFace{"Claude Code", "claude-color"}
	case "codex":
		return cardFace{"Codex", "codex-color"}
	case "copilot":
		return cardFace{"Copilot", "githubcopilot"}
	case QoderCNID:
		return cardFace{"Qoder CN", "qoder"}
	case "gemini", "antigravity":
		app, _ := googleAppOf(agent)
		return cardFace{app.name, app.icon}
	}
	if c, ok := movedCards[agent]; ok {
		return cardFace{c.name, c.icon}
	}
	return cardFace{}
}

func pluginCardFace(pp plugin.Provider) cardFace {
	name, icon := pluginCard(pp)
	return cardFace{name, icon}
}

// cardAgents are the agents whose accounts each have a card of their own
// on the Usage page, read as LoginUsage reads them. Cursor's account is
// known by asking its CLI, Grok's are read by home, and a Kiro key has
// one card for itself: their cards stay as the page read them.
var cardAgents = []string{"claude", "codex", "copilot", "kiro", "zcode", "workbuddy", WorkBuddyAIID,
	CommandCodePlanID, "qoder", QoderCNID, "zed", "devin", "factory", MiMoID, "gemini", "antigravity"}

// cardKey is one of p's keys as its plan and balance cards are read with
// it: the key, and the account the cards name it by ("" for a provider's
// one card), as PlanQuotas and KeyBalances name them.
type cardKey struct {
	user string
	key  string
}

// keyReading is a key's windows as routing read them, with the plan or
// balance read with them, for its card.
type keyReading struct {
	ws      []QuotaWindow
	plan    string
	balance string
	parts   []BalancePart
	at      time.Time
	seq     uint64 // quotaReading's, begun as it was asked
}
