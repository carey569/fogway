package usage

import (
	"testing"
)

func TestPurposeFilter(t *testing.T) {
	for _, tc := range []struct {
		purpose string
		kinds   []string
	}{
		{"kind:thread_title", []string{"thread_title", "thread_title_reconsideration", "title_generation", "title"}},
		{"kind:guardian", []string{"guardian", "auto_review", "guardian_review"}},
		{"kind:memory_consolidation", []string{"memory_consolidation", "memgen", "memory"}},
		{"kind:collab_spawn", []string{"collab_spawn", "thread_spawn", "agent_job"}},
		{"kind:ambient_suggestions", []string{"ambient_suggestions", "ambient_suggestion_safety"}},
		{"kind:future_kind", []string{"future_kind"}},
		{"kind:unmarked", []string{"unmarked"}},
		{"unmarked", []string{""}},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			f := Filter{Purpose: tc.purpose, Agent: "codex", Failed: true}
			for _, kind := range tc.kinds {
				r := Record{Kind: kind, Agent: "codex", Status: 429}
				if !f.keeps(r) {
					t.Fatalf("%q did not keep %q", tc.purpose, kind)
				}
				r.Status = 200
				if f.keeps(r) {
					t.Fatal("purpose bypassed the failure filter")
				}
			}
			if f.keeps(Record{Kind: "different_unknown", Agent: "codex", Status: 429}) {
				t.Fatal("unknown purposes were combined")
			}
		})
	}
}

func TestDescriptionPurposeLabel(t *testing.T) {
	p := PurposeKinds()["thread_description"]
	if p.Purpose != "kind:thread_description" || p.Name != "Description" {
		t.Fatalf("description purpose: %+v", p)
	}
}
