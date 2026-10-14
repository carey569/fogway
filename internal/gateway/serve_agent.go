package gateway

import (
	"net/http"

	"github.com/carey569/fogway/internal/provider"
)

// serveAgent is serve for a request an agent sent: it is known by the
// agent's session, and recorded when conversations are. fogway's own
// requests through serve (titles, the router's classifier, search and
// vision stand-ins) go without either.
func (s *Server) serveAgent(w http.ResponseWriter, r *http.Request, from provider.Protocol, body []byte) {
	withGatewaySession(w, r)
	w, recorded := recordConversation(w, r, from, body)
	defer recorded()
	s.serve(w, r, from, body)
}
