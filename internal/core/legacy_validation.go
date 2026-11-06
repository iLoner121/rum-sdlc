package core

import "strings"

const legacySHA = `(?:[0-9a-f]{40}|[0-9a-f]{64})`
const legacyMilestone = `M(?:0|[1-9][0-9]*)`
const legacyIteration = `ITER-(?:00[1-9]|0[1-9][0-9]|[1-9][0-9]{2,})`
const legacyStates = "planned active paused blocked review ready closed"

func legacyVersion(v any, planning bool) bool {
	if _, ok := v.(string); ok {
		return contains("3.2.0 3.2.1 3.2.2", str(v))
	}
	if !integer(v) {
		return false
	}
	if planning {
		return num(v) == 1
	}
	return num(v) == 3
}
func legacyBranch(v any) string {
	s := text(v, "branch")
	need(s != "@" && s != "HEAD" && !strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "/") && !strings.HasSuffix(s, "/") && !strings.HasSuffix(s, ".") && !strings.Contains(s, "..") && !strings.Contains(s, "@{") && !strings.Contains(s, "//"), "Invalid Git branch")
	for _, c := range s {
		need(c > 32 && c != 127 && !strings.ContainsRune(`~^:?*[\`, c), "Invalid Git branch")
	}
	for _, part := range strings.Split(s, "/") {
		need(!strings.HasPrefix(part, ".") && !strings.HasSuffix(part, ".lock"), "Invalid Git branch")
	}
	return s
}
func legacyOptional(o Object, k string) {
	if o[k] != nil {
		text(o[k], k)
	}
}
func legacyNotes(v any, decision bool) {
	o := obj(v, "history event")
	names := "date reason evidence"
	if decision {
		names += " decision"
	}
	fields(o, names, "history event")
	date(o["date"])
	for _, k := range strings.Fields(names) {
		if k != "date" {
			text(o[k], k)
		}
	}
}
func legacyTarget(v any, main string) {
	o := obj(v, "integration target")
	fields(o, "line target_branch", "integration target")
	if o["line"] != nil {
		slug(o["line"])
	}
	legacyBranch(o["target_branch"])
	need(main == "" || o["line"] != nil || o["target_branch"] == main, "Topic target does not match main branch")
}
func legacyList(o Object, k string) []any {
	if !has(o, k) {
		return []any{}
	}
	return arr(o[k], k)
}
func legacyAssignments(o Object) string {
	previous := date(o["created_on"])
	latest := Object{}
	var target Object
	for _, v := range legacyList(o, "assignment_changes") {
		event := obj(v, "assignment event")
		fields(event, "kind before after", "assignment event")
		legacyNotes(event, true)
		kind := enum(event["kind"], "target milestone", "assignment kind")
		need(date(event["date"]) >= previous, "Assignment history dates must be chronological")
		previous = str(event["date"])
		need(!has(latest, kind) || equal(event["before"], latest[kind]), "Assignment history is discontinuous")
		if kind == "milestone" {
			for _, k := range []string{"before", "after"} {
				if event[k] != nil {
					match(event[k], legacyMilestone, "milestone assignment")
				}
			}
		} else {
			legacyTarget(event["before"], "")
			legacyTarget(event["after"], "")
			fields(event, "baseline previous_head prepared_head retained_ref dependencies previous_status", "target event")
			match(event["baseline"], legacySHA, "migration baseline")
			for _, k := range []string{"previous_head", "prepared_head"} {
				if event[k] != nil {
					match(event[k], legacySHA, k)
				}
			}
			legacyOptional(event, "retained_ref")
			text(event["dependencies"], "dependency assessment")
			enum(event["previous_status"], "planned active paused blocked review ready", "previous Topic status")
			target = event
		}
		need(!equal(event["before"], event["after"]), "No-op assignment must not append history")
		latest[kind] = event["after"]
	}
	need(!has(latest, "milestone") || equal(latest["milestone"], o["milestone"]), "Milestone differs from latest assignment event")
	need(!has(latest, "target") || equal(latest["target"], Object{"line": o["line"], "target_branch": o["target_branch"]}), "Integration target differs from latest assignment event")
	if target != nil {
		check := obj(o["integration_recheck"], "integration_recheck")
		need(equal(check["baseline"], target["baseline"]), "Latest target migration requires its matching revalidation record")
	}
	if o["integration_recheck"] != nil {
		check := obj(o["integration_recheck"], "integration_recheck")
		fields(check, "required baseline target_branch migration_date evidence", "integration_recheck")
		required, ok := check["required"].(bool)
		need(ok, "Revalidation required must be a boolean")
		match(check["baseline"], legacySHA, "revalidation baseline")
		need(equal(check["target_branch"], o["target_branch"]), "Revalidation target mismatch")
		need(date(check["migration_date"]) >= date(o["created_on"]), "Revalidation precedes creation")
		if target != nil {
			need(equal(check["migration_date"], target["date"]), "Revalidation migration date mismatch")
		}
		if required {
			need(check["evidence"] == nil, "Pending revalidation cannot claim completed evidence")
			outcome := ""
			if o["closure"] != nil {
				outcome = str(obj(o["closure"], "closure")["outcome"])
			}
			need(!contains("review ready", str(o["status"])) && !(o["status"] == "closed" && contains("delivered research-complete", outcome)), "Revalidate the new integration baseline before review/ready/delivery")
		} else {
			fields(check, "verified_on verified_head", "completed revalidation")
			text(check["evidence"], "revalidation evidence")
			match(check["verified_head"], legacySHA, "verified_head")
			need(date(check["verified_on"]) >= date(check["migration_date"]), "Revalidation date precedes migration")
		}
	}
	return previous
}
func validateLegacyTopic(o Object, id, main string) {
	fields(o, "protocol_version id title slug type status created_on milestone branch dependencies line target_branch closure", "Topic")
	match(o["id"], TopicPattern, "Topic ID")
	need(o["id"] == id, "Topic identity/path mismatch")
	text(o["title"], "Topic title")
	slug(o["slug"])
	kind := enum(o["type"], "feat fix spike refactor docs chore", "Topic type")
	status := enum(o["status"], legacyStates, "Topic status")
	created := date(o["created_on"])
	need(strings.Split(id, "-")[1] == strings.ReplaceAll(created, "-", ""), "Topic ID date differs from created_on")
	legacyBranch(o["branch"])
	need(o["branch"] == kind+"/"+id+"-"+str(o["slug"]), "Topic branch must match type/ID/slug")
	deps := ids(o["dependencies"], TopicPattern, "dependencies")
	need(!asSet(deps)[id], "Topic cannot depend on itself")
	if o["milestone"] != nil {
		match(o["milestone"], legacyMilestone, "milestone")
	}
	legacyTarget(o, main)
	if status != "closed" {
		need(o["closure"] == nil, "Open Topic cannot have closure metadata")
	} else {
		c := obj(o["closure"], "Topic closure")
		fields(c, "outcome closed_on summary verification integration_commit integration_target delivery_scope retained_ref successor", "Topic closure")
		outcome := enum(c["outcome"], "delivered research-complete cancelled superseded", "Topic outcome")
		text(c["summary"], "closure summary")
		text(c["verification"], "closure verification")
		need(date(c["closed_on"]) >= created, "Closure precedes creation")
		need(equal(c["integration_target"], o["target_branch"]), "Closure integration target mismatch")
		enum(c["delivery_scope"], "mainline development-line maintenance-line", "delivery_scope")
		need((c["delivery_scope"] == "mainline") == (o["line"] == nil), "Closure delivery scope mismatch")
		if contains("delivered research-complete", outcome) {
			match(c["integration_commit"], legacySHA, "integration_commit")
			need(outcome != "research-complete" || kind == "spike", "research-complete requires spike type")
		} else {
			need(c["integration_commit"] == nil, "Non-delivery closure cannot claim integration_commit")
			text(c["retained_ref"], "retained code/ref or explicit discard decision")
		}
		if outcome == "superseded" {
			match(c["successor"], TopicPattern, "Topic successor")
			need(c["successor"] != id, "Topic cannot supersede itself")
		} else {
			need(c["successor"] == nil, "successor only applies to superseded")
		}
		legacyOptional(c, "retained_ref")
	}
	last := legacyAssignments(o)
	if status == "closed" {
		need(date(obj(o["closure"], "closure")["closed_on"]) >= last, "Closure precedes assignment history")
	}
}
func validateLegacyLine(o Object, id, main string) {
	fields(o, "protocol_version id kind branch target_branch status closure", "Line")
	slug(o["id"])
	need(o["id"] == id, "Line identity/path mismatch")
	kind := enum(o["kind"], "development maintenance", "Line kind")
	status := enum(o["status"], "active paused integrating closed", "Line status")
	need(kind != "maintenance" || status != "integrating", "Maintenance lines cannot enter integrating")
	prefix := "line/"
	if kind == "maintenance" {
		prefix = "maint/"
	}
	need(o["branch"] == prefix+id, "Invalid Line branch")
	if kind == "maintenance" {
		need(o["target_branch"] == nil, "Maintenance Line target must be null")
	} else {
		legacyBranch(o["target_branch"])
		need(main == "" || o["target_branch"] == main, "Invalid Line target")
	}
	modern := has(o, "created_on") || has(o, "base_commit")
	if modern {
		fields(o, "created_on base_commit state_history", "Line creation metadata")
	}
	last := ""
	if has(o, "created_on") {
		last = date(o["created_on"])
	}
	if has(o, "base_commit") {
		match(o["base_commit"], legacySHA, "Line baseline")
	}
	history := legacyList(o, "state_history")
	if modern && len(history) == 0 {
		need(status == "active", "A new Line starts active; later states require history")
	}
	previous := ""
	paths := map[string]string{"active": "paused integrating closed", "paused": "active integrating closed", "integrating": "active paused closed", "closed": ""}
	for _, v := range history {
		event := obj(v, "Line history event")
		fields(event, "from to", "Line history event")
		legacyNotes(event, false)
		old := enum(event["from"], "active paused integrating closed", "Line history source")
		next := enum(event["to"], "active paused integrating closed", "Line history destination")
		need(contains(paths[old], next), "Illegal Line history transition")
		need(previous == "" || old == previous, "Discontinuous Line state history")
		need(kind != "maintenance" || (old != "integrating" && next != "integrating"), "Maintenance lines cannot integrate")
		need(last == "" || date(event["date"]) >= last, "Line history dates must be chronological")
		if next == "paused" {
			text(event["resume_when"], "pause recovery conditions")
		}
		if contains("active integrating", next) {
			match(event["observed_tip"], legacySHA, "observed Line tip")
		}
		previous = next
		last = str(event["date"])
	}
	need(previous == "" || previous == status, "Line status differs from its last history event")
	if status != "closed" {
		need(o["closure"] == nil, "Open line cannot have closure")
		return
	}
	c := obj(o["closure"], "Line closure")
	fields(c, "outcome closed_on summary verification integration_commit retained_ref successor", "Line closure")
	outcomes := "integrated cancelled superseded"
	if kind == "maintenance" {
		outcomes = "retired"
	}
	outcome := enum(c["outcome"], outcomes, "Line outcome")
	text(c["summary"], "Line closure summary")
	text(c["verification"], "Line closure verification")
	ended := date(c["closed_on"])
	need(last == "" || ended >= last, "Line closure precedes history")
	if len(history) > 0 {
		event := obj(history[len(history)-1], "event")
		need(event["to"] == "closed" && equal(event["date"], c["closed_on"]), "Line closure/history mismatch")
	}
	if outcome == "integrated" {
		match(c["integration_commit"], legacySHA, "Line integration commit")
		need(len(history) == 0 || obj(history[len(history)-1], "event")["from"] == "integrating", "Integrated closure requires integrating state")
		need(c["retained_ref"] == nil && c["discard_decision"] == nil, "Integrated closure has incompatible retention fields")
	} else {
		need(c["integration_commit"] == nil, "Non-integrated Line cannot claim integration_commit")
		retained, discarded := truth(c["retained_ref"]), truth(c["discard_decision"])
		need(retained != discarded, "Line closure needs exactly one retention/discard disposition")
		legacyOptional(c, "retained_ref")
		legacyOptional(c, "discard_decision")
		if len(history) > 0 || has(c, "decision") {
			text(c["decision"], "Line closure decision")
		}
		if outcome == "retired" {
			need(retained && !discarded, "Retired Line must retain code")
			if len(history) > 0 || has(c, "last_release_ref") {
				text(c["last_release_ref"], "last release ref")
				match(c["last_release_commit"], legacySHA, "last release commit")
			}
		}
	}
	if outcome == "superseded" {
		slug(c["successor"])
		need(c["successor"] != id, "Line cannot supersede itself")
	} else {
		need(c["successor"] == nil, "Line successor only applies to superseded")
	}
}
func validateLegacyPlanning(o Object, kind, id string) {
	fields(o, "schema_version id title created_on status closure", kind)
	pattern, states := legacyIteration, "draft active closed"
	if kind == "milestone" {
		pattern, states = legacyMilestone, "open closed"
	}
	match(o["id"], pattern, "planning ID")
	need(o["id"] == id, "Planning identity/path mismatch")
	text(o["title"], "planning title")
	date(o["created_on"])
	status := enum(o["status"], states, "planning status")
	var c Object
	if status == "closed" {
		c = obj(o["closure"], "planning closure")
		fields(c, "outcome closed_on summary", "planning closure")
		text(c["summary"], "closure summary")
		date(c["closed_on"])
	} else {
		need(o["closure"] == nil, "Open planning record has closure")
	}
	if has(o, "closure_review") {
		legacyNotes(o["closure_review"], true)
		need(status == "closed" && equal(obj(o["closure_review"], "closure review")["date"], c["closed_on"]), "Closure review mismatch")
	}
	if kind == "milestone" {
		if status == "closed" {
			text(c["evidence"], "milestone evidence")
			outcome := enum(c["outcome"], "completed cancelled superseded", "milestone outcome")
			need(date(c["closed_on"]) >= date(o["created_on"]), "Closure precedes creation")
			if outcome == "superseded" {
				match(c["successor"], legacyMilestone, "milestone successor")
				need(c["successor"] != id, "Milestone cannot supersede itself")
			} else {
				need(c["successor"] == nil, "successor only applies to superseded")
			}
		}
		return
	}
	fields(o, "started_on last_reviewed_on initial_topics topics changes", "iteration")
	ids(o["initial_topics"], TopicPattern, "initial_topics")
	ids(o["topics"], TopicPattern, "topics")
	for _, k := range []string{"started_on", "last_reviewed_on"} {
		if o[k] != nil {
			date(o[k])
		}
	}
	for _, k := range []string{"changes", "draft_changes", "reviews"} {
		for _, v := range legacyList(o, k) {
			event := obj(v, k+" event")
			date(event["date"])
			if k == "reviews" {
				legacyNotes(event, true)
				continue
			}
			topics := ids(event["topics"], TopicPattern, "change topics")
			text(event["reason"], "change reason")
			text(event["impact"], "change impact")
			action := enum(event["action"], "add remove reorder", "scope action")
			need(action == "reorder" || len(topics) > 0, "Add/remove change cannot be empty")
			if k == "draft_changes" {
				ids(event["before"], TopicPattern, "draft before")
				ids(event["after"], TopicPattern, "draft after")
			}
			if has(event, "decision") || has(event, "evidence") || k == "draft_changes" {
				legacyNotes(event, true)
				if action == "remove" {
					enum(event["destination"], "backlog next-iteration out-of-scope", "removal destination")
				}
			}
		}
	}
	if has(o, "activation") {
		legacyNotes(o["activation"], true)
	}
	if status == "closed" {
		for _, v := range arr(c["results"], "iteration results") {
			result := obj(v, "iteration result")
			fields(result, "topic disposition note evidence", "iteration result")
			match(result["topic"], TopicPattern, "result Topic")
			enum(result["disposition"], "completed cancelled superseded carry-forward backlog", "result disposition")
			text(result["note"], "result note")
			text(result["evidence"], "result evidence")
		}
	}
	legacyScope(o)
}
func legacyScope(o Object) {
	current := append([]any{}, arr(o["initial_topics"], "initial_topics")...)
	if o["status"] == "draft" {
		current = append([]any{}, arr(o["topics"], "topics")...)
	}
	touched := asSet(current)
	for _, v := range arr(o["changes"], "changes") {
		event := obj(v, "change")
		selected := arr(event["topics"], "topics")
		existing := asSet(current)
		switch event["action"] {
		case "add":
			for _, id := range selected {
				need(!existing[str(id)], "Duplicate scope addition")
				touched[str(id)] = true
			}
			current = append(current, selected...)
		case "remove":
			for _, id := range selected {
				need(existing[str(id)], "Removing an absent Topic")
			}
			remove := asSet(selected)
			next := []any{}
			for _, id := range current {
				if !remove[str(id)] {
					next = append(next, id)
				}
			}
			current = next
		default:
			need(sameSet(asSet(selected), existing), "Invalid scope reorder")
			current = append([]any{}, selected...)
		}
	}
	need(equal(current, o["topics"]), "Source iteration scope differs from its history")
	created := date(o["created_on"])
	if o["started_on"] != nil {
		need(date(o["started_on"]) >= created && date(o["last_reviewed_on"]) >= date(o["started_on"]), "Invalid iteration chronology")
	}
	if o["status"] == "closed" {
		c := obj(o["closure"], "closure")
		need(date(c["closed_on"]) >= created, "Closure precedes creation")
		seen := map[string]bool{}
		for _, v := range arr(c["results"], "results") {
			id := str(obj(v, "result")["topic"])
			need(!seen[id], "Source iteration closure does not cover selected history")
			seen[id] = true
		}
		need(sameSet(seen, touched), "Source iteration closure does not cover selected history")
	}
}

// ValidateLegacyRecord rejects unsupported or unreconciled source metadata.
func ValidateLegacyRecord(data Object, kind, id, main string) (err error) {
	defer catch(&err)
	version := data["schema_version"]
	planning := kind != "topic" && kind != "line"
	if !planning {
		version = data["protocol_version"]
	}
	need(legacyVersion(version, planning), "Unsupported/missing source entity version")
	switch kind {
	case "topic":
		validateLegacyTopic(data, id, main)
	case "line":
		validateLegacyLine(data, id, main)
	case "milestone", "iteration":
		validateLegacyPlanning(data, kind, id)
	default:
		need(false, "Unknown source entity kind")
	}
	return
}
