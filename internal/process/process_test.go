package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("RUM_TEST_PROCESS_HELPER")
	if mode == "" {
		return
	}
	folder := os.Getenv("RUM_TEST_PROCESS_FOLDER")
	if mode == "child" {
		if e := os.WriteFile(filepath.Join(folder, "ready"), nil, 0600); e != nil {
			os.Exit(3)
		}
		time.Sleep(time.Second)
		_ = os.WriteFile(filepath.Join(folder, "escaped"), nil, 0600)
		os.Exit(0)
	}
	exe, e := os.Executable()
	if e != nil {
		os.Exit(3)
	}
	child := exec.Command(exe, "-test.run=^TestProcessHelper$")
	child.Env = append(os.Environ(), "RUM_TEST_PROCESS_HELPER=child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if e = child.Start(); e != nil {
		os.Exit(3)
	}
	_ = child.Wait()
	os.Exit(0)
}

func TestCancellationTerminatesDescendants(t *testing.T) {
	folder := t.TempDir()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	env := Environment(true)
	env["RUM_TEST_PROCESS_HELPER"] = "parent"
	env["RUM_TEST_PROCESS_FOLDER"] = folder
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := (Runner{}).Run(ctx, []string{exe, "-test.run=^TestProcessHelper$"}, "", "", env, 10*time.Second, true)
		done <- e
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e := os.Stat(filepath.Join(folder, "ready")); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("unexpected result: %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation left command running")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, e := os.Stat(filepath.Join(folder, "escaped")); !os.IsNotExist(e) {
		t.Fatal("descendant survived process-group cancellation")
	}
}
