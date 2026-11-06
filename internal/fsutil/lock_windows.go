package fsutil

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var lockFileEx = kernel.NewProc("LockFileEx")
var unlockFileEx = kernel.NewProc("UnlockFileEx")

func lockFile(f *os.File, exclusive, blocking bool) error {
	flags := uintptr(0)
	if exclusive {
		flags |= 2
	}
	if !blocking {
		flags |= 1
	}
	var overlapped syscall.Overlapped
	r, _, e := lockFileEx.Call(f.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		return fmt.Errorf("Another RumSDLC process holds this lock: %w", e)
	}
	return nil
}
func unlockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r, _, e := unlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		return e
	}
	return nil
}
