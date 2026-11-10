package store

import (
	"path/filepath"
	"rumsdlc/internal/core"
	"strings"
	"testing"
)

func TestTopicCreationOutputDocumentsAndStaleNumberPreview(t *testing.T) {
	s := fresh(t)
	req := core.Request("topic.create", Object{"name": "First", "type": "feat", "slug": "first", "unassigned_reason": "Planned"}, "2026-11-06", "test", nil, nil)
	pre, err := s.Execute(req, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if pre["short_id"] != "T1" {
		t.Fatal(pre)
	}
	result := apply(t, s, req)
	if result["short_id"] != "T1" {
		t.Fatal(result)
	}
	if _, err = s.Execute(req, false, pre["revision"].(string)); err == nil || !strings.Contains(err.Error(), "Stale") {
		t.Fatal("stale allocation accepted", err)
	}
	pre, err = s.Execute(req, true, "")
	if err != nil || pre["short_id"] != "T2" {
		t.Fatal(pre, err)
	}
	id := result["result"].(string)
	content := "# Existing context\n"
	initial, err := s.Document(DocumentOptions{Table: "topics", ID: id, Field: "context"})
	if err != nil {
		t.Fatal(err)
	}
	long, err := s.Document(DocumentOptions{Table: "topics", ID: id, Field: "context", Content: &content, ExpectedHash: initial["hash"].(string)})
	if err != nil {
		t.Fatal(err)
	}
	short, err := s.Document(DocumentOptions{Table: "topics", ID: "T1", Field: "context"})
	if err != nil {
		t.Fatal(err)
	}
	if short["path"] != long["path"] || short["hash"] != long["hash"] || !strings.Contains(short["path"].(string), filepath.Join("topics", id)) {
		t.Fatal(short, long)
	}
	task := apply(t, s, core.Request("task.create", Object{"topic_id": "T1", "name": "Design", "kind": "design"}, "2026-11-06", "test", nil, nil))
	if task["short_id"] != "T1/t001" {
		t.Fatal(task)
	}
	doc, err := s.Document(DocumentOptions{Table: "tasks", ID: "T1/t001", Ensure: true})
	if err != nil || !strings.Contains(doc["path"].(string), filepath.Join(id, "tasks", "t001.md")) {
		t.Fatal(doc, err)
	}
}
