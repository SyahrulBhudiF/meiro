//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps ffmpeg from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
