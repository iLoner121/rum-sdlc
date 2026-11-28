package core

const topicPriorityLevels = "P0 P1 P2 P3"
const topicPriorityNamespace = "rum-topic-priority"

// normalizeTopicPriority records the change in the existing extension event
// format so earlier runtimes can still replay and synchronize the Store.
func normalizeTopicPriority(request Object) {
	p := obj(request["params"], "params")
	need(has(p, "priority"), "Topic priority is required; use null to clear it")
	value := Object{}
	if p["priority"] != nil {
		value["level"] = enum(p["priority"], topicPriorityLevels, "Topic priority")
	}
	request["action"] = "extension.set"
	request["params"] = Object{
		"table": "topics", "id": p["id"], "namespace": topicPriorityNamespace, "value": value,
	}
}

// TopicPriority returns the effective level, or an empty string when unset.
func TopicPriority(topic Object) string {
	ext, _ := topic["extensions"].(Object)
	value, _ := ext[topicPriorityNamespace].(Object)
	level := str(value["level"])
	if contains(topicPriorityLevels, level) {
		return level
	}
	return ""
}

// ValidTopicPriorityFilter accepts the four levels and the unset selector.
func ValidTopicPriorityFilter(filter string) bool {
	return contains(topicPriorityLevels+" unset", filter)
}

func MatchesTopicPriority(level, filter string) bool {
	return filter == "" || level == filter || filter == "unset" && level == ""
}

func withTopicPriority(row, topic Object) Object {
	if level := TopicPriority(topic); level != "" {
		row["priority"] = level
	}
	return row
}
