//go:build !darwin && !linux && !windows

package local

// diskFree is unknown here.
func diskFree(string) uint64 { return 0 }
