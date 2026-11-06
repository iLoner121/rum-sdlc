//go:build windows

package installation

import (
	"os/exec"
	"syscall"
)

func detachRecovery(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008}
}
