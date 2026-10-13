//go:build windows

package proc

import (
	"os/exec"
	"strconv"
)

// hardenCommand makes cancellation kill the child's whole process tree.
func hardenCommand(cmd *exec.Cmd) {
	HideWindow(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		HideWindow(kill)
		return kill.Run()
	}
}
