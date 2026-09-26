//go:build !windows

package replication

import (
	"os"
	"syscall"
)

// chownLike gives path the owner and group of ref (the configuration file), so the service can read it.
func chownLike(path, ref string) {
	if fi, err := os.Stat(ref); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			_ = os.Chown(path, int(st.Uid), int(st.Gid))
		}
	}
}
