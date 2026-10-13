package gateway

import (
	"testing"
	"time"
)

// A route kept in the Routing page's history before Routed was, marked
// swapped for the member a remote fogway's group answered with, reads as
// routed; a real swap kept beside it is still one.
func TestRemoteFogwayGroupHistory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	saveRoute(Route{ID: 1, Time: now, Model: "office/group/auto-deepseek-v4-1-flash", Done: true, Status: 200,
		Tries:  []Try{{ID: "office", Model: "group/auto-deepseek-v4-1-flash", Done: true, Status: 200, Served: "deepseek/deepseek-v4.1-flash", Swapped: true}},
		Served: "deepseek/deepseek-v4.1-flash", Swapped: true})
	saveRoute(Route{ID: 2, Time: now.Add(time.Second), Model: "office/fake/sol", Done: true, Status: 200,
		Tries:  []Try{{ID: "office", Model: "fake/sol", Done: true, Status: 200, Served: "luna", Swapped: true}},
		Served: "luna", Swapped: true})
	_, rs, _ := History(now.Format(dayForm))
	if len(rs) != 2 {
		t.Fatalf("routes %+v", rs)
	}
	if g := rs[0]; g.Swapped || !g.Routed || g.Tries[0].Swapped || !g.Tries[0].Routed {
		t.Errorf("the group's route %+v", g)
	}
	if s := rs[1]; !s.Swapped || s.Routed || !s.Tries[0].Swapped || s.Tries[0].Routed {
		t.Errorf("the swap's route %+v", s)
	}
}
