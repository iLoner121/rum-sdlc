package core

import (
	"sort"
	"strings"
	"time"
)

// Query derives views from an already validated snapshot. It never reads documents.
func Query(s Object, p Object) (result Object, err error) {
	defer catch(&err)
	view := option(p, "view", "overview")
	table := str(p["table"])
	id := str(p["identifier"])
	kind := str(p["kind"])
	offset, limit := 0, 30
	if has(p, "offset") {
		offset = num(p["offset"])
	}
	if has(p, "limit") {
		limit = num(p["limit"])
	}
	need(offset >= 0 && limit >= 1 && limit <= 200, "Invalid page range")
	closed := truth(p["include_closed"])
	var statuses map[string]bool
	if p["status"] != nil {
		value, ok := p["status"].(string)
		need(ok, "Status filter must be text")
		statuses = map[string]bool{}
		for _, v := range strings.Split(value, ",") {
			need(contains("planned active review ready paused blocked closed deferred done cancelled draft integrating", v), "Invalid status filter")
			statuses[v] = true
		}
		closed = closed || statuses["closed"]
	}
	order := enum(option(p, "order_by", "id"), "id position", "Order")
	scope := enum(option(p, "scope", "scope"), "current scope history", "Iteration scope view")
	filters := map[string]any{"milestone": p["milestone_id"], "line": p["line_id"], "workset": p["workset_id"]}
	for _, v := range filters {
		if v != nil {
			need(contains("entities members orphans", view), "Relation filters require Topic views")
		}
	}
	if statuses != nil {
		need(contains("entities members orphans iteration-progress milestones", view), "Status filter not supported for this view")
		selected := "topics"
		if view == "entities" {
			selected = table
		}
		if view == "milestones" {
			selected = "milestones"
		}
		choices := map[string]string{"topics": "planned active review ready paused blocked closed", "tasks": "planned active blocked deferred done cancelled", "lines": "active paused integrating closed", "milestones": "planned active closed", "worksets": "draft active closed", "project": "active closed"}
		for status := range statuses {
			need(contains(choices[selected], status), "Status does not belong to selected entity type")
		}
	}
	switch view {
	case "iteration-progress":
		return iterationProgress(s, id, scope, offset, limit, statuses), nil
	case "line-readiness":
		return lineReadiness(s, id), nil
	case "resolve":
		return clone(resolve(s, id)).(Object), nil
	case "overview":
		rounds := []any{}
		counts := statusCounts(nil, topicPaths)
		unassigned := 0
		for _, k := range keys(rows(s, "worksets")) {
			w := entity(s, "worksets", k)
			if w["type"] != "iteration" || w["status"] == "closed" {
				continue
			}
			row := Object{"id": w["id"], "name": w["name"], "status": w["status"]}
			if truth(w["last_reviewed_on"]) {
				last, _ := time.Parse("2006-01-02", date(w["last_reviewed_on"]))
				due := last.AddDate(0, 0, 14).Format("2006-01-02")
				row["review_due_on"] = due
				if truth(p["date"]) {
					now := date(p["date"])
					need(now >= str(w["last_reviewed_on"]), "Current date precedes last review")
					row["review_due"] = now >= due
				}
			}
			rounds = append(rounds, row)
		}
		for id, v := range rows(s, "topics") {
			t := obj(v, "Topic")
			status := str(t["status"])
			counts[status] = num(counts[status]) + 1
			if status != "closed" && relation(s, "milestone", id) == nil {
				unassigned++
			}
		}
		return Object{"version": Version, "revision": s["revision"], "project": clone(s["project"]), "current_iterations": rounds, "unassigned_milestone_count": unassigned, "topic_counts": counts}, nil
	case "task-summary":
		need(has(rows(s, "topics"), id), "Missing Topic")
		tasks := tasksFor(s, id)
		count := len(tasks)
		return Object{"topic_id": id, "task_count": count, "counts": statusCounts(tasks, taskPaths), "small_plan_review": count <= 2, "large_plan_review": count > 12, "note": "Diagnostics only: assess 0-2 when preparing review, >12 before starting added activities; no quota"}, nil
	case "show":
		t := entity(s, table, id)
		result := Object{"revision": s["revision"], "entity": clone(t)}
		if table == "topics" {
			result["relations"] = resolve(s, id)
		}
		return result, nil
	case "milestones":
		view = "entities"
		table = "milestones"
		order = "position"
	}
	selected := []Object{}
	switch view {
	case "members":
		need(contains("line milestone workset", kind), "Unknown relation kind")
		parent := entity(s, parentTable(kind), id)
		if kind == "workset" && parent["type"] == "iteration" && parent["status"] == "active" && closed {
			for _, v := range arr(iterationProgress(s, id, "scope", 0, len(rows(s, "topics")), nil)["rows"], "rows") {
				selected = append(selected, obj(v, "row"))
			}
		} else {
			for _, t := range members(s, kind, id, closed) {
				topic := entity(s, "topics", t)
				row := Object{"id": t, "name": topic["name"], "status": topic["status"]}
				if kind == "workset" {
					row["position"] = relationRow(s, kind, t)["position"]
				}
				selected = append(selected, row)
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			if kind == "workset" {
				return num(selected[i]["position"]) < num(selected[j]["position"])
			}
			return str(selected[i]["id"]) < str(selected[j]["id"])
		})
		if kind == "workset" && parent["status"] == "closed" {
			need(statuses == nil && p["milestone_id"] == nil && p["line_id"] == nil && p["workset_id"] == nil, "Historical results are frozen; use their recorded dispositions instead of current filters")
			results := arr(obj(parent["closure"], "closure")["results"], "results")
			return Object{"revision": s["revision"], "basis": "historical-results", "rows": clone(page(results, offset, limit)), "total": len(results)}, nil
		}
	case "history":
		after := 0
		if has(p, "after_revision") {
			after = num(p["after_revision"])
		}
		for _, v := range arr(s["history"], "history") {
			e := obj(v, "event")
			if num(e["revision"]) <= after {
				continue
			}
			related := id == ""
			for _, v := range arr(e["changes"], "changes") {
				c := obj(v, "change")
				for _, v := range arr(c["path"], "path") {
					related = related || v == id
				}
				for _, k := range []string{"before", "after"} {
					if value, ok := c[k].(Object); ok {
						for _, f := range []string{"id", "topic_id", "parent_id"} {
							related = related || value[f] == id
						}
					}
				}
			}
			if related {
				selected = append(selected, e)
			}
		}
	case "orphans":
		for _, id := range keys(rows(s, "topics")) {
			t := entity(s, "topics", id)
			if relation(s, "milestone", id) == nil && (closed || t["status"] != "closed") {
				row := clone(t).(Object)
				row["relations"] = resolve(s, id)
				selected = append(selected, row)
			}
		}
	case "entities":
		need(contains("project milestones worksets lines topics tasks", table), "Unknown table")
		for _, id := range keys(rows(s, table)) {
			t := entity(s, table, id)
			if closed || t["status"] != "closed" {
				selected = append(selected, t)
			}
		}
	default:
		need(false, "Unknown query view")
	}
	filtered := []Object{}
	for kind, value := range filters {
		if value != nil {
			need(table == "topics" && view == "entities" || contains("members orphans", view), "Relation filters apply to Topic views")
			need(has(rows(s, parentTable(kind)), str(value)), "Missing filter parent")
		}
	}
	for _, row := range selected {
		accept := statuses == nil || statuses[str(row["status"])]
		for kind, value := range filters {
			if value != nil {
				accept = accept && equal(relation(s, kind, str(row["id"])), value)
			}
		}
		if accept {
			filtered = append(filtered, row)
		}
	}
	if order == "position" {
		need(table == "milestones" || view == "members" && kind == "workset", "Position order requires milestones or workset members")
		sort.SliceStable(filtered, func(i, j int) bool { return num(filtered[i]["position"]) < num(filtered[j]["position"]) })
	}
	return Object{"revision": s["revision"], "total": len(filtered), "offset": offset, "rows": clone(page(objects(filtered), offset, limit))}, nil
}

