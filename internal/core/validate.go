package core

import (
	"strconv"
	"strings"
)

func truth(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case Object:
		return len(x) > 0
	default:
		return !equal(v, 0)
	}
}
func deliveryScope(kind string) string {
	return map[string]string{"mainline": "mainline", "development": "development-line", "maintenance": "maintenance-line"}[kind]
}
func validateContext(value any, topic string, source, target any) {
	c := obj(value, "Integration context")
	exact(c, "topic_id from_line_id line_id dependencies contracts extensions", "Incomplete/unknown integration context")
	need(c["topic_id"] == topic && equal(c["from_line_id"], source) && equal(c["line_id"], target), "Integration context endpoints mismatch")
	text(c["dependencies"], "dependencies assessment")
	text(c["contracts"], "contracts assessment")
	extensions(c["extensions"])
}
func validateProof(value any, topic string, pending Object) {
	p := obj(value, "Integration proof")
	exact(p, "topic_id from_line_id line_id dependencies contracts extensions verification verified_on report", "Incomplete/unknown integration proof")
	need(pending["context"] != nil, "Prepare an integration context before verification")
	c := obj(pending["context"], "context")
	validateContext(c, topic, c["from_line_id"], pending["line_id"])
	for k, v := range c {
		need(equal(p[k], v), "Proof differs from selected integration context")
	}
	extensions(p["verification"])
	date(p["verified_on"])
	text(p["report"], "Integration verification report")
}

func cycles(edges map[string][]string, label string) {
	done, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		need(!visiting[id], label+" cycle at "+id)
		if done[id] {
			return
		}
		visiting[id] = true
		for _, next := range edges[id] {
			visit(next)
		}
		delete(visiting, id)
		done[id] = true
	}
	for id := range edges {
		visit(id)
	}
}

