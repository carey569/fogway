package gateway

import (
	"crypto/sha256"
)

// A client may send its conversation back with the images and files of
// earlier messages taken out (#1382): Hana's pre-step hook replaces each
// image before the last reply with a line of text, in the tool result or
// user message that held it, so pi-ai's Chat Completions body no longer
// has the user message of the tool results' images ("Attached image(s)
// from tool result:" and the images) and each result's text has the line
// added to its end. The run that had the conversation was given the
// images, and its Claude Code still holds them: told the conversation
// anew, it would write all of it to the prompt cache again, at each step
// after one that read an image. lostMedia tells this rewrite from any
// other by its shape, not the client's words for it.

// wordPart is a part of a message as hashMessages reads it: its kind and
// its words, and for a tool result how many images it carried beside them.
type wordPart struct {
	kind  Kind
	words string
	media int
}

// heardPart is a wordPart as a run keeps it: the hash and length of its
// words. words is set only on the conversation compared with what a run
// kept, which is a request's own.
type heardPart struct {
	kind  Kind
	n     int
	sum   [sha256.Size]byte
	media int
	words string
}

// heardRun is a role's messages in a row, as hashMessages runs them
// together: how a reply is split into messages is left out.
type heardRun struct {
	role  string
	parts []heardPart
}
