package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/budget"
)

// magpie's web search, the one a model that can't search is given
// (webSearch: the searcher's model, a Kimi Code plan's service, the
// search APIs), is also an MCP server of its own (Kayphoon on Discord),
// so Cursor, Claude Desktop and any other agent can mount it as a
// web_search tool: Streamable HTTP at SearchMCPPath, JSON answered to each
// POST. It is a gateway route like the others: this computer calls it with
// any key, another one only while magpie is shared and with an enabled
// gateway key (lanGuard), a gateway key's limit holds each search
// (budget.Reserve), and a web page is turned away, as the MCP spec has a
// server check Origin.

// SearchMCPPath is where the web search MCP server is served. It has two
// segments under /mcp/, so it can't hide a signed-in server's /mcp/{name}.
const SearchMCPPath = "/mcp/magpie/web-search"

// searchMCPVersions are the MCP versions the server speaks, newest first:
// a client asking for one of them is answered in it, any other in the first.
var searchMCPVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

const searchMCPTool = "web_search"

type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type searchMCPError string

func (e searchMCPError) Error() string { return string(e) }

// searchMCPRun searches for the query as a model's web_search would be:
// held to the calling gateway key's limit, and named in the Routing view
// as the agent's MCP call.
func (s *Server) searchMCPRun(r *http.Request, query string) (string, error) {
	if query == "" {
		return "", searchMCPError("web_search needs a query")
	}
	release, refused := budget.Reserve(access.Caller(r.Context()), int64(len(query)), "", limitClock())
	if refused != nil {
		return "", refused
	}
	defer release()
	ctx := context.WithValue(r.Context(), searchForKey{}, &CallFor{Agent: callerOf(r).agent, MCP: true})
	said, _, err := s.webSearch(ctx, query)
	if err != nil {
		if r.Context().Err() == nil {
			log.Printf("web search MCP: %v", err)
		}
		msg := "magpie couldn't search the web: " + err.Error()
		if errors.Is(err, errNoSearcher) {
			msg += ": add a search API in magpie's Settings → Models → Web search (or magpie search add <api> <key>), or a provider that can search"
		}
		return "", searchMCPError(msg)
	}
	return said, nil
}
