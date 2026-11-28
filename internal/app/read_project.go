package app

import (
	"fmt"
	"rumsdlc/internal/core"
	"sort"
	"strconv"
	"strings"
)

type TopicFilter struct {
	Scope, WorksetID, MilestoneID, LineID, Status, Query, Basis, Revision string
	Priority                                                              string
	Offset, Limit                                                         int
}

func (f TopicFilter) page(rows []any) (Object, error) {
	limit := f.Limit
	if limit == 0 {
		limit = 30
	}
	if f.Offset < 0 || limit < 1 || limit > 200 {
		return nil, NewReadError("invalid", "Invalid page range")
	}
	start := f.Offset
	if start > len(rows) {
		start = len(rows)
	}
	remaining := len(rows) - start
	take := limit
	if take > remaining {
		take = remaining
	}
	end := start + take
	return Object{"rows": rows[start:end], "total": len(rows), "offset": f.Offset, "limit": limit}, nil
}
func planningRows(state Object, table string) ([]any, error) {
	rows, e := entityRows(state, table)
	if e != nil {
		return nil, e
	}
	result := []any{}
	for _, v := range rows {
		row := v.(Object)
		if table == "worksets" && row["type"] != "iteration" {
			continue
		}
		kind := "milestone"
		if table == "worksets" {
			kind = "workset"
		}
		members, _, e := collect(state, Object{"view": "members", "kind": kind, "identifier": row["id"], "include_closed": true})
		if e != nil {
			return nil, e
		}
		row["member_count"] = len(members)
		if table == "worksets" {
			proposed, _ := row["proposed_topics"].([]any)
			row["proposed_count"] = len(proposed)
			if row["status"] == "draft" {
				row["member_count"] = len(proposed)
				row["progress"] = Object{"revision": state["revision"], "basis": "draft-proposal", "scope": "scope", "total": len(proposed)}
				result = append(result, row)
				continue
			}
			_, progress, e := collect(state, Object{"view": "iteration-progress", "identifier": row["id"], "scope": "scope"})
			if e != nil {
				return nil, e
			}
			delete(progress, "rows")
			delete(progress, "offset")
			row["progress"] = progress
		}
		result = append(result, row)
	}
	if table == "milestones" {
		sort.SliceStable(result, func(i, j int) bool {
			return integer(result[i].(Object)["position"]) < integer(result[j].(Object)["position"])
		})
	}
	return result, nil
}
func poolEntity(state Object) Object {
	for _, v := range getObject(state, "worksets") {
		row, ok := v.(Object)
		if ok && row["type"] == "pool" {
			return copyObject(row)
		}
	}
	return nil
}
func (r *Reader) ReadOverview(pid, revision string) (View, error) {
	_, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	overview, e := core.Query(state, Object{"view": "overview"})
	if e != nil {
		return View{}, e
	}
	ms, e := planningRows(state, "milestones")
	if e != nil {
		return View{}, e
	}
	its, e := planningRows(state, "worksets")
	if e != nil {
		return View{}, e
	}
	active := func(rows []any) []any {
		out := []any{}
		for _, v := range rows {
			if v.(Object)["status"] == "active" {
				out = append(out, v)
			}
		}
		return out
	}
	pool := poolEntity(state)
	if pool != nil {
		members, _, e := collect(state, Object{"view": "members", "kind": "workset", "identifier": pool["id"]})
		if e != nil {
			return View{}, e
		}
		pool["count"] = len(members)
	}
	return NewView(pid, state["revision"], "current", Object{"project": overview["project"], "topic_counts": overview["topic_counts"], "milestones": active(ms), "iterations": active(its), "pool": pool}), nil
}
func (r *Reader) ReadPlanning(pid, table, revision string, f TopicFilter) (View, error) {
	_, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	if table != "milestones" && table != "worksets" {
		return View{}, NewReadError("invalid", "Unknown planning collection")
	}
	rows, e := planningRows(state, table)
	if e != nil {
		return View{}, e
	}
	data, e := f.page(rows)
	return NewView(pid, state["revision"], "current", data), e
}
func (r *Reader) ReadTopics(pid string, f TopicFilter) (View, error) {
	_, state, e := r.snapshot(pid, f.Revision)
	if e != nil {
		return View{}, e
	}
	for table, id := range map[string]string{"milestones": f.MilestoneID, "lines": f.LineID} {
		if id != "" && !has(getObject(state, table), id) {
			return View{}, NewReadError("invalid", "Filter entity does not exist")
		}
	}
	scope := f.Scope
	if scope == "" {
		scope = "all"
	}
	basis := "current"
	q := Object{"view": "entities", "table": "topics", "include_closed": true}
	switch scope {
	case "all":
	case "pool":
		pool := poolEntity(state)
		if pool == nil {
			return View{}, NewReadError("identity-invalid", "Project Pool is missing")
		}
		q = Object{"view": "members", "kind": "workset", "identifier": pool["id"], "include_closed": true}
	case "iteration":
		if f.WorksetID == "" {
			return View{}, NewReadError("invalid", "Iteration is required")
		}
		b := f.Basis
		if b == "" {
			b = "scope"
		}
		q = Object{"view": "iteration-progress", "identifier": f.WorksetID, "scope": b}
	default:
		return View{}, NewReadError("invalid", "Unknown Topic scope")
	}
	var rows []any
	var result Object
	workset, _ := getObject(state, "worksets")[f.WorksetID].(Object)
	if scope == "iteration" && workset != nil && workset["type"] == "iteration" && workset["status"] == "draft" {
		if f.Basis != "" && !oneOf(f.Basis, "scope current history") {
			return View{}, NewReadError("invalid", "Invalid iteration basis")
		}
		rows = []any{}
		for _, id := range workset["proposed_topics"].([]any) {
			rows = append(rows, Object{"id": id})
		}
		result = Object{"basis": "draft-proposal"}
	} else {
		rows, result, e = collect(state, q)
		if e != nil {
			return View{}, NewReadError("invalid", "Invalid Topic scope or iteration")
		}
	}
	if b := str(result["basis"]); b != "" {
		basis = b
	}
	if basis == "historical-results" && (f.Status != "" || f.LineID != "" || f.MilestoneID != "" || f.Priority != "") {
		return View{}, NewReadError("invalid", "Historical results cannot use current-state filters")
	}
	if f.Priority != "" && !core.ValidTopicPriorityFilter(f.Priority) {
		return View{}, NewReadError("invalid", "Invalid Topic priority filter")
	}
	if f.Status != "" {
		for _, status := range strings.Split(f.Status, ",") {
			if !oneOf(status, "planned active review ready paused blocked closed") {
				return View{}, NewReadError("invalid", "Unknown Topic status")
			}
		}
	}
	selected := []any{}
	numbers := core.TopicNumbers(state)
	for _, v := range rows {
		source := v.(Object)
		id := str(source["id"])
		if id == "" {
			id = str(source["topic"])
		}
		topic, ok := getObject(state, "topics")[id].(Object)
		if !ok {
			continue
		}
		priority := core.TopicPriority(topic)
		if !core.MatchesTopicPriority(priority, f.Priority) {
			continue
		}
		row := copyObject(topic)
		if basis != "historical-results" && priority != "" {
			row["priority"] = priority
		}
		row["short_id"] = numbers[id]
		rel, e := core.ResolveRelations(state, id)
		if e != nil {
			return View{}, e
		}
		row["relations"] = rel
		if scope != "all" {
			for k, value := range source {
				row[k] = value
			}
			row["id"] = id
		}
		if f.Status != "" && !strings.Contains(","+f.Status+",", ","+str(row["status"])+",") {
			continue
		}
		if f.MilestoneID != "" && fmt.Sprint(rel["milestone_id"]) != f.MilestoneID {
			continue
		}
		if f.LineID != "" && fmt.Sprint(rel["line_id"]) != f.LineID {
			continue
		}
		query := strings.ToLower(strings.TrimSpace(f.Query))
		if query != "" && !strings.Contains(strings.ToLower(str(row["name"])+" "+str(row["short_id"])+" "+id), query) {
			continue
		}
		selected = append(selected, row)
	}
	if scope == "all" {
		sort.SliceStable(selected, func(i, j int) bool {
			a, _ := strconv.Atoi(strings.TrimPrefix(str(selected[i].(Object)["short_id"]), "T"))
			b, _ := strconv.Atoi(strings.TrimPrefix(str(selected[j].(Object)["short_id"]), "T"))
			return a < b
		})
	}
	data, e := f.page(selected)
	if e != nil {
		return View{}, e
	}
	data["scope"] = scope
	data["scope_total"] = len(rows)
	filters := Object{}
	for table, key := range map[string]string{"milestones": "milestones", "lines": "lines", "worksets": "iterations"} {
		values := []any{}
		entities, e := entityRows(state, table)
		if e != nil {
			return View{}, e
		}
		for _, v := range entities {
			row := v.(Object)
			if table == "worksets" && row["type"] != "iteration" {
				continue
			}
			values = append(values, Object{"id": row["id"], "name": row["name"], "status": row["status"]})
		}
		filters[key] = values
	}
	data["filters"] = filters
	if scope == "pool" {
		data["pool"] = poolEntity(state)
	}
	if result["counts"] != nil {
		data["counts"] = result["counts"]
	}
	return NewView(pid, state["revision"], basis, data), nil
}
func (r *Reader) ReadTopic(pid, tid, revision string) (View, error) {
	s, state, e := r.snapshot(pid, revision)
	if e != nil {
		return View{}, e
	}
	id, e := core.CanonicalReference(state, tid)
	if e != nil {
		return View{}, NewReadError("missing", "Topic does not exist")
	}
	shown, e := core.Query(state, Object{"view": "show", "table": "topics", "identifier": id})
	if e != nil {
		return View{}, NewReadError("missing", "Topic does not exist")
	}
	tasks, e := entityRows(state, "tasks")
	if e != nil {
		return View{}, e
	}
	selected := []any{}
	for _, v := range tasks {
		if v.(Object)["topic_id"] == id {
			selected = append(selected, v)
		}
	}
	history, _, e := collect(state, Object{"view": "history", "identifier": id})
	if e != nil {
		return View{}, e
	}
	documents := []any{}
	for _, field := range []string{"context", "prd", "design", "acceptance"} {
		d, de := documentInfo(s, "topics", id, field)
		if de == nil {
			delete(d, "path")
			documents = append(documents, d)
		}
	}
	return NewView(pid, state["revision"], "current", Object{"topic": shown["entity"], "relations": shown["relations"], "tasks": selected, "history": history, "documents": documents}), nil
}
