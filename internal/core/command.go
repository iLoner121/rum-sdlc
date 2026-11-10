package core

import (
	"fmt"
	"rumsdlc/internal/jsonvalue"
	"sort"
	"strings"
)

// Parameters is the public command vocabulary; the application and CLI share it.
var Parameters = map[string]string{
	"project.init": "id name", "project.close": "summary",
	"extension.set": "table id namespace value", "entity.rename": "table id name correction", "closure.correct": "table id changes",
	"milestone.create": "name", "milestone.activate": "id", "milestone.reorder": "ids", "milestone.close": "id outcome summary successor",
	"workset.create": "name type", "workset.propose": "id topics", "workset.activate": "id", "workset.change": "id action topics destination", "workset.review": "id", "workset.close": "id outcome summary results goal_acceptance",
	"line.create": "id name type extensions", "line.state": "id status resume_when observations", "line.close": "id outcome summary verification delivery_record retention discard_decision successor last_release extensions",
	"topic.create": "name type slug milestone_id line_id unassigned_reason id id_reason", "topic.integration.prepare": "id context", "topic.dependencies": "id dependencies", "topic.state": "id status verification acceptance integration_proof", "topic.close": "id outcome summary verification delivery_record retention successor extensions",
	"relation.move": "kind topic_id to destination integration_context", "task.correct": "id result", "task.kind": "id kind", "task.create": "topic_id name kind", "task.state": "id status result destination",
}

func Request(action string, params Object, when, reason string, evidence, decision any) Object {
	return Object{"action": action, "params": params, "date": when, "reason": reason, "evidence": evidence, "decision": decision}
}

// Dispatch evaluates a command without mutating the caller's snapshot.
// Validation of persisted snapshots is the store's responsibility.
func Dispatch(before Object, request Object) (after Object, result any, err error) {
	defer catch(&err)
	exact(request, "action params date reason evidence decision", "Invalid command envelope")
	action := str(request["action"])
	allowed, ok := Parameters[action]
	need(ok, "Unknown action")
	when := date(request["date"])
	text(request["reason"], "Change reason")
	p := obj(request["params"], "params")
	only(p, allowed, "Unknown command parameters")
	for _, k := range []string{"evidence", "decision"} {
		if request[k] != nil {
			text(request[k], k)
		}
	}
	if action == "project.init" {
		need(before == nil, "Project exists; no implicit reinitialization")
		id := match(p["id"], `P-[a-z0-9]+(?:-[a-z0-9]+)*`, "Project ID")
		after = empty()
		after["version"] = Version
		after["revision"] = 0
		after["history"] = []any{}
		after["project"] = Object{"id": id}
		after["project"] = newEntity(after, id, p["name"], when, "active", Object{"pool_id": "POOL", "default_line_id": "mainline"})
		rows(after, "worksets")["POOL"] = newEntity(after, "POOL", "长期候选", when, "active", Object{"type": "pool", "proposed_topics": []any{}, "initial_topics": []any{}, "activation_revision": nil, "started_on": nil, "last_reviewed_on": nil})
		rows(after, "lines")["mainline"] = newEntity(after, "mainline", "项目主线", when, "active", Object{"type": "mainline", "target_line_id": nil})
		return seal(nil, after, request, id)
	}
	need(before != nil, "Initialize a Project first")
	need(obj(before["project"], "Project")["status"] == "active" || contains("closure.correct task.correct extension.set", action) || action == "entity.rename" && truth(p["correction"]), "Project is closed")
	// Python accepts nonempty factual correction text as well as true.
	history := arr(before["history"], "history")
	need(len(history) > 0, "Invalid history length")
	need(when >= str(obj(history[len(history)-1], "history")["date"]), "Change date precedes latest revision")
	after = clone(before).(Object)
	effective := clone(request).(Object)
	effective["params"] = canonicalParams(before, action, p)
	result = apply(after, effective)
	return seal(before, after, request, result)
}

