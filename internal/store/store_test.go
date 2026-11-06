package store

import (
	"os"
	"path/filepath"
	"rumsdlc/internal/core"
	"rumsdlc/internal/jsonvalue"
	"strings"
	"sync"
	"testing"
)

func fresh(t *testing.T) *Store {
	t.Helper()
	s, e := New(filepath.Join(t.TempDir(), "project"))
	if e != nil {
		t.Fatal(e)
	}
	r := core.Request("project.init", Object{"id": "P-test", "name": "测试"}, "2026-11-06", "test", nil, nil)
	apply(t, s, r)
	return s
}
func apply(t *testing.T, s *Store, r Object) Object {
	t.Helper()
	pre, e := s.Execute(r, true, "")
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.Execute(r, false, pre["revision"].(string))
	if e != nil {
		t.Fatal(e)
	}
	return result
}
func TestPreviewDoesNotCreateAndStaleApplyDoesNotWrite(t *testing.T) {
	s, e := New(filepath.Join(t.TempDir(), "new"))
	if e != nil {
		t.Fatal(e)
	}
	r := core.Request("project.init", Object{"id": "P-test", "name": "Test"}, "2026-11-06", "test", nil, nil)
	pre, e := s.Execute(r, true, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(s.Root); !os.IsNotExist(e) {
		t.Fatal("preview created root")
	}
	if _, e = s.Execute(r, false, pre["revision"].(string)); e != nil {
		t.Fatal(e)
	}
	rename := core.Request("entity.rename", Object{"table": "project", "id": "P-test", "name": "First"}, "2026-11-06", "rename", nil, nil)
	pre, e = s.Execute(rename, true, "")
	if e != nil {
		t.Fatal(e)
	}
	apply(t, s, core.Request("milestone.create", Object{"name": "Concurrent"}, "2026-11-06", "new", nil, nil))
	before, e := os.ReadFile(filepath.Join(s.Root, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Execute(rename, false, pre["revision"].(string)); e == nil || !strings.Contains(e.Error(), "Stale") {
		t.Fatalf("%v", e)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "state.json"))
	if string(before) != string(after) {
		t.Fatal("stale apply changed snapshot")
	}
}
func TestConcurrentCASOnlyOneWriterCommits(t *testing.T) {
	s := fresh(t)
	r := core.Request("milestone.create", Object{"name": "One"}, "2026-11-06", "test", nil, nil)
	pre, e := s.Execute(r, true, "")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, e := s.Execute(r, false, pre["revision"].(string)); results <- e }()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("%d writers committed", success)
	}
	state, e := s.Read(false)
	if e != nil {
		t.Fatal(e)
	}
	if len(state["milestones"].(Object)) != 1 {
		t.Fatal("wrong milestone count")
	}
}
func TestSymlinkAndTamperedHistoryRejected(t *testing.T) {
	s := fresh(t)
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(s.Root, "documents")); e != nil {
		t.Skipf("symlink unavailable: %v", e)
	}
	if _, e := s.Document(DocumentOptions{Table: "project", ID: "P-test", Ensure: true}); e == nil {
		t.Fatal("accepted symlink")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("wrote outside root")
	}
	state, e := s.Read(false)
	if e != nil {
		t.Fatal(e)
	}
	state["project"].(Object)["name"] = "forged"
	if e = Atomic(filepath.Join(s.Root, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(false); e == nil {
		t.Fatal("accepted forged state")
	}
}
func TestDocumentsRequireCASAndDoNotChangeMetadata(t *testing.T) {
	s := fresh(t)
	before, _ := s.Read(false)
	o := DocumentOptions{Table: "project", ID: "P-test"}
	pre, e := s.Document(o)
	if e != nil {
		t.Fatal(e)
	}
	body := "# 用户内容\n"
	o.Content = &body
	o.ExpectedHash = pre["hash"].(string)
	result, e := s.Document(o)
	if e != nil {
		t.Fatal(e)
	}
	if result["changed"] != true {
		t.Fatal(result)
	}
	if _, e = s.Document(o); e == nil {
		t.Fatal("stale document accepted")
	}
	after, e := s.Read(false)
	if e != nil {
		t.Fatal(e)
	}
	if !jsonvalue.Equal(before, after) {
		t.Fatal("document changed state")
	}
}
func TestLiveWriterRecoveryAndLockRelease(t *testing.T) {
	s := fresh(t)
	err := s.locked(func() error {
		status, e := s.LockStatus()
		if e != nil {
			t.Fatal(e)
		}
		owner := status["owner"].(Object)
		if _, e = s.RecoverLock(owner["token"].(string), true); e == nil {
			t.Fatal("recovered live writer")
		}
		return os.ErrPermission
	})
	if err != os.ErrPermission {
		t.Fatalf("%v", err)
	}
	status, e := s.LockStatus()
	if e != nil || status["locked"] != false {
		t.Fatalf("%v %v", status, e)
	}
}
