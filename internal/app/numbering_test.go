package app

import (
	"context"
	"rumsdlc/internal/core"
	"strings"
	"testing"
)

func TestShortTopicCodeAssociationAndOfflineObservation(t *testing.T) {
	d, _, _, root := setupDevices(t)
	create(t, d, root)
	created := command(t, d, "topic.create", Object{"name": "Topic", "type": "feat", "slug": "topic", "unassigned_reason": "Planned"})
	id := str(created["result"])
	result := call(t, d, "bind-code", Object{"project_id": "P-demo", "topic": "T1", "branch": "feat/custom-name"})
	code := object(object(result["repositories"], "repositories")["code"], "code")
	if object(code["topic_branches"], "branches")[id] != "feat/custom-name" {
		t.Fatal(result)
	}
	if _, err := d.Call(context.Background(), "observe-code", Object{"project_id": "P-demo", "topic": "T1"}); err != nil {
		t.Fatal(err)
	}
}
func TestOfflineAllocationConflictAndExplicitReplayRecovery(t *testing.T) {
	step := func(s Object, action string, p Object) (Object, string) {
		after, result, err := core.Dispatch(s, core.Request(action, p, "2026-11-06", "test", "evidence", "decision"))
		if err != nil {
			t.Fatal(err)
		}
		return after, str(result)
	}
	makeTopic := func(name string) Object {
		return Object{"name": name, "type": "feat", "slug": "topic", "unassigned_reason": "Planned"}
	}
	base, _ := step(nil, "project.init", Object{"id": "P-test", "name": "Test"})
	base, shared := step(base, "topic.create", makeTopic("Shared"))
	local, localID := step(base, "topic.create", makeTopic("Local"))
	remote, remoteID := step(base, "topic.create", makeTopic("Remote"))
	beforeLocal, beforeRemote := fingerprint(local), fingerprint(remote)
	err := attempt(func() { mergeState(local, remote) })
	if err == nil || !strings.Contains(err.Error(), "T2") || !strings.Contains(err.Error(), localID) || !strings.Contains(err.Error(), remoteID) {
		t.Fatal("missing allocation conflict", err)
	}
	if fingerprint(local) != beforeLocal || fingerprint(remote) != beforeRemote {
		t.Fatal("conflict mutated inputs")
	}
	// Explicit recovery appends the remote creation using its canonical identity;
	// the agreed common history and local allocation keep their numbers.
	params := makeTopic("Remote")
	params["id"] = remoteID
	params["id_reason"] = "Explicit offline conflict recovery"
	recovered, _ := step(local, "topic.create", params)
	if core.ShortReference(recovered, shared) != "T1" || core.ShortReference(recovered, localID) != "T2" || core.ShortReference(recovered, remoteID) != "T3" {
		t.Fatal(core.TopicNumbers(recovered))
	}
	if err := core.Validate(recovered); err != nil {
		t.Fatal(err)
	}
}
