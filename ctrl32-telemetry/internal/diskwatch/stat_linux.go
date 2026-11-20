//go:build linux

package diskwatch

import (
	"strconv"
	"syscall"
)

// diskStat returns the free and total bytes of the file system holding path and its device.
func diskStat(path string) (free, total uint64, id string, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, "", false
	}
	var fs syscall.Stat_t
	if err := syscall.Stat(path, &fs); err != nil {
		return 0, 0, "", false
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize), strconv.FormatUint(uint64(fs.Dev), 10), true
}
