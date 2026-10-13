//go:build !darwin && !linux && !windows

package local

func totalRAM() uint64 { return 0 }
