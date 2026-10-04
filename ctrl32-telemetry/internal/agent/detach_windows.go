//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

// spawnDetached starts a process that outlives the agent service (the update guard).
func spawnDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
