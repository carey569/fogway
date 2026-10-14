package agent

import (
	"maps"
	"slices"
	"strings"

	"github.com/carey569/fogway/internal/provider"
)

// Models are fogway's models the agent is set to, as catalog ids
// (provider/model, group/<id>), each once: the agent's own models aside.
// Usage stats count them (internal/stats).
func (a *Agent) Models() []string {
	var out []string
	add := func(r string) {
		if r != "" && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	if a.Native != nil {
		st := a.Native.Read()
		if st.Provider != "connected" {
			return nil
		}
		for _, k := range slices.Sorted(maps.Keys(st.Fields)) {
			add(modelRef(st.Fields[k].Value))
		}
		return out
	}
	vals := a.Values()
	for _, f := range a.Fields {
		v := vals[f.Key]
		if !fogwayValue(a, f, v, vals) {
			continue
		}
		m, _, _ := a.split(v)
		r := modelRef(m)
		if r == "" && f.Options != nil {
			for _, o := range f.Options(vals) {
				if o.Value == m {
					r = o.Ref
					break
				}
			}
		}
		add(r)
	}
	return out
}

// modelRef is v as a catalog id when it names one of fogway's models or
// groups, as an agent spells it (fogway/…, Claude Code's [1m]); "" else.
func modelRef(v string) string {
	v = strings.TrimSuffix(strings.TrimSpace(v), "[1m]")
	v = strings.TrimPrefix(v, fogwayID+"/")
	if strings.HasPrefix(v, provider.GroupPrefix) || isFogway(v) {
		return v
	}
	return ""
}
