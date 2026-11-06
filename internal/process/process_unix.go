//go:build !windows

package process

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func runProcess(_ context.Context, cmd *exec.Cmd) error { configure(cmd); return cmd.Run() }
