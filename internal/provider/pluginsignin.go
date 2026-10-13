package provider

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/plugin"
)

// pluginRunsCLI are the plugins (by OpenCode's id) that sign in with the
// vendor's CLI themselves, as the built-in did: the community Grok plugin
// runs `grok login`. One installed beside the built-in, not moved onto,
// said "install Grok Build first: curl … | bash", which a magpie in Docker
// has no shell to run; its CLI is installed first, as a moved one's is.
var pluginRunsCLI = map[string]bool{"grok": true}

// deviceCode is the code a device sign-in's instructions ask to check on
// the vendor's page ("Confirm the code ABCD-EFGH on Factory's page").
var deviceCode = regexp.MustCompile(`(?i:\bcode):?\s+([A-Z0-9]+(?:-[A-Z0-9]+)+|[A-Z0-9]{6,})\b`)

// pluginCode finishes a plugin's sign-in with the code its page showed.
func (s *signInFlow) pluginCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("paste the code the sign-in page showed")
	}
	s.mu.Lock()
	session := s.plugin
	s.plugin = ""
	waiting := s.st.State == "waiting"
	s.mu.Unlock()
	if session == "" || !waiting {
		return errors.New("this sign-in is over; start it again")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	saved, err := plugin.Finish(ctx, session, code)
	s.pluginDone(saved, err)
	return err
}

func (s *signInFlow) pluginDone(saved plugin.Saved, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		s.finish(SignInState{State: "canceled"})
	case errors.Is(err, context.DeadlineExceeded):
		s.finish(SignInState{State: "failed", Error: "the sign-in timed out; start it again"})
	case err != nil:
		s.finish(SignInState{State: "failed", Error: err.Error()})
	default:
		clearPluginLapse(saved)
		user := pluginUser(saved)
		s.finish(SignInState{State: "done", User: user, Using: pluginUsing(saved, user)})
	}
}

// pluginUsing is whether the account signed in to is the one in use, the
// first, as a built-in's sign-in says.
func pluginUsing(saved plugin.Saved, user string) bool {
	for _, l := range Logins(PluginID(saved.Provider)) {
		if l.Active {
			return user != "" && strings.EqualFold(l.User, user)
		}
	}
	return false
}

// pluginUser is the account signed in to, as the accounts list names it.
func pluginUser(saved plugin.Saved) string {
	for _, pp := range plugin.Cached() {
		if pp.ID == saved.Provider {
			return pluginLabels(pp)[saved.Account]
		}
	}
	return ""
}
