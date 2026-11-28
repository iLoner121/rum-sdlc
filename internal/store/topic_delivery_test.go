package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rumsdlc/internal/core"
)

func TestDeliveredTopicRequiresMilestoneInPoolAndIteration(t *testing.T) {
	for _, tc := range []struct {
		name, workset string
		activate      bool
	}{
		{name: "Pool with planned Milestone", workset: "POOL"},
		{name: "Iteration with active Milestone", workset: "ITER-001", activate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fresh(t)
			request := func(action string, p Object) Object {
				return core.Request(action, p, "2026-11-06", "test", "evidence", "decision")
			}
			command := func(action string, p Object) any {
				t.Helper()
				return apply(t, s, request(action, p))["result"]
			}
			id := command("topic.create", Object{"name": "Delivery", "type": "feat", "slug": "delivery", "unassigned_reason": "Pending planning"})
			milestone := command("milestone.create", Object{"name": "First"})
			if tc.activate {
				command("milestone.activate", Object{"id": milestone})
				iteration := command("workset.create", Object{"name": "Round"})
				command("workset.propose", Object{"id": iteration, "topics": []any{id}})
				command("workset.activate", Object{"id": iteration})
			}
			command("topic.state", Object{"id": id, "status": "active"})
			command("topic.state", Object{"id": id, "status": "review", "verification": "Tested"})
			command("topic.state", Object{"id": id, "status": "ready", "acceptance": "Accepted"})
			closeRequest := request("topic.close", Object{"id": id, "outcome": "delivered", "summary": "Delivered", "verification": "Tested", "delivery_record": "Report"})
			move := func(to any) {
				command("relation.move", Object{"kind": "milestone", "topic_id": id, "to": to})
			}
			snapshotBytes := func() string {
				t.Helper()
				raw, err := os.ReadFile(filepath.Join(s.Root, "state.json"))
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			}
			wantRefusal := func(dryRun bool, token string) {
				t.Helper()
				before := snapshotBytes()
				if _, err := s.Execute(closeRequest, dryRun, token); err == nil || !strings.Contains(err.Error(), "planned or active Milestone") {
					t.Fatalf("dry-run=%v: got %v", dryRun, err)
				}
				if snapshotBytes() != before {
					t.Fatal("rejected closure changed the snapshot")
				}
			}
			wantRefusal(true, "")
			move(milestone)
			preview, err := s.Execute(closeRequest, true, "")
			if err != nil {
				t.Fatal(err)
			}
			move(nil)
			wantRefusal(false, preview["revision"].(string))

			move(milestone)
			apply(t, s, closeRequest)
			state, err := s.Read(false)
			if err != nil {
				t.Fatal(err)
			}
			topic := state["topics"].(Object)[id.(string)].(Object)
			snapshot := topic["closure"].(Object)["memberships"].(Object)
			if topic["status"] != "closed" || snapshot["milestone_id"] != milestone || snapshot["workset_id"] != tc.workset || snapshot["line_id"] != "mainline" {
				t.Fatalf("unexpected delivery snapshot: %v", snapshot)
			}
		})
	}
}
