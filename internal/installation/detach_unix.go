//go:build !windows

package installation

import (
	"os/exec"
	"syscall"
)

func detachRecovery(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
