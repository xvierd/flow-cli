//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// detach makes c a session leader so it survives the terminal closing.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
