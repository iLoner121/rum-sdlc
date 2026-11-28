package core

import "testing"

func TestTopicPriorityReplaysAsExtensionAndPreservesQueue(t *testing.T) {
	s, _ := numberingStep(t, nil, "project.init", Object{"id": "P-test", "name": "Test"})
	s, first := numberingTopic(t, s, "First")
	s, second := numberingTopic(t, s, "Second")
	query := func(options Object) Object {
		t.Helper()
		result, err := Query(s, options)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	pool := Object{"view": "members", "kind": "workset", "identifier": "POOL"}
	original := query(pool)
	s, _ = numberingStep(t, s, "topic.priority", Object{"id": "T2", "priority": "P0"})
	events := arr(s["history"], "history")
	request := obj(obj(events[len(events)-1], "event")["request"], "request")
	params := obj(request["params"], "params")
	if request["action"] != "extension.set" || params["id"] != second {
		t.Fatal("Priority writes must use the existing event format and long ID", request)
	}
	got := arr(query(pool)["rows"], "rows")
	old := arr(original["rows"], "rows")
	for i, id := range []string{first, second} {
		row := obj(got[i], "row")
		if row["id"] != id || !equal(row["position"], obj(old[i], "row")["position"]) {
			t.Fatal("Priority changed the pool order", got)
		}
	}
	pool["priority"] = "P0"
	pool["limit"] = 1
	if result := query(pool); !equal(result["total"], 1) || obj(arr(result["rows"], "rows")[0], "row")["id"] != second {
		t.Fatal("Priority must filter before paging", result)
	}
	pool["priority"] = "unset"
	if result := query(pool); !equal(result["total"], 1) || obj(arr(result["rows"], "rows")[0], "row")["id"] != first {
		t.Fatal("Legacy Topic should match unset", result)
	}
	s, _ = numberingStep(t, s, "topic.priority", Object{"id": second, "priority": "P3"})
	show := Object{"view": "show", "table": "topics", "identifier": "T2"}
	if got := obj(query(show)["entity"], "entity")["priority"]; got != "P3" {
		t.Fatal("Expected updated priority, got", got)
	}
	s, _ = numberingStep(t, s, "topic.priority", Object{"id": "T2", "priority": nil})
	if got := obj(query(show)["entity"], "entity"); has(got, "priority") {
		t.Fatal("Cleared priority must be omitted", got)
	}
	for _, parameters := range []Object{{"id": "T2"}, {"id": "T2", "priority": "P4"}, {"id": "T2", "priority": 2}} {
		if _, _, err := Dispatch(s, Request("topic.priority", parameters, "2026-11-06", "test", "evidence", "decision")); err == nil {
			t.Fatal("Unexpected success for", parameters)
		}
	}
}
