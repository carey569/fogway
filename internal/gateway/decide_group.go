package gateway

// A decision group (provider/decide_group.go) is asked at /v1/systemone as
// a chat group is asked a conversation (ARNO on Discord: 给decision模型也加
// 上group功能): "group/<id>", or the group's id or name, goes to its models'
// keys and accounts in the group's routing order, and one that fails as a
// chat request would fail over (retryable) rests, the next asked, each try
// in the routing log.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/provider"
)

// decideTries are the keys and accounts of a decision group's models, in
// the group's routing order — those resting after the rest, as for a
// chat — with the models of it that answer no System One left out. left
// and order are what the routing log shows of the plan.
func (s *Server) decideTries(g provider.Group, ms []provider.Member) ([]candidate, planned, []provider.Member) {
	ms = slices.DeleteFunc(slices.Clone(ms), func(m provider.Member) bool { return !m.Provider.DecidesModel(m.Model) })
	if len(ms) == 0 {
		return nil, planned{}, nil
	}
	cs, pl := s.planGroup(g, ms, provider.Chat)
	return cs, pl, ms
}

// decideFailed rests a candidate whose System One call failed as status,
// header and body say, the way a chat's failed try rests: not when the
// request itself was at fault (its shape, its prompt, its content), which
// would fail the same at any. It returns why it failed, and its rest when
// it rests.
func (s *Server) decideFailed(c candidate, status int, header http.Header, body []byte) (string, *Rest) {
	if shapeRefused(status, body) || promptRefused(status, body) || protectionRefused(status, body) || !retryable(status, body) {
		return failureOf(c, status, body), nil
	}
	rest := s.restAfter(c, status, header, body)
	return rest.Why, &rest
}

// decideRefused is a System One call a decision provider answered with a
// failure: its status, headers and body, for a decision group's classifier
// to rest the one that failed as a chat's try would rest.
type decideRefused struct {
	err    error
	status int
	header http.Header
	body   []byte
}

func (e decideRefused) Error() string { return e.err.Error() }
func (e decideRefused) Unwrap() error { return e.err }

// askDecisionGroup is askClassifier for a decision group: its keys and
// accounts in the group's order, each asked as Jev is (askJev) until one
// answers, the one that failed resting as it would for a chat. ok is false
// when model names no decision group.
func (s *Server) askDecisionGroup(model string, intents []string, prev before, effort bool, text string) (v verdict, ok bool, err error) {
	if !strings.HasPrefix(model, provider.GroupPrefix) {
		return verdict{}, false, nil
	}
	g, ms, found := provider.FindGroup(model)
	if !found || !g.Decides() {
		return verdict{}, false, nil
	}
	g = g.Live()
	ms, _ = provider.PausedOut(g, ms, RouterAgent, "", ruleClock())
	cs, _, _ := s.decideTries(g, ms)
	cs = probeOne(cs) // of those resting, only the one back soonest (#1799)
	if len(cs) == 0 {
		return verdict{}, true, fmt.Errorf("no decision model of the group %s can be asked now", g.Name)
	}
	for _, c := range cs {
		if v, err = s.askJev(c.p, c.model, intents, prev, effort, text); err == nil {
			servedCandidate(c, 0)
			return v, true, nil
		}
		var refused decideRefused
		var answer answerError
		switch {
		case errors.As(err, &refused):
			s.decideFailed(c, refused.status, refused.header, refused.body)
		case errors.As(err, &answer), errors.Is(err, context.Canceled):
			// it answered, not in System One's shape: nothing to rest for
		default:
			s.decideFailed(c, http.StatusBadGateway, nil, []byte(err.Error()))
		}
	}
	return verdict{}, true, err
}
