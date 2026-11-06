//go:build !windows

package cli

import "syscall"

const connectionRefused = syscall.ECONNREFUSED
