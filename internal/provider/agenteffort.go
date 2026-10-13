package provider

import (
	"strings"
)

// An agent whose own config can't carry a reasoning effort (Cursor Private
// Inference, #1003: neither its environment nor its app picks one for most
// models) has one picked in fogway, kept by agent (settings' AgentEfforts),
// and asked for by the gateway on the agent's requests to a model that
// reasons, in place of the level the agent sent: the user picked it, and
// Cursor's own, where it has a Reasoning control, is its default as often
// as a choice. None picked, the agent's requests go as it asks.

// AgentEffort is the effort agent's requests are asked for, "" for none.
func AgentEffort(agent string) string {
	return heldSettings().AgentEfforts[strings.ToLower(agent)]
}
