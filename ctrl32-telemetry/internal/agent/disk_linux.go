//go:build linux

package agent

import "syscall"

func diskInfo(path string) (DiskInfo, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return DiskInfo{}, false
	}
	var fs syscall.Stat_t
	_ = syscall.Stat(path, &fs) // the device tells two paths on the same file system apart
	bs := float64(st.Bsize)
	return DiskInfo{Path: path, TotalGB: float64(st.Blocks) * bs / 1e9, FreeGB: float64(st.Bavail) * bs / 1e9, dev: uint64(fs.Dev)}, true
}
