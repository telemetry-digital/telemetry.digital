//go:build !linux && !windows

package agent

func diskInfo(path string) (DiskInfo, bool) { return DiskInfo{}, false }
