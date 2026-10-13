package gateway

// The edge-cloud route's judge: as a user's turn begins, the group's local
// model is asked two things of the user's message — can a small model on
// this computer do it well, and does it carry what shouldn't leave the
// computer — and answers in JSON. Asked with no reasoning: it is one short
// answer, on the turn's critical path.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// judgement is what the local model said of a turn.
type judgement struct {
	Local     bool   `json:"local"`     // a small local model does it well
	Sensitive bool   `json:"sensitive"` // it carries secrets, personal or confidential data
	Why       string `json:"why,omitempty"`
}

const judgePrompt = `You are the router of a coding assistant that runs a small model on the user's computer and can call a large cloud model.
Read the user's message and answer two questions.

local: can a small local model (a few billion parameters) do this well on its own?
  true for: greetings, short questions, explaining a snippet, small and local edits, renaming, formatting, writing a commit message or a comment, simple shell commands, summarizing a short text.
  false for: designing or refactoring across files, hard debugging, writing a feature or many lines of code, anything that needs deep reasoning or broad knowledge, long documents.

sensitive: does the message carry what must not leave this computer?
  true for: passwords, API keys, tokens, private keys, connection strings; personal data (ID or phone numbers, addresses, names of real customers or employees with their details); internal hostnames, IPs, or confidential business data; or the user says it is confidential or internal.
  false for: ordinary code and questions, public libraries, generic examples.

First name the task itself in a few words, leaving out the data it carries ("fix a typo in a comment"); judge local by that task alone. A data that must not leave the computer makes a message sensitive, never too hard for the local model: a small task with a password in it is local and sensitive.

Answer with JSON only, no other words: {"task": "the task in a few words", "local": true or false, "sensitive": true or false, "why": "a few words"}`

// judgeBody is the Chat request asking model to judge text. prev, when
// set, is what the turn before was judged, for a message that only carries
// on ("go on", "yes", "do it"), which is judged as what it goes on with.
func judgeBody(model, text string, prev *judgement) []byte {
	var b strings.Builder
	if prev != nil {
		fmt.Fprintf(&b, "The turn before in this conversation was judged local=%t, sensitive=%t. "+
			"A message that only carries on from it is judged the same.\n\n", prev.Local, prev.Sensitive)
	}
	b.WriteString("The user's message:\n<message>\n")
	b.WriteString(text)
	b.WriteString("\n</message>")
	req := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": judgePrompt},
			{"role": "user", "content": b.String()},
		},
		"stream":      false,
		"temperature": 0,
		"max_tokens":  200,
		// llama.cpp's chat template: Qwen and the like answer at once
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
		"reasoning_effort":     "none",
		"response_format":      map[string]any{"type": "json_object"},
	}
	body, _ := json.Marshal(req)
	return body
}

var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)

// readJudgement reads the judge's answer: its JSON object, wherever in the
// text it is, else the words true/false after each name.
func readJudgement(answer string) (judgement, error) {
	var j judgement
	if m := jsonObject.FindString(answer); m != "" {
		var raw map[string]any
		if json.Unmarshal([]byte(m), &raw) == nil {
			l, lok := truth(raw["local"])
			s, sok := truth(raw["sensitive"])
			if lok && sok {
				j.Local, j.Sensitive = l, s
				j.Why, _ = raw["why"].(string)
				return j, nil
			}
		}
	}
	low := strings.ToLower(answer)
	l, lok := wordAfter(low, "local")
	s, sok := wordAfter(low, "sensitive")
	if lok && sok {
		return judgement{Local: l, Sensitive: s}, nil
	}
	a := []rune(strings.TrimSpace(answer))
	if len(a) > 80 {
		a = append(a[:80], '…')
	}
	return j, answerError{fmt.Errorf("it answered %q, not {\"local\": …, \"sensitive\": …}", string(a))}
}

func truth(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	case float64:
		return x != 0, true
	}
	return false, false
}

// afterName finds the true or false a judge wrote after a name, outside JSON.
var afterName = map[string]*regexp.Regexp{
	"local":     regexp.MustCompile(`local\W{0,4}(true|false|yes|no)`),
	"sensitive": regexp.MustCompile(`sensitive\W{0,4}(true|false|yes|no)`),
}

func wordAfter(s, name string) (bool, bool) {
	m := afterName[name].FindStringSubmatch(s)
	if m == nil {
		return false, false
	}
	return m[1] == "true" || m[1] == "yes", true
}