func seal(before, after, request Object, result any) (Object, any, error) {
	changes := Changes(before, after)
	if len(changes) == 0 {
		return before, result, nil
	}
	revision := 1
	history := []any{}
	if before != nil {
		revision = num(before["revision"]) + 1
		history = clone(before["history"]).([]any)
	}
	after["revision"] = revision
	event := Object{"revision": revision, "request": clone(request), "changes": clone(changes)}
	for _, k := range []string{"action", "date", "reason", "evidence", "decision"} {
		event[k] = request[k]
	}
	after["history"] = append(history, event)
	return after, result, nil
}

func Changes(before, after Object) []any {
	old := empty()
	if before != nil {
		old = metadata(before)
	}
	next := metadata(after)
	changes := []any{}
	add := func(path []any, a, b any) {
		if !equal(a, b) {
			changes = append(changes, Object{"path": path, "before": a, "after": b})
		}
	}
	add([]any{"project"}, old["project"], next["project"])
	for _, table := range entityTables {
		a, b := rows(old, table), rows(next, table)
		for _, id := range sortedUnion(a, b) {
			add([]any{table, id}, a[id], b[id])
		}
	}
	for _, kind := range relationKinds {
		a := obj(obj(old["relations"], "relations")[kind], kind)
		b := obj(obj(next["relations"], "relations")[kind], kind)
		for _, id := range sortedUnion(a, b) {
			add([]any{"relations", kind, id}, a[id], b[id])
		}
	}
	return changes
}