// Validate checks a complete snapshot, including replay of its audit history.
func Validate(s Object) (err error) {
	defer catch(&err)
	exact(s, "project milestones worksets lines topics tasks relations version revision history", "Unexpected/missing state tables")
	need(contains("4.1.0 4.1.1 4.1.2", str(s["version"])), "Unsupported workflow version; explicit migration required")
	need(integer(s["revision"]) && num(s["revision"]) >= 1, "Invalid revision")
	history := arr(s["history"], "history")
	need(len(history) == num(s["revision"]), "Invalid history length")
	p := obj(s["project"], "Project")
	project := match(p["id"], `P-[a-z0-9]+(?:-[a-z0-9]+)*`, "Project ID")
	extras := map[string]string{"project": "pool_id default_line_id", "milestones": "position", "worksets": "type proposed_topics initial_topics activation_revision started_on last_reviewed_on", "lines": "type target_line_id", "topics": "type slug dependencies integration_recheck", "tasks": "topic_id kind result destination"}
	choices := map[string]string{"project": "active closed", "milestones": "planned active closed", "worksets": "draft active closed", "lines": "active paused integrating closed", "topics": "planned active review ready paused blocked closed", "tasks": "planned active blocked deferred done cancelled"}
	patterns := map[string]string{"milestones": `M(?:0|[1-9][0-9]*)`, "worksets": `(?:POOL|ITER-(?:00[1-9]|0[1-9][0-9]|[1-9][0-9]{2,}))`, "lines": SlugPattern, "topics": TopicPattern, "tasks": TopicPattern + `/t(?:00[1-9]|0[1-9][0-9]|[1-9][0-9]{2,})`}
	for _, table := range tables {
		for id, v := range rows(s, table) {
			row := obj(v, table)
			fields(row, "version project_id id name created_on status closure extensions", table)
			fields(row, extras[table], table)
			extensions(row["extensions"])
			only(row, "version project_id id name created_on status closure legacy extensions "+extras[table], "Unknown managed fields; put custom content in documents")
			need(row["id"] == id && row["project_id"] == project && row["version"] == s["version"], "Entity identity/project/version mismatch")
			text(row["name"], "name")
			if row["created_on"] != nil {
				date(row["created_on"])
			} else {
				need(has(row, "legacy"), "Unknown creation date only allowed for explicit historical import")
			}
			if table != "project" {
				match(id, patterns[table], table+" ID")
			}
			if table == "lines" {
				slug(id)
			}
			enum(row["status"], choices[table], table+" status")
			if table == "tasks" {
				need(row["closure"] == nil, "Task has no entity closure")
			} else {
				_, isObject := row["closure"].(Object)
				need((row["status"] == "closed") == isObject, "Closure/status mismatch")
				if row["closure"] != nil {
					c := obj(row["closure"], "closure")
					fields(c, "outcome closed_on summary evidence", "closure")
					date(c["closed_on"])
					for _, k := range []string{"outcome", "summary", "evidence"} {
						text(c[k], k)
					}
					if truth(row["created_on"]) {
						need(str(c["closed_on"]) >= str(row["created_on"]), "Closure precedes creation")
					}
				}
			}
		}
	}
	need(p["project_id"] == p["id"], "Project self identity mismatch")
	pools, iterations := 0, 0
	for id, v := range rows(s, "worksets") {
		w := obj(v, "workset")
		enum(w["type"], "pool iteration", "workset type")
		need((id == "POOL") == (w["type"] == "pool"), "Workset type/ID mismatch")
		if w["type"] == "pool" {
			pools++
			need(id == p["pool_id"] && w["status"] == "active", "Exactly one active Pool per project")
		} else if w["status"] != "closed" {
			iterations++
		}
		for _, k := range []string{"proposed_topics", "initial_topics"} {
			for _, t := range ids(w[k], TopicPattern, "Topic IDs") {
				need(has(rows(s, "topics"), str(t)), "Missing proposed/initial Topic")
			}
		}
		for _, k := range []string{"started_on", "last_reviewed_on"} {
			if w[k] != nil {
				date(w[k])
			}
		}
		if w["activation_revision"] != nil {
			need(integer(w["activation_revision"]) && num(w["activation_revision"]) >= 1 && num(w["activation_revision"]) <= num(s["revision"]), "Invalid activation revision")
			need(w["started_on"] != nil && w["last_reviewed_on"] != nil && str(w["last_reviewed_on"]) >= str(w["started_on"]), "Invalid iteration dates")
		}
		if w["type"] == "iteration" {
			if w["status"] == "draft" {
				need(w["activation_revision"] == nil && w["started_on"] == nil && w["last_reviewed_on"] == nil && len(arr(w["initial_topics"], "initial_topics")) == 0, "Draft has no activated scope/history")
			}
			if w["status"] == "active" {
				need(w["activation_revision"] != nil, "Iteration requires activation")
			}
		}
	}
	need(pools == 1, "Exactly one active Pool per project")
	need(iterations <= 1, "At most one draft/active iteration")
	positions := map[int]bool{}
	active := 0
	for _, v := range rows(s, "milestones") {
		m := obj(v, "milestone")
		need(integer(m["position"]) && num(m["position"]) >= 0, "Invalid milestone position")
		n := num(m["position"])
		need(!positions[n], "Duplicate milestone position")
		positions[n] = true
		if m["status"] == "active" {
			active++
		}
	}
	need(active <= 1, "At most one active milestone")
	mains := 0
	for id, v := range rows(s, "lines") {
		l := obj(v, "Line")
		kind := enum(l["type"], "mainline development maintenance", "Line type")
		switch kind {
		case "mainline":
			mains++
			need(l["status"] == "active" && l["target_line_id"] == nil, "Mainline is permanent within the project")
			need(id == p["default_line_id"], "Exactly one default mainline")
		case "development":
			need(l["target_line_id"] == p["default_line_id"], "Development Line targets project mainline")
		case "maintenance":
			need(l["target_line_id"] == nil && l["status"] != "integrating", "Maintenance Line cannot integrate wholesale")
		}
	}
	need(mains == 1, "Exactly one default mainline")
	relations := obj(s["relations"], "relations")
	exact(relations, "milestone workset line", "Invalid relation tables")
	for kind, v := range relations {
		relations := obj(v, kind+" relations")
		parents := rows(s, map[string]string{"milestone": "milestones", "workset": "worksets", "line": "lines"}[kind])
		slots := map[string]map[int]bool{}
		for id, v := range relations {
			row := obj(v, "relation")
			names := "version project_id topic_id parent_id"
			if kind == "workset" {
				names += " position"
			}
			exact(row, names, "Invalid relation fields")
			need(row["version"] == s["version"] && row["project_id"] == project && row["topic_id"] == id, "Relation identity/project/version mismatch")
			parent := str(row["parent_id"])
			need(has(rows(s, "topics"), id) && has(parents, parent), "Missing relation endpoint")
			t := entity(s, "topics", id)
			par := obj(parents[parent], "parent")
			if t["status"] != "closed" {
				need(par["status"] != "closed", "Open Topic assigned to closed parent")
			}
			if kind == "workset" {
				need(t["status"] != "closed", "Closed Topic must not occupy current work queue")
				need(par["status"] == "active", "Current workset must be active")
				need(integer(row["position"]) && num(row["position"]) >= 0, "Invalid member position")
				if slots[parent] == nil {
					slots[parent] = map[int]bool{}
				}
				n := num(row["position"])
				need(!slots[parent][n], "Duplicate member position")
				slots[parent][n] = true
			}
		}
	}
	dependencies, successors := map[string][]string{}, map[string][]string{}
	for id, v := range rows(s, "topics") {
		t := obj(v, "Topic")
		enum(t["type"], "feat fix spike refactor docs chore", "Topic type")
		slug(t["slug"])
		deps := ids(t["dependencies"], TopicPattern, "Topic IDs")
		need(t["created_on"] != nil && strings.Split(id, "-")[1] == strings.ReplaceAll(str(t["created_on"]), "-", ""), "Topic ID/creation date mismatch")
		for _, dep := range deps {
			d := str(dep)
			need(d != id && has(rows(s, "topics"), d), "Invalid Topic dependencies")
			dependencies[id] = append(dependencies[id], d)
		}
		need(relation(s, "line", id) != nil, "Topic requires a Line")
		if t["status"] != "closed" {
			need(relation(s, "workset", id) != nil, "Open Topic requires a current workset")
		}
		line := entity(s, "lines", str(relation(s, "line", id)))
		if line["status"] == "integrating" {
			need(contains("review ready paused blocked closed", str(t["status"])), "Integrating Line cannot have planned/active members")
		}
		if t["integration_recheck"] != nil {
			pending := obj(t["integration_recheck"], "Invalid recheck")
			exact(pending, "required line_id evidence context proof", "Invalid integration recheck fields")
			_, ok := pending["required"].(bool)
			need(ok && pending["line_id"] == line["id"], "Recheck mismatch")
			c := obj(pending["context"], "Required integration context")
			validateContext(c, id, c["from_line_id"], line["id"])
			need(has(rows(s, "lines"), str(c["from_line_id"])) && c["from_line_id"] != line["id"], "Invalid source integration Line")
			if !boolean(pending["required"]) {
				text(pending["evidence"], "New baseline verification")
				validateProof(pending["proof"], id, pending)
				verified := str(obj(pending["proof"], "proof")["verified_on"])
				need(str(t["created_on"]) <= verified && verified <= str(obj(history[len(history)-1], "history")["date"]), "Invalid verification chronology")
			} else {
				need(pending["evidence"] == nil && pending["proof"] == nil, "Pending revalidation cannot claim verification")
			}
			if contains("review ready", str(t["status"])) || t["status"] == "closed" && contains("delivered research-complete", str(obj(t["closure"], "closure")["outcome"])) {
				need(!boolean(pending["required"]), "New integration target not verified")
			}
		}
		for _, task := range tasksFor(s, id) {
			if t["status"] == "closed" {
				need(!contains("planned active blocked", str(task["status"])), "Closed Topic has unfinished Task")
			}
			if contains("review ready", str(t["status"])) && contains("planned active blocked", str(task["status"])) {
				need(contains("verify process", str(task["kind"])), "Unfinished non-convergence Task")
			}
		}
		if t["status"] == "closed" {
			c := obj(t["closure"], "closure")
			outcome := enum(c["outcome"], "delivered research-complete cancelled superseded", "Topic outcome")
			fields(c, "memberships delivery_record integration_target delivery_scope retention successor verification extensions", "Topic closure")
			need(obj(c["memberships"], "memberships")["line_id"] == line["id"] && c["integration_target"] == line["id"], "Historical delivery target mismatch")
			if contains("delivered research-complete", outcome) {
				text(c["delivery_record"], "Delivery record")
				need(outcome != "research-complete" || t["type"] == "spike", "Research completion requires spike")
			} else {
				need(c["delivery_record"] == nil, "Cancellation cannot claim delivery")
				text(c["retention"], "Work disposition")
			}
			if outcome == "superseded" {
				next := str(c["successor"])
				need(has(rows(s, "topics"), next) && next != id, "Missing successor")
				successors[id] = []string{next}
			}
		}
	}
	for id, v := range rows(s, "tasks") {
		task := obj(v, "Task")
		need(has(rows(s, "topics"), str(task["topic_id"])) && strings.HasPrefix(id, str(task["topic_id"])+"/"), "Task owner mismatch")
		match(task["kind"], `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`, "Task kind")
		if task["status"] == "done" {
			text(task["result"], "Task activity result; not acceptance")
		}
		if task["status"] == "deferred" {
			text(task["destination"], "Deferred destination")
		}
	}
	for _, table := range []string{"milestones", "lines"} {
		edges := map[string][]string{}
		for id, v := range rows(s, table) {
			row := obj(v, table)
			if row["closure"] != nil {
				c := obj(row["closure"], "closure")
				if c["outcome"] == "superseded" {
					next := str(c["successor"])
					need(has(rows(s, table), next) && next != id, "Missing historical successor")
					edges[id] = []string{next}
				}
			}
		}
		cycles(edges, table+" successor")
	}
	for id, v := range rows(s, "worksets") {
		w := obj(v, "workset")
		if w["type"] != "iteration" || w["status"] != "closed" {
			continue
		}
		c := obj(w["closure"], "closure")
		enum(c["outcome"], "completed replanned cancelled", "Iteration closure outcome")
		results := arr(c["results"], "Missing historical iteration results")
		selected := []any{}
		for _, v := range results {
			row := obj(v, "Historical iteration result")
			fields(row, "topic disposition note evidence", "Historical iteration result")
			selected = append(selected, row["topic"])
			need(has(rows(s, "topics"), str(row["topic"])), "Missing historical Topic")
			enum(row["disposition"], "completed cancelled superseded carry-forward backlog", "Historical disposition")
			text(row["note"], "Historical result note")
			text(row["evidence"], "Historical result evidence")
		}
		need(len(selected) == len(asSet(selected)) && sameSet(asSet(selected), touched(s, id)), "Historical scope coverage mismatch")
	}
	cycles(dependencies, "Topic dependency")
	cycles(successors, "Topic successor")
	for _, table := range tables {
		if table != "tasks" {
			for _, v := range rows(s, table) {
				validateClosure(s, table, obj(v, table))
			}
		}
	}
	replay(s)
	return nil
}

