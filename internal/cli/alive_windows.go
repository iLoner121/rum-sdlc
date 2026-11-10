package cli

import "syscall"

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, e := syscall.OpenProcess(0x00100000, false, uint32(pid))
	if e != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	r, e := syscall.WaitForSingleObject(h, 0)
	return e == nil && r == 0x102
}
