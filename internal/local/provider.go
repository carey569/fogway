package local

import (
	"log"
	"slices"

	"github.com/fogway/fogway/internal/provider"
)

// ProviderID is the provider the gateway serves the local model under:
// agents and routing groups name it local/<model>.
const ProviderID = "local"

// Ref is the local model's id in the catalog: local/<model>.
func Ref(m Model) string { return ProviderID + "/" + m.ID() }

func init() {
	// the provider follows llama-server: its address (the port may move
	// from one run to the next) and its model, once it serves
	OnChange(func(s Status) {
		if s.Phase != Ready {
			return
		}
		if err := register(s); err != nil {
			log.Println("local model: keeping its provider:", err)
		}
	})
}

// register keeps the local provider at llama-server's address with its
// model picked. What else the user set on it (its fallbacks, its window,
// a model's price) stays.
func register(s Status) error {
	id := s.Model.ID()
	p, err := provider.Find(ProviderID)
	if err != nil || p == nil {
		return provider.Save(provider.Provider{
			ID: ProviderID, Name: "Local model", Icon: "llamacpp",
			Chat: s.URL, Models: []string{id},
			// on this computer: what it is sent needn't be masked
			Unredacted: true,
			// the window it runs at, not the one it was trained for that
			// llama-server lists: past it a turn moves to the cloud
			Contexts: map[string]int{id: s.Context},
		})
	}
	if p.Chat == s.URL && slices.Equal(p.Models, []string{id}) && p.Contexts[id] == s.Context {
		return nil
	}
	p.Chat = s.URL
	p.Models = []string{id} // llama-server serves the one model it loaded
	p.Contexts = map[string]int{id: s.Context}
	return provider.Save(*p)
}
