//go:build !windows

package player

import "os/exec"

// hideWindow is a no-op on platforms without console windows.
func hideWindow(*exec.Cmd) {}
