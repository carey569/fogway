//go:build darwin || linux

package local

import "golang.org/x/sys/unix"

// diskFree is the space free to this user on path's disk, 0 when unknown.
func diskFree(path string) uint64 {
	var st unix.Statfs_t
	if unix.Statfs(existing(path), &st) != nil {
		return 0
	}
	return uint64(st.Bavail) * uint64(st.Bsize)
}
