//go:build !darwin

package local

// appleGPUCores: only a Mac has Apple silicon's GPU.
func appleGPUCores() int { return 0 }