func apply(s, r Object) any {
	a := str(r["action"])
	p := obj(r["params"], "params")
	when := str(r["date"])
	id := str(p["id"])
	switch a {
	case "extension.set":
		t := entity(s, str(p["table"]), id)
		notes(r, true, true)
		namespace := text(p["namespace"], "Extension namespace")
		extensions(Object{namespace: p["value"]})
		obj(t["extensions"], "extensions")[namespace] = clone(p["value"])
		return id
	case "entity.rename":
		t := entity(s, str(p["table"]), id)
		text(p["name"], "name")
		if t["status"] == "closed" {
			text(p["correction"], "Factual correction reason")
		}
		t["name"] = p["name"]
		return id
	case "closure.correct":
		table := enum(p["table"], "project topics lines milestones worksets", "Entity has no closure")
		t := entity(s, table, id)
		need(t["status"] == "closed", "Fact correction requires a closed entity")
		notes(r, true, true)
		changes := obj(p["changes"], "Closure fact changes")
		c := obj(t["closure"], "closure")
		need(len(changes) > 0, "Only existing mutable closure facts may be corrected")
		for k, v := range changes {
			need(contains("summary evidence verification delivery_record retention discard_decision last_release extensions successor goal_acceptance", k) && has(c, k), "Only existing mutable closure facts may be corrected")
			c[k] = clone(v)
		}
		return id
	case "task.correct":
		t := entity(s, "tasks", id)
		need(t["status"] == "done", "Only an ended Task result can be factually corrected")
		notes(r, true, true)
		t["result"] = text(p["result"], "Corrected actual activity result")
		return id
	case "topic.integration.prepare":
		t := topic(s, id)
		need(t["integration_recheck"] != nil, "No existing integration context to refresh")
		pending := obj(t["integration_recheck"], "pending")
		context := obj(p["context"], "Refreshed integration context")
		old := obj(pending["context"], "context")
		line := relation(s, "line", id)
		notes(r, true, true)
		need(context["from_line_id"] == old["from_line_id"] && context["line_id"] == line, "Refresh cannot move membership or replace its historical source")
		prepareContext(s, t, context, old["from_line_id"], line)
		return id
	case "project.close":
		for _, v := range rows(s, "topics") {
			need(obj(v, "Topic")["status"] == "closed", "Project has unfinished Topics")
		}
		for _, v := range rows(s, "worksets") {
			w := obj(v, "workset")
			need(w["type"] == "pool" || w["status"] == "closed", "Close iterations first")
		}
		for _, v := range rows(s, "milestones") {
			need(obj(v, "milestone")["status"] == "closed", "Close milestones first")
		}
		for _, v := range rows(s, "lines") {
			l := obj(v, "Line")
			need(l["type"] == "mainline" || l["status"] == "closed", "Close temporary/support Lines first")
		}
		notes(r, true, false)
		project := obj(s["project"], "Project")
		project["status"] = "closed"
		project["closure"] = closure(r, p, "completed")
		return project["id"]
	case "milestone.create":
		id = fmt.Sprint("M", sequence(rows(s, "milestones"), "M", 0))
		pos := -1
		for _, v := range rows(s, "milestones") {
			if n := num(obj(v, "milestone")["position"]); n > pos {
				pos = n
			}
		}
		rows(s, "milestones")[id] = newEntity(s, id, p["name"], when, "planned", Object{"position": pos + 1})
		return id
	case "milestone.reorder":
		selected := arr(p["ids"], "ids")
		positions := []int{}
		planned := map[string]bool{}
		for k, v := range rows(s, "milestones") {
			m := obj(v, "milestone")
			if m["status"] == "planned" {
				planned[k] = true
				positions = append(positions, num(m["position"]))
			}
		}
		need(len(selected) == len(asSet(selected)) && sameSet(asSet(selected), planned), "Reorder must include all and only planned milestones")
		sort.Ints(positions)
		for i, v := range selected {
			entity(s, "milestones", str(v))["position"] = positions[i]
		}
		return selected
	case "milestone.activate", "milestone.close":
		return applyMilestone(s, r, p)
	case "workset.create":
		need(!has(p, "type") || p["type"] == "iteration", "Project already has its singleton Pool")
		id = formatID("ITER-", sequence(rows(s, "worksets"), "ITER-", 1))
		rows(s, "worksets")[id] = newEntity(s, id, p["name"], when, "draft", Object{"type": "iteration", "proposed_topics": []any{}, "initial_topics": []any{}, "activation_revision": nil, "started_on": nil, "last_reviewed_on": nil})
		return id
	case "workset.propose", "workset.activate", "workset.change", "workset.review", "workset.close":
		return applyWorkset(s, r, p)
	case "line.create":
		slug(id)
		need(!has(rows(s, "lines"), id), "Line ID cannot be reused")
		kind := enum(p["type"], "development maintenance", "Line type")
		var target any
		if kind == "development" {
			target = obj(s["project"], "Project")["default_line_id"]
		}
		l := newEntity(s, id, p["name"], when, "active", Object{"type": kind, "target_line_id": target})
		l["extensions"] = extensionParam(p)
		rows(s, "lines")[id] = l
		return id
	case "line.state", "line.close":
		return applyLine(s, r, p)
	case "topic.create":
		kind := enum(p["type"], "feat fix spike refactor docs chore", "Topic type")
		slug(p["slug"])
		raw, e := jsonvalue.LegacyJSON([]any{obj(s["project"], "Project")["id"], s["revision"], r})
		need(e == nil, "Invalid request JSON")
		if !has(p, "id") {
			id = "T-" + strings.ReplaceAll(when, "-", "") + "-" + jsonvalue.HashBytes(raw)[:8]
			need(!has(p, "id_reason"), "ID reason requires a reserved ID")
		} else {
			match(id, TopicPattern, "Reserved Topic ID")
			need(strings.Split(id, "-")[1] == strings.ReplaceAll(when, "-", ""), "Reserved ID date must match creation")
			text(p["id_reason"], "Reason for externally reserved Topic identity")
		}
		need(!has(rows(s, "topics"), id), "Topic ID cannot be reused")
		line := obj(s["project"], "Project")["default_line_id"]
		if has(p, "line_id") {
			line = p["line_id"]
		}
		need(entity(s, "lines", str(line))["status"] == "active", "New Topic needs active receiving Line")
		if p["milestone_id"] != nil {
			open(entity(s, "milestones", str(p["milestone_id"])))
		} else {
			text(p["unassigned_reason"], "Explicit reason for no milestone")
		}
		rows(s, "topics")[id] = newEntity(s, id, p["name"], when, "planned", Object{"type": kind, "slug": p["slug"], "dependencies": []any{}, "integration_recheck": nil})
		setRelation(s, "line", id, line)
		setRelation(s, "workset", id, obj(s["project"], "Project")["pool_id"])
		setRelation(s, "milestone", id, p["milestone_id"])
		return id
	case "relation.move":
		move(s, r, str(p["kind"]), str(p["topic_id"]), p["to"], p["destination"], p["integration_context"])
		return p["topic_id"]
	case "topic.dependencies", "topic.state", "topic.close":
		return applyTopic(s, r, p)
	case "task.create", "task.kind", "task.state":
		return applyTask(s, r, p)
	}
	need(false, "Unknown action")
	return nil
}

