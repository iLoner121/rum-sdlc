package core

import (
	"fmt"
	"rumsdlc/internal/jsonvalue"
	"testing"
)

func numberingStep(t *testing.T, s Object, action string, p Object) (Object, string) {
	t.Helper()
	after, id, err := Dispatch(s, Request(action, p, "2026-11-06", "numbering test", "evidence", "decision"))
	if err != nil {
		t.Fatal(action, err)
	}
	if err = Validate(after); err != nil {
		t.Fatal(action, err)
	}
	return after, str(id)
}
func numberingTopic(t *testing.T, s Object, name string) (Object, string) {
	return numberingStep(t, s, "topic.create", Object{"name": name, "type": "feat", "slug": "topic", "unassigned_reason": "Planned"})
}
func TestTopicNumbersRemainStableAcrossLifecycleAndRelations(t *testing.T) {
	s, _ := numberingStep(t, nil, "project.init", Object{"id": "P-test", "name": "Test"})
	s, first := numberingTopic(t, s, "First")
	s, second := numberingTopic(t, s, "Second")
	s, _ = numberingStep(t, s, "topic.close", Object{"id": "T1", "outcome": "cancelled", "summary": "Cancelled", "verification": "Inspected", "retention": "Retain documents"})
	s, _ = numberingStep(t, s, "milestone.create", Object{"name": "Next"})
	s, _ = numberingStep(t, s, "relation.move", Object{"kind": "milestone", "topic_id": "T2", "to": "M0"})
	s, third := numberingTopic(t, s, "Third")
	s, _ = numberingStep(t, s, "workset.create", Object{"name": "Iteration"})
	s, _ = numberingStep(t, s, "workset.propose", Object{"id": "ITER-001", "topics": []any{"T2"}})
	s, _ = numberingStep(t, s, "workset.activate", Object{"id": "ITER-001"})
	s, _ = numberingStep(t, s, "line.create", Object{"id": "next", "name": "Next", "type": "development"})
	s, _ = numberingStep(t, s, "relation.move", Object{"kind": "line", "topic_id": "T2", "to": "next", "integration_context": Object{"topic_id": "T2", "from_line_id": "mainline", "line_id": "next", "dependencies": "Reviewed", "contracts": "Reviewed", "extensions": Object{}}})
	if obj(obj(entity(s, "topics", second)["integration_recheck"], "pending")["context"], "context")["topic_id"] != second {
		t.Fatal("context identity was not canonicalized")
	}
	view, err := Query(s, Object{"view": "iteration-progress", "identifier": "ITER-001"})
	if err != nil || obj(arr(view["rows"], "rows")[0], "row")["short_id"] != "T2" {
		t.Fatal(view, err)
	}

	for id, want := range map[string]string{first: "T1", second: "T2", third: "T3"} {
		if got := ShortReference(s, id); got != want {
			t.Fatalf("%s = %s, want %s", id, got, want)
		}
		got, err := CanonicalReference(s, want)
		if err != nil || got != id {
			t.Fatalf("%s: %s, %v", want, got, err)
		}
	}
	// Presentation never mutates the authoritative snapshot.
	before := jsonvalue.Clone(s)
	for _, p := range []Object{{"view": "entities", "table": "topics", "include_closed": true}, {"view": "show", "table": "topics", "identifier": "T2"}, {"view": "resolve", "identifier": "T1"}, {"view": "members", "kind": "milestone", "identifier": "M0"}, {"view": "orphans"}} {
		if _, err := Query(s, p); err != nil {
			t.Fatal(err)
		}
	}
	if !jsonvalue.Equal(before, s) {
		t.Fatal("query changed managed state")
	}
	a, _ := Query(s, Object{"view": "show", "table": "topics", "identifier": "T2"})
	b, _ := Query(s, Object{"view": "show", "table": "topics", "identifier": second})
	if !equal(a, b) || obj(a["entity"], "entity")["short_id"] != "T2" {
		t.Fatal("short/long query mismatch")
	}
}
func TestTopicReferencesNormalizeOnlyStructuredFields(t *testing.T) {
	s, _ := numberingStep(t, nil, "project.init", Object{"id": "P-test", "name": "Test"})
	s, first := numberingTopic(t, s, "First")
	s, second := numberingTopic(t, s, "Second")
	s, _ = numberingStep(t, s, "topic.dependencies", Object{"id": "T2", "dependencies": []any{"T1"}})
	if arr(entity(s, "topics", second)["dependencies"], "dependencies")[0] != first {
		t.Fatal("dependency not canonical")
	}
	s, _ = numberingStep(t, s, "topic.state", Object{"id": "T2", "status": "active"})
	s, task := numberingStep(t, s, "task.create", Object{"topic_id": "T2", "name": "Discuss T1", "kind": "design"})
	s, _ = numberingStep(t, s, "task.state", Object{"id": "T2/t001", "status": "active"})
	if task != second+"/t001" {
		t.Fatal(task)
	}
	s, _ = numberingStep(t, s, "extension.set", Object{"table": "topics", "id": "T2", "namespace": "example", "value": Object{"id": "T1", "topic_id": "T1"}})
	if obj(obj(entity(s, "topics", second)["extensions"], "extensions")["example"], "example")["id"] != "T1" {
		t.Fatal("opaque extension changed")
	}
	s, _ = numberingStep(t, s, "entity.rename", Object{"table": "topics", "id": "T2", "name": "Discuss T1"})
	if entity(s, "topics", second)["name"] != "Discuss T1" {
		t.Fatal("name changed")
	}
	for _, id := range []string{"T0", "T01", "T999", "T-1", "t1", "T1/../../bad"} {
		if _, err := Query(s, Object{"view": "show", "table": "topics", "identifier": id}); err == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	// A mixed duplicate must fail after resolving aliases to canonical identities.
	if _, _, err := Dispatch(s, Request("topic.dependencies", Object{"id": "T2", "dependencies": []any{"T1", first}}, "2026-11-06", "test", nil, nil)); err == nil {
		t.Fatal("alias duplicate accepted")
	}
	history, err := Query(s, Object{"view": "history", "identifier": "T2"})
	if err != nil || len(arr(history["rows"], "rows")) < 3 {
		t.Fatal("short history query", err)
	}
	// Requests retain what the caller supplied; persisted relationships are canonical.
	last := obj(arr(s["history"], "history")[num(s["revision"])-1], "event")
	if obj(obj(last["request"], "request")["params"], "params")["id"] != "T2" {
		t.Fatal("caller request rewritten")
	}
}
func TestTopicNumbersGrowBeyond999AndIgnoreCurrentSorting(t *testing.T) {
	// Focus the scale check on the public history projection without building
	// thousands of redundant full-state audit snapshots in the test process.
	s := empty()
	s["history"] = []any{}
	for i := 1; i <= 1001; i++ {
		id := fmt.Sprintf("T-20261106-%08x", 1002-i)
		rows(s, "topics")[id] = Object{"id": id, "status": "closed"}
		s["history"] = append(s["history"].([]any), Object{"changes": []any{Object{"path": []any{"topics", id}, "before": nil, "after": Object{"id": id}}}})
	}
	if got := ShortReference(s, "T-20261106-00000001"); got != "T1001" {
		t.Fatal(got)
	}
	if got := ShortReference(s, "T-20261106-000003e9"); got != "T1" {
		t.Fatal(got)
	}
	if id, err := CanonicalReference(s, "T1000/t001"); err != nil || id != "T-20261106-00000002/t001" {
		t.Fatal(id, err)
	}
}
