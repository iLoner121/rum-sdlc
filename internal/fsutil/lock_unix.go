//go:build !windows

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

func lockFile(f *os.File, exclusive, blocking bool) error {
	flags := syscall.LOCK_SH
	if exclusive {
		flags = syscall.LOCK_EX
	}
	if !blocking {
		flags |= syscall.LOCK_NB
	}
	if e := syscall.Flock(int(f.Fd()), flags); e != nil {
		return fmt.Errorf("Another RumSDLC process holds this lock: %w", e)
	}
	return nil
}
func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