func extensionParam(p Object) any {
	if has(p, "extensions") {
		return clone(p["extensions"])
	}
	return Object{}
}
func applyMilestone(s, r, p Object) any {
	id := str(p["id"])
	t := entity(s, "milestones", id)
	open(t)
	if r["action"] == "milestone.activate" {
		if t["status"] == "active" {
			return id
		}
		notes(r, true, false)
		for k, v := range rows(s, "milestones") {
			m := obj(v, "milestone")
			need(k == id || m["status"] == "closed" || num(m["position"]) >= num(t["position"]), "Earlier milestones must end before activation")
		}
		t["status"] = "active"
	} else {
		outcome := enum(p["outcome"], "completed cancelled superseded", "Milestone outcome")
		notes(r, true, false)
		need(len(members(s, "milestone", id, false)) == 0, "Reassign unfinished milestone Topics first")
		if outcome == "completed" {
			need(t["status"] == "active", "Completed milestone must have been active")
		}
		c := closure(r, p, outcome)
		c["successor"] = p["successor"]
		if outcome == "superseded" {
			need(c["successor"] != id && entity(s, "milestones", str(c["successor"]))["status"] != "closed", "Invalid milestone successor")
		} else {
			need(c["successor"] == nil, "Successor only applies to supersession")
		}
		t["status"] = "closed"
		t["closure"] = c
	}
	return id
}

