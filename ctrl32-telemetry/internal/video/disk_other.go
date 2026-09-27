//go:build !linux && !windows

package video

func diskFree(string) (uint64, uint64, bool) { return 0, 0, false }
