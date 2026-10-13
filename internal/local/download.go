package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Progress is a download under way: Done of Total bytes (Total 0 while it
// isn't known).
type Progress struct {
	What  string `json:"what"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

// progressFn is told how a download goes, at most a few times a second.
type progressFn func(Progress)

// hfEndpoint is where models are pulled from: the setting, else
// HF_ENDPOINT, else Hugging Face itself.
func hfEndpoint(c Config) string {
	for _, e := range []string{c.HFEndpoint, os.Getenv("HF_ENDPOINT")} {
		if e = strings.TrimRight(strings.TrimSpace(e), "/"); e != "" {
			return e
		}
	}
	return "https://huggingface.co"
}

// hfFile is what Hugging Face says of a file of a repository: its size
// and, for a file kept in LFS (every GGUF), its SHA-256.
type hfFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  *struct {
		Oid  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"lfs"`
}

// hfInfo asks the repository's tree for the file.
func hfInfo(ctx context.Context, c Config, m Model) (hfFile, error) {
	dir := ""
	if i := strings.LastIndex(m.File, "/"); i >= 0 {
		dir = "/" + m.File[:i]
	}
	u := hfEndpoint(c) + "/api/models/" + m.Repo + "/tree/main" + dir
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return hfFile{}, err
	}
	hfAuth(req)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return hfFile{}, fmt.Errorf("asking %s: %w", hostOf(u), err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return hfFile{}, fmt.Errorf("%s answered %s for %s", hostOf(u), res.Status, m.Repo)
	}
	var files []hfFile
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&files); err != nil {
		return hfFile{}, fmt.Errorf("reading %s's list of %s: %w", hostOf(u), m.Repo, err)
	}
	for _, f := range files {
		if f.Path == m.File {
			return f, nil
		}
	}
	return hfFile{}, fmt.Errorf("%s has no file %s", m.Repo, m.File)
}

// hfAuth gives a request the user's Hugging Face token, for a gated
// repository, when HF_TOKEN has one.
func hfAuth(req *http.Request) {
	if t := strings.TrimSpace(os.Getenv("HF_TOKEN")); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
}

// ModelPath is where a model is kept once downloaded.
func ModelPath(m Model) string {
	return filepath.Join(Dir(), "models", strings.ReplaceAll(m.Repo, "/", "--"), filepath.Base(m.File))
}

// pullModel downloads m unless it is there already, whole and as Hugging
// Face says it is, and returns its path.
func pullModel(ctx context.Context, c Config, m Model, tell progressFn) (string, error) {
	path := ModelPath(m)
	info, err := hfInfo(ctx, c, m)
	if err != nil {
		// offline, with the model there already: run it as it is
		if st, serr := os.Stat(path); serr == nil && st.Size() > 0 {
			return path, nil
		}
		return "", err
	}
	want, sum := info.Size, ""
	if info.LFS != nil {
		want, sum = info.LFS.Size, info.LFS.Oid
	}
	if st, err := os.Stat(path); err == nil && st.Size() == want {
		if sum == "" || verified(path, sum) {
			return path, nil
		}
	}
	u := hfEndpoint(c) + "/" + m.Repo + "/resolve/main/" + escapePath(m.File)
	if err := fetch(ctx, u, path, want, sum, "model "+m.ID(), tell, hfAuth); err != nil {
		return "", err
	}
	return path, nil
}

// verified says the file's SHA-256 is sum, and remembers that beside it
// so a model of several gigabytes isn't read whole at every start.
func verified(path, sum string) bool {
	mark := path + ".sha256"
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	stamp := sum + " " + strconv.FormatInt(st.Size(), 10) + " " + strconv.FormatInt(st.ModTime().UnixNano(), 10)
	if b, err := os.ReadFile(mark); err == nil && strings.TrimSpace(string(b)) == stamp {
		return true
	}
	got, err := fileSum(path)
	if err != nil || !strings.EqualFold(got, sum) {
		return false
	}
	_ = os.WriteFile(mark, []byte(stamp+"\n"), 0o600)
	return true
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetch downloads u to path: into path.part first, going on from where an
// earlier try stopped when the server takes a range, checked against size
// and sum (either may be unknown) before it is renamed into place.
func fetch(ctx context.Context, u, path string, size int64, sum, what string, tell progressFn, auth func(*http.Request)) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	part := path + ".part"
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
		if size > 0 && have > size {
			os.Remove(part)
			have = 0
		}
	}
	for attempt := 0; ; attempt++ {
		err := fetchOnce(ctx, u, part, &have, size, what, tell, auth)
		if err == nil {
			break
		}
		if ctx.Err() != nil || attempt >= 4 || errors.Is(err, errRefused) {
			return err
		}
		select { // a dropped connection: on from where it stopped
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if size > 0 && have != size {
		return fmt.Errorf("%s: got %d bytes of %d", what, have, size)
	}
	if sum != "" {
		got, err := fileSum(part)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, sum) {
			os.Remove(part)
			return fmt.Errorf("%s: its SHA-256 is %s, not %s as published; removed, download it again", what, got, sum)
		}
	}
	if err := os.Rename(part, path); err != nil {
		return err
	}
	if sum != "" {
		if st, err := os.Stat(path); err == nil {
			stamp := sum + " " + strconv.FormatInt(st.Size(), 10) + " " + strconv.FormatInt(st.ModTime().UnixNano(), 10)
			_ = os.WriteFile(path+".sha256", []byte(stamp+"\n"), 0o600)
		}
	}
	return nil
}

// errRefused is a server's answer no retry mends (404, 401, 403).
var errRefused = errors.New("refused")

// stallAfter is how long a download may go without a byte before its
// connection is given up and opened again.
const stallAfter = 60 * time.Second

func fetchOnce(ctx context.Context, u, part string, have *int64, size int64, what string, tell progressFn, auth func(*http.Request)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stall := time.AfterFunc(stallAfter, cancel)
	defer stall.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if auth != nil {
		auth(req)
	}
	if *have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(*have, 10)+"-")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	defer res.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch res.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		flags |= os.O_TRUNC // the server sent it whole
		*have = 0
	case http.StatusRequestedRangeNotSatisfiable:
		if size > 0 && *have == size {
			return nil
		}
		err := fmt.Errorf("%s: the server refused to go on from byte %d", what, *have)
		os.Remove(part)
		*have = 0
		return err
	default:
		err := fmt.Errorf("%s: %s answered %s", what, hostOf(u), res.Status)
		if res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 404 {
			return fmt.Errorf("%w: %w", errRefused, err)
		}
		return err
	}
	total := size
	if total <= 0 && res.ContentLength > 0 {
		total = *have + res.ContentLength
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	var done atomic.Int64
	done.Store(*have)
	stop := make(chan struct{})
	defer close(stop)
	if tell != nil {
		go func() {
			t := time.NewTicker(300 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					tell(Progress{What: what, Done: done.Load(), Total: total})
				case <-stop:
					return
				}
			}
		}()
	}
	buf := make([]byte, 1<<20)
	for {
		n, rerr := res.Body.Read(buf)
		stall.Reset(stallAfter)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			*have += int64(n)
			done.Store(*have)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("%s: %w", what, rerr)
		}
	}
	if tell != nil {
		tell(Progress{What: what, Done: *have, Total: total})
	}
	return nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}
