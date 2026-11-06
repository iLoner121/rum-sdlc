package migration

import (
	"errors"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
	"testing"
)

func fixture(t *testing.T) Options {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(root, "source")
	for _, collection := range []string{"topics", "iterations", "milestones"} {
		if e = os.MkdirAll(filepath.Join(source, "workflow", collection), 0700); e != nil {
			t.Fatal(e)
		}
	}
	cfg := jsonvalue.Object{"protocol_version": "3.2.2", "main_branch": "main"}
	if e = fsutil.WriteJSON(filepath.Join(source, "workflow", "config.json"), cfg); e != nil {
		t.Fatal(e)
	}
	id := "T-20261026-1234abcd"
	dir := filepath.Join(source, "workflow", "topics", id)
	os.MkdirAll(filepath.Join(dir, "tasks"), 0700)
	row := jsonvalue.Object{"protocol_version": "3.2.2", "id": id, "title": "Imported", "slug": "imported", "type": "feat", "status": "active", "created_on": "2026-10-26", "milestone": nil, "branch": "feat/" + id + "-imported", "dependencies": []any{}, "line": nil, "target_branch": "main", "closure": nil}
	if e = fsutil.WriteJSON(filepath.Join(dir, "topic.json"), row); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "tasks", "t001.md"), []byte("# t001 Actual investigation\n- Status: done\n- Kind: investigate\n- Topic: "+id+"\n\nActual result\n"), 0644); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "script.bin"), []byte{0, 1, 254, 255}, 0755); e != nil {
		t.Fatal(e)
	}
	return Options{Source: source, Destination: filepath.Join(root, "imported"), ProjectID: "P-import", Name: "Imported", Date: "2026-10-26", DryRun: true}
}
func TestNativeImportPreviewApplyAndStaleSource(t *testing.T) {
	o := fixture(t)
	preview, e := Run(o)
	if e != nil {
		t.Fatal(e)
	}
	if fsutil.Exists(o.Destination) {
		t.Fatal("preview mutated destination")
	}
	o.DryRun = false
	o.Expected = preview["revision"].(string)
	result, e := Run(o)
	if e != nil || result["changed"] != true {
		t.Fatal(result, e)
	}
	s, _ := store.New(o.Destination)
	state, e := s.Read(false)
	if e != nil || state["project"].(map[string]any)["id"] != "P-import" {
		t.Fatal(e)
	}
	original := filepath.Join(o.Source, "workflow/topics/T-20261026-1234abcd/tasks/t001.md")
	raw, _ := os.ReadFile(original)
	if !strings.Contains(string(raw), "- Status: done") {
		t.Fatal("source changed")
	}
	transformed, _ := os.ReadFile(filepath.Join(o.Destination, "documents/topics/T-20261026-1234abcd/tasks/t001.md"))
	if strings.Contains(string(transformed), "- Status:") {
		t.Fatal("live labels survived")
	}
	if _, e = Run(o); e == nil {
		t.Fatal("nonempty destination accepted")
	}
	o = fixture(t)
	preview, e = Run(o)
	if e != nil {
		t.Fatal(e)
	}
	o.DryRun = false
	o.Expected = preview["revision"].(string)
	os.WriteFile(filepath.Join(o.Source, "PROJECT_CONTEXT.md"), []byte("changed"), 0600)
	if _, e = Run(o); e == nil {
		t.Fatal("stale source accepted")
	}
	if fsutil.Exists(o.Destination) {
		t.Fatal("stale request created destination")
	}
}
func TestNativeImportFinalWriteFailureRollsBackOnlyOwnFiles(t *testing.T) {
	o := fixture(t)
	preview, e := Run(o)
	if e != nil {
		t.Fatal(e)
	}
	o.DryRun = false
	o.Expected = preview["revision"].(string)
	original := writeState
	defer func() { writeState = original }()
	writeState = func(path string, value any) error {
		if e := os.WriteFile(filepath.Join(filepath.Dir(path), "external.txt"), []byte("keep"), 0600); e != nil {
			return e
		}
		return errors.New("injected disk failure")
	}
	if _, e = Run(o); e == nil || !strings.Contains(e.Error(), "disk failure") {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(o.Destination)
	if e != nil || len(entries) != 1 || entries[0].Name() != "external.txt" {
		t.Fatal(entries, e)
	}
	raw, _ := os.ReadFile(filepath.Join(o.Destination, "external.txt"))
	if string(raw) != "keep" {
		t.Fatal("external file changed")
	}
}
func TestLegacyFenceAndSourceShapeRefusal(t *testing.T) {
	for _, content := range []string{"# t001 Task\n```\n- Status: done\n", "# t001 Task\n```example\n```text does not close\n- Status: done\n- Topic: T-20261026-1234abcd\n"} {
		o := fixture(t)
		path := filepath.Join(o.Source, "workflow/topics/T-20261026-1234abcd/tasks/t001.md")
		os.WriteFile(path, []byte(content), 0600)
		if _, e := Run(o); e == nil {
			t.Fatal("unclosed fenced metadata accepted")
		}
	}
	o := fixture(t)
	os.Mkdir(filepath.Join(o.Source, "workflow/topics/incomplete"), 0700)
	if _, e := Run(o); e == nil {
		t.Fatal("incomplete record skipped")
	}
}