func applyWorkset(s, r, p Object) any {
	id := str(p["id"])
	w := entity(s, "worksets", id)
	need(w["type"] == "iteration", "Pool does not use iteration lifecycle")
	open(w)
	when := str(r["date"])
	pool := obj(s["project"], "Project")["pool_id"]
	switch r["action"] {
	case "workset.propose":
		need(w["status"] == "draft", "Proposals belong to a draft")
		selected := ids(p["topics"], TopicPattern, "Topic IDs")
		for _, v := range selected {
			topic(s, str(v))
		}
		w["proposed_topics"] = clone(selected)
	case "workset.activate":
		selected := arr(w["proposed_topics"], "proposed_topics")
		need(w["status"] == "draft" && len(selected) > 0, "Activation needs a nonempty draft selection")
		notes(r, true, true)
		for _, v := range selected {
			topic(s, str(v))
			need(relation(s, "workset", str(v)) == pool, "Selected Topic is not in Pool")
			setRelation(s, "workset", str(v), id)
		}
		w["status"] = "active"
		w["initial_topics"] = clone(selected)
		w["proposed_topics"] = []any{}
		w["activation_revision"] = num(s["revision"]) + 1
		w["started_on"] = when
		w["last_reviewed_on"] = when
	case "workset.change":
		need(w["status"] == "active", "Runtime scope changes require active iteration")
		notes(r, true, true)
		selected := ids(p["topics"], TopicPattern, "Topic IDs")
		action := enum(p["action"], "add remove reorder", "Scope action")
		current := asSet(stringList(members(s, "workset", id, false)))
		if action == "reorder" {
			need(sameSet(asSet(selected), current), "Reorder must contain exactly the current queue")
			for i, v := range selected {
				obj(obj(obj(s["relations"], "relations")["workset"], "workset")[str(v)], "relation")["position"] = i
			}
		} else {
			need(len(selected) > 0, "Select Topics")
			for _, v := range selected {
				t := str(v)
				topic(s, t)
				if action == "add" {
					need(relation(s, "workset", t) == pool, "Added Topic must be in Pool")
					setRelation(s, "workset", t, id)
				} else {
					need(current[t], "Topic is not a current member")
					text(p["destination"], "Removal destination/resumption condition")
					move(s, r, "workset", t, pool, p["destination"], nil)
				}
			}
		}
	case "workset.review":
		need(w["status"] == "active", "Review requires active iteration")
		notes(r, true, true)
		w["last_reviewed_on"] = when
	case "workset.close":
		notes(r, true, false)
		outcome := enum(p["outcome"], "completed replanned cancelled", "Iteration outcome")
		if w["status"] == "draft" {
			need(outcome == "cancelled", "Unactivated draft can only be cancelled")
		}
		results := arr(p["results"], "Results")
		selected := []any{}
		for _, v := range results {
			x := obj(v, "Iteration result")
			fields(x, "topic disposition note evidence", "Iteration result")
			text(x["note"], "Result note")
			text(x["evidence"], "Result evidence")
			selected = append(selected, x["topic"])
			t := entity(s, "topics", str(x["topic"]))
			if t["status"] == "closed" {
				expected := map[string]string{"delivered": "completed", "research-complete": "completed", "cancelled": "cancelled", "superseded": "superseded"}
				need(x["disposition"] == expected[str(obj(t["closure"], "closure")["outcome"])], "Result does not match actual Topic outcome")
			} else {
				enum(x["disposition"], "carry-forward backlog", "Unfinished Topic disposition")
			}
		}
		need(len(selected) == len(asSet(selected)) && sameSet(asSet(selected), touched(s, id)), "Results must cover all actually selected Topics, including removed members")
		var acceptance any
		if outcome == "completed" {
			acceptance = p["goal_acceptance"]
			if !truth(acceptance) {
				acceptance = r["decision"]
			}
			text(acceptance, "Actual acceptance of achieved iteration goal, including approved scope adjustments")
		}
		for _, t := range members(s, "workset", id, false) {
			setRelation(s, "workset", t, pool)
		}
		c := closure(r, p, outcome)
		c["results"] = clone(results)
		c["goal_acceptance"] = acceptance
		w["status"] = "closed"
		w["closure"] = c
	}
	return id
}

