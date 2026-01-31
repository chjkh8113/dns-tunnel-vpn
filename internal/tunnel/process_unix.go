//go:build !windows

package tunnel

import (
	"os/exec"
	"syscall"
)

// setProcAttr sets Unix-specific process attributes
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// checkWindowsProcess is a no-op on Unix (never called)
func checkWindowsProcess(pid int) bool {
	return false
}
