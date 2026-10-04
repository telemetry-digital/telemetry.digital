//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

// spawnDetached starts a process in its own session that outlives the agent (the update guard, when systemd-run is
// not available).
func spawnDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
