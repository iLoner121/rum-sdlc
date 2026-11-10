package app

import (
	"fmt"
	"rumsdlc/internal/core"
	"sort"
	"strings"
)

func mergeState(left, right Object) Object {
	must(core.Validate(left))
	must(core.Validate(right))
	a, b := array(left["history"], "history"), array(right["history"], "history")
	common := 0
	for common < len(a) && common < len(b) && equal(a[common], b[common]) {
		common++
	}
	require(common > 0, "No common business history")
	require(common < len(a) && common < len(b), "Not a divergent business history")
	suffixes := [][]any{a[common:], b[common:]}
	paths := [2]map[string]bool{{}, {}}
	type entry struct {
		date, hash string
		index      int
		event      Object
	}
	entries := []entry{}
	for side, events := range suffixes {
		hash := fingerprint(events)
		for index, v := range events {
			event := object(v, "event")
			if !oneOf(str(event["action"]), "entity.rename extension.set task.correct") {
				collisions := []string{}
				a, b := core.TopicNumbers(left), core.TopicNumbers(right)
				otherByNumber := map[string]string{}
				for id, number := range b {
					otherByNumber[number] = id
				}
				for _, id := range sortedKeys(a) {
					if other := otherByNumber[a[id]]; other != "" && id != other {
						collisions = append(collisions, fmt.Sprintf("%s: local %s / remote %s", a[id], id, other))
					}
				}
				detail := ""
				if len(collisions) > 0 {
					detail = "; Topic number conflicts (unshared allocations): " + strings.Join(collisions, ", ") + ". Preserve both canonical IDs and replay allocations in an explicitly selected order from the common history"
				}
				require(false, "Concurrent lifecycle/allocation changes require explicit business resolution"+detail)
			}
			for _, v := range array(event["changes"], "changes") {
				paths[side][fingerprint(object(v, "change")["path"])] = true
			}
			entries = append(entries, entry{str(event["date"]), hash, index, event})
		}
	}
	for path := range paths[0] {
		require(!paths[1][path], "Concurrent changes affect the same business entity")
	}
	var state Object
	for _, v := range a[:common] {
		next, _, e := core.Dispatch(state, object(object(v, "event")["request"], "request"))
		must(e)
		state = next
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.date != b.date {
			return a.date < b.date
		}
		if a.hash != b.hash {
			return a.hash < b.hash
		}
		return a.index < b.index
	})
	for _, entry := range entries {
		after, _, e := core.Dispatch(state, object(entry.event["request"], "request"))
		must(e)
		history := array(after["history"], "history")
		require(integer(after["revision"]) == integer(state["revision"])+1 && equal(object(history[len(history)-1], "event")["changes"], entry.event["changes"]), "Replaying would change the original business effect")
		state = after
	}
	must(core.Validate(state))
	return state
}
func attempt(fn func()) (err error) { defer guard(&err); fn(); return nil }

func sortedKeys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
