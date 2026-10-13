package provider

import (
	"slices"
	"strings"
)

// Hybrid makes a routing group Fogway's edge-cloud route: its Local member,
// a model on this computer, judges each user's turn as it begins — can it
// do the turn itself, and does the turn carry what mustn't leave this
// computer — and the turn goes to it, or to the group's other members (the
// cloud), as the gateway's hybrid.go decides:
//
//   - a turn the local model can do goes to it; should it fail within the
//     turn, the cloud takes the rest of the turn — unless the turn is
//     sensitive and Mask is on, when it stays on this computer whatever
//     happens;
//   - any other turn goes to the cloud, masked when it is sensitive and Mask
//     is on: the local model names the strings that mustn't leave, which go
//     as placeholders, with secrets and personal data, and come back in the
//     reply;
//   - a cloud that doesn't answer leaves the turn to the local model.
//
// With Mask off, what is sensitive decides nothing: the route goes by what
// the local model can do alone.
type Hybrid struct {
	Local string `json:"local"`          // the member on this computer, as Members spells it
	Mask  bool   `json:"mask,omitempty"` // 脱敏: sensitive turns stay local or go masked
}

// HybridGroupID is the edge-cloud group Fogway keeps for the local model:
// group/auto, which agents are pointed at by default.
const HybridGroupID = "auto"

// EnsureHybrid keeps group/auto the edge-cloud route over local, the
// local model's catalog id, and the cloud: on first run, local and the
// first ready model of another provider; later, local in place of whatever
// local model it had, the user's cloud members and masking kept. It
// reports whether it saved a change.
func EnsureHybrid(local string) (bool, error) {
	if local == "" {
		return false, nil
	}
	f, err := read()
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(f.Groups, func(g Group) bool { return g.ID == HybridGroupID })
	if i >= 0 {
		g := f.Groups[i]
		if g.Hybrid == nil {
			return false, nil // the user made group/auto a group of their own
		}
		if g.Hybrid.Local == local && slices.Contains(g.Members, local) {
			return false, nil
		}
		old := g.Hybrid.Local
		g.Members = slices.DeleteFunc(slices.Clone(g.Members), func(m string) bool { return m == old || m == local })
		g.Members = append([]string{local}, g.Members...)
		g.Hybrid = &Hybrid{Local: local, Mask: g.Hybrid.Mask}
		return true, SaveGroup(g)
	}
	members := []string{local}
	for _, e := range Catalog() {
		if e.Group == "" && e.Provider.ID != strings.SplitN(local, "/", 2)[0] && e.Provider.Ready() {
			members = append(members, e.ID)
			break
		}
	}
	g := Group{ID: HybridGroupID, Name: "Edge-cloud auto", Members: members, Hybrid: &Hybrid{Local: local}}
	return true, SaveGroup(g)
}
