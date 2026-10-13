//go:build !windows

package proc

import "os/exec"

// HideWindow is a no-op on platforms without console windows.
func HideWindow(*exec.Cmd) {}
