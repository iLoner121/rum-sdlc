package core

import (
	"fmt"
	"regexp"
	"strings"
)

// TopicNumbers derives project-local labels from append-only creation history.
// Closed Topics stay in the sequence. Canonical IDs, documents and history are
// never renamed. Unshared, divergent histories may have conflicting labels.
func TopicNumbers(s Object) map[string]string {
	numbers := map[string]string{}
	if s == nil {
		return numbers
	}
	for _, value := range arr(s["history"], "history") {
		event := obj(value, "event")
		for _, value := range arr(event["changes"], "changes") {
			change := obj(value, "change")
			path := arr(change["path"], "path")
			if len(path) == 2 && path[0] == "topics" && change["before"] == nil && change["after"] != nil {
				id := str(path[1])
				if numbers[id] == "" {
					numbers[id] = fmt.Sprintf("T%d", len(numbers)+1)
				}
			}
		}
	}
	// Dispatch has not sealed a newly created Topic into history yet.
	for _, id := range keys(rows(s, "topics")) {
		if numbers[id] == "" {
			numbers[id] = fmt.Sprintf("T%d", len(numbers)+1)
		}
	}
	return numbers
}

// CanonicalReference resolves Tn and Tn/tNNN without touching free-form text.
// Long identities pass through, preserving the existing boundary validators.
func CanonicalReference(s Object, reference string) (id string, err error) {
	defer catch(&err)
	return canonicalReference(s, TopicNumbers(s), reference), nil
}
func canonicalReference(s Object, numbers map[string]string, reference string) string {
	parts := strings.SplitN(reference, "/", 2)
	label := parts[0]
	if !strings.HasPrefix(label, "T") || strings.HasPrefix(label, "T-") {
		return reference
	}
	need(regexp.MustCompile(`^T[1-9][0-9]*$`).MatchString(label), "Invalid Topic number; use T1, T2, ...")
	for id, number := range numbers {
		if number == label && has(rows(s, "topics"), id) {
			if len(parts) == 2 {
				return id + "/" + parts[1]
			}
			return id
		}
	}
	need(false, "Unknown Topic number: "+label)
	return ""
}

// ShortReference is additive presentation data; it is never persisted as identity.
func ShortReference(s Object, reference string) string {
	parts := strings.SplitN(reference, "/", 2)
	label := TopicNumbers(s)[parts[0]]
	if label == "" {
		return ""
	}
	if len(parts) == 2 {
		return label + "/" + parts[1]
	}
	return label
}

func numberedResult(s Object, value Object) Object {
	if value == nil {
		return nil
	}
	result := clone(value).(Object)
	numbers := TopicNumbers(s)
	var decorate func(Object)
	decorate = func(row Object) {
		if id := str(row["id"]); numbers[id] != "" {
			row["short_id"] = numbers[id]
		}
		if id := str(row["topic_id"]); numbers[id] != "" {
			row["short_id"] = numbers[id]
		}
		if id := str(row["topic"]); numbers[id] != "" {
			row["short_id"] = numbers[id]
		}
		if parts := strings.SplitN(str(row["id"]), "/", 2); len(parts) == 2 && numbers[parts[0]] != "" {
			row["short_id"] = numbers[parts[0]] + "/" + parts[1]
			row["topic_short_id"] = numbers[parts[0]]
		}
		for _, key := range []string{"entity", "relations"} {
			if child, ok := row[key].(Object); ok {
				decorate(child)
			}
		}
		if children, ok := row["rows"].([]any); ok {
			for _, child := range children {
				if child, ok := child.(Object); ok {
					decorate(child)
				}
			}
		}
	}
	decorate(result)
	return result
}

func canonicalParams(s Object, action string, original Object) Object {
	p := clone(original).(Object)
	numbers := TopicNumbers(s)
	ref := func(value any) any {
		if text, ok := value.(string); ok {
			return canonicalReference(s, numbers, text)
		}
		return value
	}
	if action != "topic.create" && (strings.HasPrefix(action, "topic.") || strings.HasPrefix(action, "task.") || contains("topics tasks", str(p["table"]))) {
		if has(p, "id") {
			p["id"] = ref(p["id"])
		}
	}
	if has(p, "topic_id") {
		p["topic_id"] = ref(p["topic_id"])
	}
	if strings.HasPrefix(action, "topic.") || strings.HasPrefix(action, "workset.") {
		for _, key := range []string{"dependencies", "topics"} {
			if values, ok := p[key].([]any); ok {
				for i, value := range values {
					values[i] = ref(value)
				}
			}
		}
		if has(p, "successor") {
			p["successor"] = ref(p["successor"])
		}
		if values, ok := p["results"].([]any); ok {
			for _, value := range values {
				if row, ok := value.(Object); ok {
					row["topic"] = ref(row["topic"])
				}
			}
		}
	}
	for _, key := range []string{"context", "integration_context", "integration_proof"} {
		if value, ok := p[key].(Object); ok {
			value["topic_id"] = ref(value["topic_id"])
		}
	}
	if action == "closure.correct" && p["table"] == "topics" {
		if changes, ok := p["changes"].(Object); ok && has(changes, "successor") {
			changes["successor"] = ref(changes["successor"])
		}
	}
	return p
}
