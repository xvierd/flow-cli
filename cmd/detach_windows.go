//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// Process creation flags (winbase.h).
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// detach starts c without a console, in its own process group, so it
// survives the terminal closing.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
