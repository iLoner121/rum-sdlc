// Package core owns workflow rules and history. It has no filesystem, network,
// CLI or Git dependencies. JSON objects preserve the existing extensible format.
package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"rumsdlc/internal/jsonvalue"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Object = jsonvalue.Object

const Version = "4.1.2"
const TopicPattern = `T-[0-9]{8}-[0-9a-f]{8}`
const SlugPattern = `[a-z0-9]+(?:-[a-z0-9]+)*`

var tables = []string{"project", "milestones", "worksets", "lines", "topics", "tasks"}
var entityTables = []string{"lines", "milestones", "tasks", "topics", "worksets"}
var relationKinds = []string{"line", "milestone", "workset"}
var topicPaths = map[string]string{"planned": "active paused blocked", "active": "review paused blocked", "review": "ready active paused blocked", "ready": "active paused blocked", "paused": "active blocked", "blocked": "active paused", "closed": ""}
var taskPaths = map[string]string{"planned": "active blocked deferred cancelled", "active": "done blocked deferred cancelled", "blocked": "planned active deferred cancelled", "deferred": "planned cancelled", "done": "active", "cancelled": ""}
var linePaths = map[string]string{"active": "paused integrating", "paused": "active integrating", "integrating": "active paused", "closed": ""}

// ruleError is used only inside a single pure rule evaluation; public entry
// points turn it into an error. Programming panics are deliberately not hidden.
type ruleError struct{ message string }

