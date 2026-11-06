package core

import (
	"rumsdlc/internal/jsonvalue"
	"testing"
)

func fixture(t *testing.T) Object {
	t.Helper()
	state, _, e := Dispatch(nil, Request("project.init", Object{"id": "P-test", "name": "Test"}, "2026-11-06", "test", nil, nil))
	if e != nil {
		t.Fatal(e)
	}
	for _, step := range []struct {
		action string
		params Object
	}{
		{"topic.create", Object{"id": "T-20261106-00000001", "id_reason": "Fixture", "name": "Topic", "type": "feat", "slug": "topic", "unassigned_reason": "Planned"}},
		{"line.create", Object{"id": "next", "name": "Next", "type": "development"}},
		{"relation.move", Object{"kind": "line", "topic_id": "T-20261106-00000001", "to": "next", "integration_context": Object{"topic_id": "T-20261106-00000001", "from_line_id": "mainline", "line_id": "next", "dependencies": "Reviewed", "contracts": "Reviewed", "extensions": Object{}}}},
	} {
		state, _, e = Dispatch(state, Request(step.action, step.params, "2026-11-06", "test", "evidence", "decision"))
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = Validate(state); e != nil {
		t.Fatal(e)
	}
	return state
}
func TestFailedCommandDoesNotMutateInput(t *testing.T) {
	s := fixture(t)
	before := jsonvalue.Clone(s)
	_, _, e := Dispatch(s, Request("topic.state", Object{"id": "T-20261106-00000001", "status": "ready"}, "2026-11-06", "test", nil, nil))
	if e == nil {
		t.Fatal("illegal transition accepted")
	}
	if !jsonvalue.Equal(s, before) {
		t.Fatal("caller state mutated")
	}
}
func TestMalformedSnapshotReturnsErrorNotPanic(t *testing.T) {
	state := fixture(t)
	replacements := []any{nil, true, false, 123, "wrong", []any{}, Object{}}
	// Every object field, including context and historical snapshots, is an input
	// boundary. Mutation checks ensure malformed JSON cannot crash the service.
	var visit func(any, func(any) Object)
	visit = func(value any, replace func(any) Object) {
		switch x := value.(type) {
		case Object:
			for k, v := range x {
				key := k
				child := func(n any) Object { copy := jsonvalue.Clone(x).(Object); copy[key] = n; return replace(copy) }
				for _, bad := range replacements {
					if jsonvalue.Equal(v, bad) {
						continue
					}
					candidate := child(bad)
					func() {
						defer func() {
							if p := recover(); p != nil {
								t.Fatalf("panic replacing %s: %v", key, p)
							}
						}()
						_ = Validate(candidate)
					}()
				}
				visit(v, child)
			}
		case []any:
			for i, v := range x {
				index := i
				visit(v, func(n any) Object { copy := jsonvalue.Clone(x).([]any); copy[index] = n; return replace(copy) })
			}
		}
	}
	visit(state, func(v any) Object { return v.(Object) })
}
func TestOpaqueNumericEqualityPreservesNoopRevision(t *testing.T) {
	s := fixture(t)
	r := Request("extension.set", Object{"table": "project", "id": "P-test", "namespace": "example", "value": Object{"number": 1}}, "2026-11-06", "test", "evidence", "decision")
	first, _, e := Dispatch(s, r)
	if e != nil {
		t.Fatal(e)
	}
	p := r["params"].(Object)
	value, _ := jsonvalue.Decode([]byte(`{"number":1.0}`))
	p["value"] = value
	second, _, e := Dispatch(first, r)
	if e != nil {
		t.Fatal(e)
	}
	if !jsonvalue.Equal(first, second) {
		t.Fatal("numeric-equivalent extension created history")
	}
}
func TestRejectYearZero(t *testing.T) {
	_, _, e := Dispatch(nil, Request("project.init", Object{"id": "P-test", "name": "Test"}, "0000-01-01", "test", nil, nil))
	if e == nil {
		t.Fatal("accepted year zero")
	}
}
