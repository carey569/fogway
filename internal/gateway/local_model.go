package gateway

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"time"

	lm "github.com/fogway/fogway/internal/local"
	"github.com/fogway/fogway/internal/provider"
)

// keepLocal runs the local model while the gateway serves, as local.json
// says, and follows a change to it: the app's Local page, or fogway local,
// writing it starts, restarts or stops llama-server within seconds.
func keepLocal(ctx context.Context) {
	var was []byte
	for {
		b, _ := os.ReadFile(lm.Path())
		if !bytes.Equal(b, was) {
			was = b
			switch c, err := lm.Load(); {
			case err != nil:
				log.Println("local model:", lm.Path()+":", err)
			case c.On:
				lm.Start(ctx, c)
			default:
				lm.Stop()
			}
		}
		select {
		case <-ctx.Done():
			lm.Stop()
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func init() {
	// once the local model serves, group/auto routes between it and the cloud
	lm.OnChange(func(s lm.Status) {
		if s.Phase != lm.Ready {
			return
		}
		if _, err := provider.EnsureHybrid(lm.Ref(s.Model)); err != nil {
			log.Println("local model: the edge-cloud group:", err)
		}
	})
}

// localStatus answers GET /v1/fogway/local: the local model as this
// gateway runs it, for fogway local and the app.
func (s *Server) localStatus(w http.ResponseWriter, r *http.Request) {
	if !local(r) {
		writeError(w, provider.Chat, http.StatusForbidden, "the local model's status is told to this computer only")
		return
	}
	writeJSON(w, 200, lm.Now())
}
