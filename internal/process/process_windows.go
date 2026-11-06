package process

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var createJob = kernel32.NewProc("CreateJobObjectW")
var setJob = kernel32.NewProc("SetInformationJobObject")
var assignJob = kernel32.NewProc("AssignProcessToJobObject")
var terminateJob = kernel32.NewProc("TerminateJobObject")
var threadFirst = kernel32.NewProc("Thread32First")
var threadNext = kernel32.NewProc("Thread32Next")
var openThread = kernel32.NewProc("OpenThread")
var resumeThread = kernel32.NewProc("ResumeThread")

type jobBasicLimits struct {
	ProcessTime, JobTime         int64
	Flags                        uint32
	MinWorkingSet, MaxWorkingSet uintptr
	ActiveProcesses              uint32
	Affinity                     uintptr
	Priority, Scheduling         uint32
}
type jobLimits struct {
	Basic                                                      jobBasicLimits
	IO                                                         [6]uint64
	ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory uintptr
}
type threadEntry struct {
	Size, Usage, ID, ProcessID  uint32
	BasePriority, DeltaPriority int32
	Flags                       uint32
}

func resumeProcess(pid int) error {
	snapshot, e := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return e
	}
	defer syscall.CloseHandle(snapshot)
	var entry threadEntry
	entry.Size = uint32(unsafe.Sizeof(entry))
	result, _, e := threadFirst.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	for result != 0 {
		if entry.ProcessID == uint32(pid) {
			thread, _, e := openThread.Call(0x0002, 0, uintptr(entry.ID))
			if thread == 0 {
				return e
			}
			defer syscall.CloseHandle(syscall.Handle(thread))
			result, _, e = resumeThread.Call(thread)
			if result == 0xffffffff {
				return e
			}
			return nil
		}
		result, _, e = threadNext.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	}
	return fmt.Errorf("Cannot find suspended subprocess thread: %w", e)
}

// Suspend before assigning the Job Object: a Git/credential-helper child must
// not be able to escape cancellation by starting before the job is attached.
func runProcess(ctx context.Context, cmd *exec.Cmd) error {
	job, _, e := createJob.Call(0, 0)
	if job == 0 {
		return e
	}
	defer syscall.CloseHandle(syscall.Handle(job))
	limits := jobLimits{}
	limits.Basic.Flags = 0x00002000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	result, _, e := setJob.Call(job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if result == 0 {
		return e
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000004 | 0x00000200} // suspended, new process group
	cmd.Cancel = func() error {
		result, _, e := terminateJob.Call(job, 1)
		if result == 0 {
			return e
		}
		return nil
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	fail := func(e error) error { _ = cmd.Process.Kill(); _ = cmd.Wait(); return e }
	process, e := syscall.OpenProcess(0x0100|0x0001, false, uint32(cmd.Process.Pid))
	if e != nil {
		return fail(e)
	}
	result, _, e = assignJob.Call(job, uintptr(process))
	syscall.CloseHandle(process)
	if result == 0 {
		return fail(e)
	}
	if e = ctx.Err(); e != nil {
		_ = cmd.Cancel()
		return fail(e)
	}
	if e = resumeProcess(cmd.Process.Pid); e != nil {
		return fail(e)
	}
	return cmd.Wait()
}