func applyLine(s, r, p Object) any {
	id := str(p["id"])
	line := entity(s, "lines", id)
	open(line)
	need(line["type"] != "mainline", "Project mainline has no temporary Line lifecycle")
	if r["action"] == "line.state" {
		obs := p["observations"]
		if !has(p, "observations") {
			obs = Object{}
		}
		extensions(obs)
		next := enum(p["status"], "active paused integrating", "Line state")
		if next == line["status"] {
			return id
		}
		need(contains(linePaths[str(line["status"])], next), "Illegal Line transition")
		if next == "paused" {
			text(p["resume_when"], "Recovery condition")
		} else {
			need(!truth(p["resume_when"]), "Recovery condition only applies when pausing")
		}
		if next == "integrating" {
			need(line["type"] == "development", "Maintenance cannot integrate wholesale")
			selected := members(s, "line", id, true)
			need(len(selected) > 0, "Empty Line cannot integrate")
			for _, t := range selected {
				need(contains("ready closed", str(entity(s, "topics", t)["status"])) && !unfinished(s, t), "Line members are not ready/closed or have unfinished Tasks")
			}
		}
		line["status"] = next
	} else {
		need(len(members(s, "line", id, false)) == 0, "Close or transfer unfinished Line members first")
		outcomes := "integrated cancelled superseded"
		if line["type"] == "maintenance" {
			outcomes = "retired"
		}
		outcome := enum(p["outcome"], outcomes, "Line outcome")
		c := closure(r, p, outcome)
		c["verification"] = text(p["verification"], "Line verification")
		for _, k := range []string{"delivery_record", "retention", "discard_decision", "successor", "last_release"} {
			c[k] = p[k]
		}
		c["extensions"] = extensionParam(p)
		if outcome == "integrated" {
			need(line["status"] == "integrating", "Integrated closure requires integrating")
		} else {
			notes(r, true, false)
			if outcome == "superseded" {
				next := entity(s, "lines", str(c["successor"]))
				need(c["successor"] != id && next["type"] == "development" && next["status"] != "closed", "Invalid Line successor")
			}
		}
		line["status"] = "closed"
		line["closure"] = c
	}
	return id
}

func applyTopic(s, r, p Object) any {
	id := str(p["id"])
	t := topic(s, id)
	switch r["action"] {
	case "topic.dependencies":
		ids(p["dependencies"], TopicPattern, "Topic IDs")
		t["dependencies"] = clone(p["dependencies"])
	case "topic.state":
		next := str(p["status"])
		need(next == t["status"] || contains(topicPaths[str(t["status"])], next), "Illegal Topic transition")
		if !contains("paused blocked", next) {
			op := "execute"
			if contains("review ready", next) {
				op = "converge"
			}
			lineGuard(s, t, op, "process")
		}
		if contains("review ready", next) {
			need(!unfinished(s, id), "Resolve planned/active/blocked Tasks first")
			if next == "review" {
				text(p["verification"], "Actual self-verification and acceptance mapping")
			}
			if next == "ready" {
				text(p["acceptance"], "User acceptance reference")
			}
			if t["integration_recheck"] != nil {
				pending := obj(t["integration_recheck"], "pending")
				if boolean(pending["required"]) || next == "review" {
					need(next == "review", "Retargeted Topic must return through review")
					validateProof(p["integration_proof"], id, pending)
					proof := obj(p["integration_proof"], "proof")
					need(proof["verified_on"] == r["date"], "Verification date must match this review")
					pending["required"] = false
					pending["evidence"] = p["verification"]
					pending["proof"] = clone(proof)
				}
			}
		}
		t["status"] = next
	case "topic.close":
		lineGuard(s, t, "close", "")
		need(!unfinished(s, id), "Resolve planned/active/blocked Tasks first")
		outcome := enum(p["outcome"], "delivered research-complete cancelled superseded", "Topic outcome")
		text(p["verification"], "Actual verification")
		if contains("delivered research-complete", outcome) {
			need(t["status"] == "ready", "Delivery requires accepted ready Topic")
			text(p["delivery_record"], "Delivery record")
			need(t["integration_recheck"] == nil || !boolean(obj(t["integration_recheck"], "pending")["required"]), "New baseline needs verification")
			need(outcome != "research-complete" || t["type"] == "spike", "Research completion requires spike")
		} else {
			notes(r, true, false)
			text(p["retention"], "Retained work or explicit discard decision")
		}
		snapshot := resolve(s, id)
		line := entity(s, "lines", str(snapshot["line_id"]))
		c := closure(r, p, outcome)
		c["verification"] = p["verification"]
		c["memberships"] = snapshot
		c["integration_target"] = line["id"]
		c["delivery_scope"] = deliveryScope(str(line["type"]))
		for _, k := range []string{"delivery_record", "retention", "successor"} {
			c[k] = p[k]
		}
		c["extensions"] = extensionParam(p)
		if !contains("delivered research-complete", outcome) {
			need(c["delivery_record"] == nil, "Non-delivery cannot claim integration")
		}
		if outcome != "superseded" {
			need(c["successor"] == nil, "Successor only applies to supersession")
		}
		t["status"] = "closed"
		t["closure"] = c
		setRelation(s, "workset", id, nil)
	}
	return id
}

