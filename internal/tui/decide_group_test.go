package tui

import (
	"slices"
	"testing"

	"github.com/fogway/fogway/internal/provider"
)

// The member picker offers the models kept out of the lists agents see:
// a provider's kept for routing groups and a group's with listed=no
// (#1783), as the Routing view's does. They are what groups are made of.
func TestMemberOptionsOfferUnlistedModels(t *testing.T) {
	home(t)
	for _, p := range []provider.Provider{
		{ID: "ua", Name: "UA", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}},
		{ID: "ub", Name: "UB", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"n1"}, Unlisted: true},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := provider.SaveGroup(provider.Group{ID: "g", Name: "G", Members: []string{"ua/m1"}, Unlists: true}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range memberOptions(nil, "other") {
		got = append(got, o.Value)
	}
	if !slices.Contains(got, "ua/m1") || !slices.Contains(got, "ub/n1") || !slices.Contains(got, "group/g") {
		t.Fatalf("offered %v", got)
	}
}
