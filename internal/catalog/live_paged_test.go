package catalog

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// A vendor's list of n models, claude-1 … claude-n, answered a page at a
// time as Anthropic's own is: after_id names the id the next page starts
// after, and has_more says whether one is to come. The page is size ids
// long unless the question asks for more (limit, which Anthropic takes),
// and every question is kept, so a test can see what the fetch carried on
// from.
type pagedList struct {
	mu    sync.Mutex
	n     int
	size  int
	asked []string
	fixed bool // a relay that pages its own way and takes no limit
}

func (l *pagedList) serve(rw http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	after := r.URL.Query().Get("after_id")
	l.asked = append(l.asked, after)
	start := 0
	if after != "" {
		i, err := strconv.Atoi(strings.TrimPrefix(after, "claude-"))
		if err != nil || i < 1 || i >= l.n {
			http.Error(rw, "no such id: "+after, http.StatusBadRequest)
			return
		}
		start = i // after claude-7 the list goes on with claude-8
	}
	size := l.size
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && !l.fixed {
		size = n
	}
	end := min(start+size, l.n)
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := start + 1; i <= end; i++ {
		if i > start+1 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"claude-%d","name":"Claude %d","supported_endpoints":["/messages"]}`, i, i)
	}
	more := end < l.n
	last := ""
	if more {
		last = fmt.Sprintf("claude-%d", end)
	}
	fmt.Fprintf(&b, `],"has_more":%t,"first_id":"claude-%d","last_id":%q}`, more, start+1, last)
	rw.Write([]byte(b.String()))
}

func claudeIDs(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("claude-%d", i))
	}
	return out
}
