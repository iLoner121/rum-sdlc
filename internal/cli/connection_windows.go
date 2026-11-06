package cli

import "syscall"

// Winsock returns WSAECONNREFUSED; syscall.ECONNREFUSED is a synthetic Go errno
// on Windows and does not match the error returned by net.Dial.
const connectionRefused = syscall.Errno(10061)
