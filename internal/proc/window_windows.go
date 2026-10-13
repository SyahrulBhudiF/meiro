//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// HideWindow prevents a child process from flashing a console window.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
