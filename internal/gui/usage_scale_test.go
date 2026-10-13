package gui

// Run with FOGWAY_USAGE_SCALE=1 go test -tags nogui ./internal/gui
// -run TestUsageScale -count=1 -v. The fixture is entirely synthetic.

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/fogway/fogway/internal/sessions"
)

func scaleShardBytes(t *testing.T) int64 {
	t.Helper()
	var n int64
	filepath.WalkDir(filepath.Dir(sessions.CachePath()), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if st, e := d.Info(); e == nil {
				n += st.Size()
			}
		}
		return nil
	})
	return n
}
func scaleMemory() func() uint64 {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	var peak uint64
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			peak = max(peak, m.HeapInuse)
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() uint64 { close(done); wg.Wait(); return peak }
}

func scaleShardSnapshot() map[string]os.FileInfo {
	out := map[string]os.FileInfo{}
	filepath.WalkDir(filepath.Dir(sessions.CachePath()), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if st, e := d.Info(); e == nil {
				out[path] = st
			}
		}
		return nil
	})
	return out
}
