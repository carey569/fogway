package gateway

import (
	"context"
	"net/http"
	"strings"

	"github.com/fogway/fogway/internal/provider"
)

// fogwayUA is the User-Agent fogway's requests go to a vendor with.
func fogwayUA() string { return "fogway/" + Version }

// userAgentFor is the User-Agent a request to p goes with, ua being the
// one the request came to fogway with: the agent's own, as it sent it,
// where p is set to pass it on (provider.PassesUserAgent, #1851), and
// fogway's where it isn't, where the agent sent none, and on fogway's own
// requests — its classifier's, vision's, search's, the image tool's
// (fogwaysOwn) — which no agent sent. A User-Agent the user set among the
// provider's headers is put on after, when the request is signed, and
// stands over either.
func userAgentFor(p provider.Provider, ua string) string {
	if ua != "" && p.PassesUserAgent() && !fogwaysOwn(ua) {
		return ua
	}
	return fogwayUA()
}

// fogwaysOwn is a User-Agent fogway's own requests carry: another fogway's
// (fogway/…), the classifier's, vision's and search's (fogway-router/1,
// fogway-vision/1, fogway-search/1), and the image tool's, which names the
// agent it draws for before its own (claude-code/2.1.0 fogway-image/1)
// from what the agent told it, not from a User-Agent the agent sent.
func fogwaysOwn(ua string) bool {
	return strings.HasPrefix(ua, "fogway/") || strings.HasPrefix(ua, "fogway-")
}

// callerUA is the User-Agent the request ctx is for came with (withCaller),
// "" when there is none.
func callerUA(ctx context.Context) string {
	c, _ := ctx.Value(callerCtx{}).(caller)
	return c.ua
}

// stampUA puts on req, a request to p that isn't fogway's own forwarded
// one (an embedding, a drawing, a video), the User-Agent forwardOnce's
// carry: fogway's, or the agent's where p passes it on. A signed-in
// account's is left to its signing, as before, and another fogway's is
// set where it is asked.
func stampUA(ctx context.Context, p provider.Provider, h http.Header) {

	h.Set("User-Agent", userAgentFor(p, callerUA(ctx)))
}
