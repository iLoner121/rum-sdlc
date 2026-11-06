package store

import "syscall"

func processRunning(pid int) (bool, error) {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if err == syscall.Errno(87) {
			return false, nil
		}
		return false, err
	}
	defer syscall.CloseHandle(h)
	event, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		return false, err
	}
	return event == syscall.WAIT_TIMEOUT, nil
}