func (e ruleError) Error() string { return e.message }
func catch(err *error) {
	if p := recover(); p != nil {
		if e, ok := p.(ruleError); ok {
			*err = e
		} else {
			panic(p)
		}
	}
}
func need(ok bool, message string) {
	if !ok {
		panic(ruleError{message})
	}
}
func obj(v any, label string) Object {
	x, ok := v.(map[string]any)
	need(ok, label+" must be a JSON object")
	return x
}
func arr(v any, label string) []any {
	x, ok := v.([]any)
	need(ok, label+" must be an array")
	return x
}
func str(v any) string { x, _ := v.(string); return x }
func text(v any, label string) string {
	x, ok := v.(string)
	need(ok && strings.TrimSpace(x) != "", label+" must be nonempty text")
	return x
}
func num(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case json.Number:
		n, e := strconv.Atoi(string(x))
		need(e == nil, "Expected integer")
		return n
	default:
		need(false, "Expected integer")
		return 0
	}
}
func integer(v any) bool {
	switch x := v.(type) {
	case int:
		return true
	case json.Number:
		_, e := strconv.Atoi(string(x))
		return e == nil
	}
	return false
}
func boolean(v any) bool { x, _ := v.(bool); return x }
func match(v any, pattern, label string) string {
	x, ok := v.(string)
	need(ok && regexp.MustCompile("^(?:"+pattern+")$").MatchString(x), "Invalid "+label)
	return x
}
func date(v any) string {
	x := match(v, `[0-9]{4}-[0-9]{2}-[0-9]{2}`, "date; use YYYY-MM-DD in the user timezone")
	_, err := time.Parse("2006-01-02", x)
	need(err == nil && !strings.HasPrefix(x, "0000-"), "Invalid calendar date: "+x)
	return x
}
func slug(v any) string {
	x := match(v, SlugPattern, "slug")
	need(len(x) <= 64, "Slug exceeds 64 characters")
	return x
}
func contains(words, s string) bool {
	for _, w := range strings.Fields(words) {
		if s == w {
			return true
		}
	}
	return false
}
func enum(v any, words, label string) string {
	x, ok := v.(string)
	need(ok && contains(words, x), "Invalid "+label)
	return x
}
func keys(o Object) []string {
	r := make([]string, 0, len(o))
	for k := range o {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func has(o Object, k string) bool { _, ok := o[k]; return ok }
func fields(o Object, names, label string) {
	missing := []string{}
	for _, k := range strings.Fields(names) {
		if !has(o, k) {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	need(len(missing) == 0, label+" missing fields: "+strings.Join(missing, ", "))
}
func only(o Object, names, message string) {
	for k := range o {
		need(contains(names, k), message)
	}
}
func exact(o Object, names, message string) {
	need(len(o) == len(strings.Fields(names)), message)
	only(o, names, message)
}
func ids(v any, pattern, label string) []any {
	a := arr(v, label)
	seen := map[string]bool{}
	for _, v := range a {
		s := match(v, pattern, label)
		need(!seen[s], "Duplicate "+label)
		seen[s] = true
	}
	return a
}
func extensions(v any) {
	o := obj(v, "External extensions")
	for k, v := range o {
		match(k, `[a-z][a-z0-9]*(?:-[a-z0-9]+)*`, "Extension namespace")
		obj(v, "Extension payload")
	}
	_, e := jsonvalue.Canonical(v)
	need(e == nil, "Extensions must be finite JSON data")
}
func clone(v any) any     { return jsonvalue.Clone(v) }
func equal(a, b any) bool { return jsonvalue.Equal(a, b) }
func rows(s Object, table string) Object {
	if table == "project" {
		p := obj(s["project"], "Project")
		return Object{str(p["id"]): p}
	}
	return obj(s[table], table)
}
func entity(s Object, table, id string) Object {
	need(contains(strings.Join(tables, " "), table), "Unknown entity table")
	r := rows(s, table)
	need(has(r, id), "Missing "+table+" entity: "+id)
	return obj(r[id], table)
}
func open(t Object) {
	need(t["status"] != "closed", "Closed entity cannot resume or change membership")
}
func topic(s Object, id string) Object { t := entity(s, "topics", id); open(t); return t }
func relation(s Object, kind, id string) any {
	r := obj(obj(s["relations"], "relations")[kind], kind)
	if r[id] == nil {
		return nil
	}
	return obj(r[id], kind)["parent_id"]
}
func resolve(s Object, id string) Object {
	entity(s, "topics", id)
	return Object{"topic_id": id, "milestone_id": relation(s, "milestone", id), "workset_id": relation(s, "workset", id), "line_id": relation(s, "line", id), "revision": s["revision"]}
}
func tasksFor(s Object, id string) []Object {
	result := []Object{}
	for _, k := range keys(rows(s, "tasks")) {
		t := entity(s, "tasks", k)
		if t["topic_id"] == id {
			result = append(result, t)
		}
	}
	return result
}
func members(s Object, kind, id string, closed bool) []string {
	result := []string{}
	rs := obj(obj(s["relations"], "relations")[kind], kind)
	for _, t := range keys(rs) {
		r := obj(rs[t], kind)
		if r["parent_id"] == id && (closed || entity(s, "topics", t)["status"] != "closed") {
			result = append(result, t)
		}
	}
	return result
}
func unfinished(s Object, id string) bool {
	for _, t := range tasksFor(s, id) {
		if contains("planned active blocked", str(t["status"])) {
			return true
		}
	}
	return false
}
func taskAllowed(t Object, kind string) bool {
	return t["status"] == "active" || contains("review ready", str(t["status"])) && contains("verify process", kind)
}
func lineGuard(s, t Object, operation, kind string) {
	line := entity(s, "lines", str(relation(s, "line", str(t["id"]))))
	if line["status"] == "active" {
		return
	}
	need(line["status"] == "integrating" && contains("review ready", str(t["status"])) && contains("converge close", operation) && (operation == "close" || contains("verify process", kind)), "Line does not permit this activity; return/resume Line active first")
}
func empty() Object {
	return Object{"project": nil, "milestones": Object{}, "worksets": Object{}, "lines": Object{}, "topics": Object{}, "tasks": Object{}, "relations": Object{"milestone": Object{}, "workset": Object{}, "line": Object{}}}
}
func metadata(s Object) Object {
	r := empty()
	for k := range r {
		r[k] = s[k]
	}
	return r
}
func newEntity(s Object, id string, name any, when, status string, extra Object) Object {
	text(name, "name")
	r := Object{"version": s["version"], "project_id": obj(s["project"], "project")["id"], "id": id, "name": name, "created_on": when, "status": status, "closure": nil, "extensions": Object{}}
	for k, v := range extra {
		r[k] = v
	}
	return r
}
func sequence(rs Object, prefix string, start int) int {
	n := start - 1
	for id := range rs {
		if strings.HasPrefix(id, prefix) {
			v, e := strconv.Atoi(strings.TrimPrefix(id, prefix))
			need(e == nil, "Invalid sequential ID")
			if v > n {
				n = v
			}
		}
	}
	return n + 1
}
func notes(r Object, decision, evidence bool) {
	if decision {
		text(r["decision"], "Existing scope/acceptance decision reference")
	}
	if evidence {
		text(r["evidence"], "Actual evidence reference")
	}
}
func closure(r, p Object, outcome string) Object {
	notes(r, false, true)
	text(p["summary"], "Closure summary")
	return Object{"outcome": outcome, "closed_on": r["date"], "summary": p["summary"], "evidence": r["evidence"]}
}
func setRelation(s Object, kind, id string, parent any) {
	rs := obj(obj(s["relations"], "relations")[kind], kind)
	if parent == nil {
		delete(rs, id)
		return
	}
	r := Object{"version": s["version"], "project_id": obj(s["project"], "project")["id"], "topic_id": id, "parent_id": parent}
	if kind == "workset" {
		pos := -1
		for _, v := range rs {
			x := obj(v, kind)
			if x["parent_id"] == parent && num(x["position"]) > pos {
				pos = num(x["position"])
			}
		}
		r["position"] = pos + 1
		if rs[id] != nil {
			old := obj(rs[id], kind)
			if old["parent_id"] == parent {
				r["position"] = old["position"]
			}
		}
	}
	rs[id] = r
}
func touched(s Object, id string) map[string]bool {
	w := entity(s, "worksets", id)
	r := map[string]bool{}
	for _, v := range arr(w["initial_topics"], "initial_topics") {
		r[str(v)] = true
	}
	if w["legacy"] != nil {
		if a := obj(w["legacy"], "legacy")["imported_touched_topics"]; a != nil {
			for _, v := range arr(a, "imported_touched_topics") {
				r[str(v)] = true
			}
		}
	}
	if w["activation_revision"] == nil {
		return r
	}
	for _, v := range arr(s["history"], "history") {
		e := obj(v, "event")
		if num(e["revision"]) < num(w["activation_revision"]) {
			continue
		}
		for _, v := range arr(e["changes"], "changes") {
			c := obj(v, "change")
			p := arr(c["path"], "path")
			if len(p) >= 2 && p[0] == "relations" && p[1] == "workset" {
				for _, k := range []string{"before", "after"} {
					if c[k] != nil {
						x := obj(c[k], "relation")
						if x["parent_id"] == id {
							r[str(x["topic_id"])] = true
						}
					}
				}
			}
		}
	}
	return r
}
func asSet(a []any) map[string]bool {
	r := map[string]bool{}
	for _, v := range a {
		r[str(v)] = true
	}
	return r
}
func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
func stringList(a []string) []any {
	r := make([]any, len(a))
	for i, v := range a {
		r[i] = v
	}
	return r
}
func sortedUnion(a, b Object) []string {
	o := Object{}
	for k := range a {
		o[k] = nil
	}
	for k := range b {
		o[k] = nil
	}
	return keys(o)
}
func formatID(prefix string, n int) string { return fmt.Sprintf("%s%03d", prefix, n) }
