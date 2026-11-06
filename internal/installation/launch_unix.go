//go:build !windows

package installation

import (
	"os"
	"os/exec"
	"time"
)

func configureManaged(command *exec.Cmd) {
	// Let the native runtime cancel Git and release its operation locks before
	// falling back to a forced stop. Terminal interrupts may reach both processes.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 5 * time.Second
}
