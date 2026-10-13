package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCallsConcurrentWithSessionReaders(t *testing.T) {
	d := setupCalls(t)
	path := filepath.Join(d.claude, "projects", "-p", "sess1.jsonl")
	writeLines(t, path, claudeMsg("m1", "claude-sonnet-5", 1, 2, 3, 4, 1))
	// OpenCode's DB connection must survive a concurrent call-file refresh.
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db := ocMakeEmpty(t, dbPath, "")
	t.Setenv("OPENCODE_DB", dbPath)
	if _, err := db.Exec(`INSERT INTO session(id,project_id,slug,directory,title,version,time_created,time_updated) VALUES('s','p','s','/work','db session','1',1,1)`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				if len(Calls(time.Time{})) != 1 {
					t.Error("concurrent Calls lost a call")
				}
			case 1:
				if len(List(0)) != 2 {
					t.Error("concurrent List lost a session")
				}
			case 2:
				StatsFor(30)
			}
		}(i)
	}
	wg.Wait()
}

func TestRequestShardsAreBoundedAndIndependent(t *testing.T) {
	d := setupCalls(t)
	for i := 0; i < 40; i++ {
		var lines []string
		for j := 0; j < 150; j++ {
			lines = append(lines, strings.ReplaceAll(claudeMsg(fmt.Sprintf("%d-%d", i, j), "m", 1, 1, 0, 0, j), `"text":"hi"`, `"text":"PRIVATE REPLY MUST NOT BE CACHED"`))
		}
		writeLines(t, filepath.Join(d.claude, "projects", "-p", fmt.Sprintf("s-%02d.jsonl", i)), lines...)
	}
	if n := len(Calls(time.Time{})); n != 6000 {
		t.Fatal(n)
	}
	kept := 0
	for _, st := range callCache {
		kept += len(st.Calls)
	}
	if len(callCache) > maxKeptFiles || kept > maxKeptCalls {
		t.Fatalf("unbounded memory cache: %d files, %d calls", len(callCache), kept)
	}
	a := filepath.Join(d.claude, "projects", "-p", "s-00.jsonl")
	b := filepath.Join(d.claude, "projects", "-p", "s-01.jsonl")
	before, err := os.Stat(callCachePath(b))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(callCachePath(b))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE REPLY") {
		t.Fatal("conversation content persisted in request shard")
	}
	appendText(t, a, claudeMsg("new", "m", 2, 1, 0, 0, 200)+"\n")
	if n := len(Calls(time.Time{})); n != 6001 {
		t.Fatal(n)
	}
	after, err := os.Stat(callCachePath(b))
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("one changed session rewrote an unrelated shard")
	}
	resetCalls()
	if err := os.WriteFile(callCachePath(b), []byte("broken cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := len(Calls(time.Time{})); n != 6001 {
		t.Fatalf("corrupt shard was not rebuilt: %d", n)
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if n := len(Calls(time.Time{})); n != 5851 {
		t.Fatal(n)
	}
	if _, err := os.Stat(callCachePath(b)); !os.IsNotExist(err) {
		t.Fatal("removed session's shard remains")
	}
}