func replay(s Object) {
	expected := empty()
	last := ""
	for index, v := range arr(s["history"], "history") {
		event := obj(v, "history")
		fields(event, "revision action date reason evidence decision changes request", "history")
		need(integer(event["revision"]) && num(event["revision"]) == index+1, "Discontinuous revision history")
		when := date(event["date"])
		need(last == "" || when >= last, "History dates must not go backwards")
		last = when
		text(event["action"], "action")
		text(event["reason"], "reason")
		changes := arr(event["changes"], "changes")
		need(len(changes) > 0, "Empty history event")
		seen := map[string]bool{}
		for _, v := range changes {
			c := obj(v, "change")
			exact(c, "path before after", "Invalid change fields")
			path := arr(c["path"], "path")
			need(len(path) > 0, "Invalid change path")
			parts := []string{}
			for _, v := range path {
				x, ok := v.(string)
				need(ok, "Invalid change path")
				parts = append(parts, x)
			}
			// Length framing avoids ambiguous keys containing separator characters.
			key := ""
			for _, part := range parts {
				key += stringKey(part)
			}
			need(!seen[key], "Duplicate history change")
			seen[key] = true
			var parent Object
			name := parts[len(parts)-1]
			if len(parts) == 1 && parts[0] == "project" {
				parent = expected
			} else if len(parts) == 2 && contains("lines milestones tasks topics worksets", parts[0]) {
				parent = rows(expected, parts[0])
			} else if len(parts) == 3 && parts[0] == "relations" && contains("line milestone workset", parts[1]) {
				parent = obj(obj(expected["relations"], "relations")[parts[1]], parts[1])
			} else {
				need(false, "Unknown history path")
			}
			need(equal(parent[name], c["before"]), "History before value mismatch")
			if c["after"] == nil {
				delete(parent, name)
			} else {
				parent[name] = clone(c["after"])
			}
		}
	}
	need(equal(expected, metadata(s)), "Managed data differs from recorded API history; do not edit state.json manually")
}

func stringKey(s string) string { return strconv.Itoa(len(s)) + ":" + s }

// ValidateIntegrationContext and ValidateIntegrationProof expose the same pure
// rules to retained project adapters without copying business rules into Python.
func ValidateIntegrationContext(value any, topic string, source, target any) (err error) {
	defer catch(&err)
	validateContext(value, topic, source, target)
	return nil
}
func ValidateIntegrationProof(value any, topic string, pending Object) (err error) {
	defer catch(&err)
	validateProof(value, topic, pending)
	return nil
}