func option(p Object, k, fallback string) string {
	if has(p, k) {
		return str(p[k])
	}
	return fallback
}
func parentTable(kind string) string {
	return map[string]string{"milestone": "milestones", "workset": "worksets", "line": "lines"}[kind]
}
func relationRow(s Object, kind, id string) Object {
	return obj(obj(obj(s["relations"], "relations")[kind], kind)[id], "relation")
}
func objects(rows []Object) []any {
	a := make([]any, len(rows))
	for i, v := range rows {
		a[i] = v
	}
	return a
}
func page(rows []any, offset, limit int) []any {
	if offset >= len(rows) {
		return []any{}
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end]
}
func statusCounts(rows []Object, choices map[string]string) Object {
	c := Object{}
	for status := range choices {
		c[status] = 0
	}
	for _, r := range rows {
		status := str(r["status"])
		c[status] = num(c[status]) + 1
	}
	return c
}

func iterationProgress(s Object, id, scope string, offset, limit int, statuses map[string]bool) Object {
	w := entity(s, "worksets", id)
	need(w["type"] == "iteration", "Iteration progress requires an iteration")
	if w["status"] == "closed" {
		need(statuses == nil, "Historical results use recorded dispositions, not current status filters")
		rows := arr(obj(w["closure"], "closure")["results"], "results")
		if scope == "current" {
			rows = []any{}
		}
		counts := Object{"completed": 0, "cancelled": 0, "superseded": 0, "carry-forward": 0, "backlog": 0}
		for _, v := range rows {
			k := str(obj(v, "result")["disposition"])
			counts[k] = num(counts[k]) + 1
		}
		return Object{"revision": s["revision"], "basis": "historical-results", "scope": scope, "total": len(rows), "offset": offset, "rows": clone(page(rows, offset, limit)), "counts": counts}
	}
	current := members(s, "workset", id, false)
	queue := asSet(stringList(current))
	ever := touched(s, id)
	ended := []string{}
	removed := map[string]bool{}
	for t := range ever {
		topic := entity(s, "topics", t)
		if topic["status"] == "closed" && obj(obj(topic["closure"], "closure")["memberships"], "memberships")["workset_id"] == id {
			ended = append(ended, t)
		} else if !queue[t] {
			removed[t] = true
		}
	}
	sort.Slice(current, func(i, j int) bool {
		return num(relationRow(s, "workset", current[i])["position"]) < num(relationRow(s, "workset", current[j])["position"])
	})
	revision := func(t string) int {
		return num(obj(obj(entity(s, "topics", t)["closure"], "closure")["memberships"], "memberships")["revision"])
	}
	sort.Slice(ended, func(i, j int) bool {
		a, b := revision(ended[i]), revision(ended[j])
		if a == b {
			return ended[i] < ended[j]
		}
		return a < b
	})
	past := []string{}
	for _, v := range arr(w["initial_topics"], "initial_topics") {
		t := str(v)
		if removed[t] {
			past = append(past, t)
			delete(removed, t)
		}
	}
	rest := []string{}
	for t := range removed {
		rest = append(rest, t)
	}
	sort.Strings(rest)
	past = append(past, rest...)
	selected := []Object{}
	max := -1
	add := func(t, participation string, pos int) {
		topic := entity(s, "topics", t)
		if statuses == nil || statuses[str(topic["status"])] {
			selected = append(selected, Object{"id": t, "name": topic["name"], "status": topic["status"], "position": pos, "participation": participation, "current_workset_id": relation(s, "workset", t)})
		}
	}
	for _, t := range current {
		pos := num(relationRow(s, "workset", t)["position"])
		if pos > max {
			max = pos
		}
		add(t, "current", pos)
	}
	if scope != "current" {
		for i, t := range ended {
			add(t, "completed-in-round", max+1+i)
		}
		if scope == "history" {
			for i, t := range past {
				add(t, "removed", max+1+len(ended)+i)
			}
		}
	}
	return Object{"revision": s["revision"], "basis": "derived-participation", "scope": scope, "total": len(selected), "offset": offset, "rows": clone(page(objects(selected), offset, limit)), "counts": statusCounts(selected, topicPaths), "current_queue_count": len(current), "closed_in_round_count": len(ended), "ever_selected_count": len(ever)}
}

func lineReadiness(s Object, id string) Object {
	line := entity(s, "lines", id)
	ids := members(s, "line", id, true)
	blockers := []any{}
	closeBlockers := []any{}
	for _, id := range ids {
		t := entity(s, "topics", id)
		unfinished := []any{}
		for _, task := range tasksFor(s, id) {
			if contains("planned active blocked", str(task["status"])) {
				unfinished = append(unfinished, task["id"])
			}
		}
		if !contains("ready closed", str(t["status"])) || len(unfinished) > 0 {
			blockers = append(blockers, Object{"topic_id": id, "status": t["status"], "unfinished_tasks": unfinished})
		}
		if t["status"] != "closed" {
			closeBlockers = append(closeBlockers, id)
		}
	}
	return Object{"revision": s["revision"], "line_id": id, "status": line["status"], "type": line["type"], "member_count": len(ids), "integration_blockers": blockers, "close_blockers": closeBlockers, "can_integrate_structure": line["type"] == "development" && contains("active paused", str(line["status"])) && len(ids) > 0 && len(blockers) == 0, "limits": "Structural diagnostics only; actual technical exit, code and acceptance evidence require review"}
}
