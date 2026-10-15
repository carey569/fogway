package usage

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/carey569/fogway/internal/filememo"
	"github.com/carey569/fogway/internal/sessions"
)

type desktopSessionInfo struct {
	Session string `json:"cliSessionId"`
	Email   string `json:"emailAddress"`
}

type desktopSessionIdentity struct {
	account, org, email string
	linkable            bool
}

// sessionResolver only identifies accounts recorded by local sessions. It does
// not infer a service provider, endpoint, or billing account.
type sessionResolver struct {
	desktop   map[string]desktopSessionIdentity // absolute metadata file
	bySession map[string][]desktopSessionIdentity
	emails    map[string]string // exact account/organization identity
	roots     []string
}

func newSessionResolver(logs []sessions.Call) *sessionResolver {
	r := &sessionResolver{}
	if len(logs) == 0 {
		return r
	}
	r.desktop = map[string]desktopSessionIdentity{}
	r.bySession = map[string][]desktopSessionIdentity{}
	r.emails = map[string]string{}
	r.roots = sessions.DesktopDataDirs()
	for _, root := range r.roots {
		for _, kind := range []string{"local-agent-mode-sessions", "claude-code-sessions"} {
			files, _ := sessions.SessionGlob(filepath.Join(root, kind, "*", "*", "local_*.json"))
			for _, path := range files {
				meta, err := filememo.Read("session desktop identity", path, func(b []byte) (desktopSessionInfo, error) {
					var info desktopSessionInfo
					err := json.Unmarshal(b, &info)
					return info, err
				})
				if err != nil {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				parts := strings.Split(rel, string(filepath.Separator))
				id := desktopSessionIdentity{account: parts[1], org: parts[2], email: meta.Email,
					linkable: !strings.Contains(filepath.Base(root), "-3p") && uuidIdentity(parts[1]) && uuidIdentity(parts[2])}
				r.desktop[path] = id
				if meta.Session != "" {
					r.bySession[meta.Session] = append(r.bySession[meta.Session], id)
				}
				if id.linkable {
					r.noteEmail(id.account, id.org, id.email)
				}
			}
		}
	}
	return r
}

func uuidIdentity(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func (r *sessionResolver) noteEmail(account, org, email string) {
	if account == "" || email == "" {
		return
	}
	key := account + "/" + org
	if old, ok := r.emails[key]; ok && old != email {
		r.emails[key] = "" // conflicting identities never pick an arbitrary email
	} else {
		r.emails[key] = email
	}
}

func (r *sessionResolver) resolve(c sessions.Call) string {
	if c.Agent == "codex" {
		return ""
	}
	id, found := r.desktopIdentity(c)
	if !found {
		return ""
	}
	if id.email == "" {
		id.email = r.emails[id.account+"/"+id.org]
	}
	return id.email
}

func (r *sessionResolver) desktopIdentity(c sessions.Call) (desktopSessionIdentity, bool) {
	if c.Agent != "claude" && c.Agent != "claude-desktop" {
		return desktopSessionIdentity{}, false
	}
	var id desktopSessionIdentity
	found := false
	for _, root := range r.roots {
		rel, err := filepath.Rel(root, c.File)
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 4 && parts[0] == "local-agent-mode-sessions" && strings.HasPrefix(parts[3], "local_") && parts[4] == ".claude" {
			path := filepath.Join(root, filepath.Join(parts[:4]...)) + ".json"
			id, found = r.desktop[path]
			break
		}
	}
	if !found {
		if ids := r.bySession[c.Session]; len(ids) == 1 {
			id, found = ids[0], true
		}
	}
	return id, found
}