func applyTask(s, r, p Object) any {
	id := str(p["id"])
	if r["action"] == "task.create" {
		t := topic(s, str(p["topic_id"]))
		kind := match(p["kind"], `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`, "Task kind")
		if t["status"] != "planned" {
			need(taskAllowed(t, kind), "Task kind not allowed in this Topic state")
		}
		op := "execute"
		if contains("review ready", str(t["status"])) {
			op = "converge"
		}
		lineGuard(s, t, op, kind)
		prefix := str(t["id"]) + "/t"
		id = formatID(prefix, sequence(rows(s, "tasks"), prefix, 1))
		rows(s, "tasks")[id] = newEntity(s, id, p["name"], str(r["date"]), "planned", Object{"topic_id": t["id"], "kind": kind, "result": nil, "destination": nil})
		return id
	}
	task := entity(s, "tasks", id)
	t := topic(s, str(task["topic_id"]))
	if r["action"] == "task.kind" {
		kind := match(p["kind"], `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`, "Task kind")
		need(contains("planned blocked", str(task["status"])) || task["kind"] == "legacy-unknown", "Classify planned/blocked or unknown legacy activity")
		need(kind != "legacy-unknown", "Provide actual activity kind")
		task["kind"] = kind
		return id
	}
	next := str(p["status"])
	if next == task["status"] {
		return id
	}
	need(contains(taskPaths[str(task["status"])], next), "Illegal Task transition")
	if contains("active done", next) {
		need(task["kind"] != "legacy-unknown", "Classify legacy Task through task.kind before execution")
		need(taskAllowed(t, str(task["kind"])), "Task execution requires active Topic or convergence activity")
		op := "execute"
		if contains("review ready", str(t["status"])) {
			op = "converge"
		}
		lineGuard(s, t, op, str(task["kind"]))
	}
	if next == "done" {
		task["result"] = text(p["result"], "Activity result, including failures/limits")
	}
	if next == "deferred" {
		task["destination"] = text(p["destination"], "Deferred destination")
	}
	task["status"] = next
	return id
}

func move(s, r Object, kind, id string, dest, destination, context any) {
	t := topic(s, id)
	need(contains("line milestone workset", kind), "Unknown relation kind")
	old := relation(s, kind, id)
	if equal(old, dest) {
		return
	}
	notes(r, true, true)
	switch kind {
	case "milestone":
		if dest != nil {
			open(entity(s, "milestones", str(dest)))
		}
	case "workset":
		need(entity(s, "worksets", str(dest))["status"] == "active", "Assign only to Pool or an active iteration")
		if entity(s, "worksets", str(old))["type"] == "iteration" {
			text(destination, "Removal destination/resumption condition")
		}
	case "line":
		need(entity(s, "lines", str(old))["status"] != "integrating", "Return source Line active before retargeting")
		prepareContext(s, t, context, old, dest)
	}
	setRelation(s, kind, id, dest)
}

func prepareContext(s, t Object, context, source, target any) {
	need(entity(s, "lines", str(target))["status"] == "active", "Receiving Line must be active")
	idle := t["status"] != "active"
	for _, task := range tasksFor(s, str(t["id"])) {
		idle = idle && task["status"] != "active"
	}
	need(idle, "Pause/block executing Topic and Tasks before preparing integration")
	validateContext(context, str(t["id"]), source, target)
	if contains("review ready", str(t["status"])) {
		t["status"] = "paused"
	}
	t["integration_recheck"] = Object{"required": true, "line_id": target, "evidence": nil, "context": clone(context), "proof": nil}
}
