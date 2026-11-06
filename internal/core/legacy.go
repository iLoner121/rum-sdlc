package core

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"rumsdlc/internal/jsonvalue"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// LegacySource is a captured, read-only source snapshot. Paths use forward
// slashes; filesystem safety and executable-mode capture belong to migration.
type LegacyRecord struct {
	Path string
	Data Object
}
type LegacySource struct {
	Root       string
	Config     Object
	Records    map[string][]LegacyRecord
	Files      map[string][]byte
	Executable map[string]bool
}
type LegacyResult struct {
	State  Object
	Files  map[string][]byte
	Report Object
}

const migrationLimits = "One explicitly reconciled local snapshot only; no automatic worktree authority selection, Git history rewrite or remote synchronization"

func legacyClosure(old Object, extra string) Object {
	r := Object{}
	for _, k := range strings.Fields("outcome closed_on summary evidence " + extra) {
		r[k] = clone(old[k])
	}
	return r
}
func legacyRecord(s Object, id string, name, created any, status string, extra Object) Object {
	r := newEntity(s, id, name, str(created), status, extra)
	r["created_on"] = created
	return r
}
func legacyUTF8(raw []byte) string {
	need(utf8.Valid(raw), "Legacy document is not valid UTF-8")
	return string(raw)
}
func legacyUnfenced(content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	fence := ""
	size := 0
	opening := regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	for i, line := range lines {
		if fence == "" {
			if m := opening.FindStringSubmatch(line); m != nil {
				fence = m[1][:1]
				size = len(m[1])
				lines[i] = ""
			}
			continue
		}
		closing := regexp.MustCompile("^ {0,3}" + regexp.QuoteMeta(fence) + "{" + strconv.Itoa(size) + ",}\\s*$")
		if closing.MatchString(line) {
			fence = ""
		}
		lines[i] = ""
	}
	need(fence == "", "Unclosed Markdown fence in Task")
	return strings.Join(lines, "\n")
}

// ConvertLegacy preserves source bytes, translates explicit source records and
// seals one native migration event. It never runs source scripts or infers Git facts.
func ConvertLegacy(source LegacySource, id, name, when, active string) (result LegacyResult, err error) {
	defer catch(&err)
	need(legacyVersion(source.Config["protocol_version"], false), "Prepare a reconciled protocol-3/3.2.x snapshot first")
	main := legacyBranch(source.Config["main_branch"])
	for kind, records := range source.Records {
		for _, record := range records {
			e := ValidateLegacyRecord(record.Data, kind, path.Base(path.Dir(record.Path)), main)
			need(e == nil, errorText(e))
		}
	}
	rawFiles := map[string][]byte{}
	payload := map[string][]byte{}
	executable := map[string]bool{}
	mapping := map[string]string{}
	for name, data := range source.Files {
		rawFiles[name] = append([]byte{}, data...)
		payload["legacy/source/"+name] = append([]byte{}, data...)
		if source.Executable[name] {
			executable["legacy/source/"+name] = true
		}
	}
	request := Request("project.init", Object{"id": id, "name": name}, when, "Initialize explicit legacy import", nil, nil)
	s, _, e := Dispatch(nil, request)
	need(e == nil, errorText(e))
	s["history"] = []any{}
	s["revision"] = 0
	entity(s, "lines", "mainline")["extensions"] = Object{"git": Object{"branch": main, "base_commit": nil}}
	for _, record := range source.Records["milestone"] {
		data := clone(record.Data).(Object)
		identifier := str(data["id"])
		status := "planned"
		if data["status"] == "closed" {
			status = "closed"
		} else if identifier == active {
			status = "active"
		}
		position, e := strconv.Atoi(identifier[1:])
		need(e == nil, "Invalid Milestone ID")
		row := legacyRecord(s, identifier, data["title"], data["created_on"], status, Object{"position": position, "legacy": data})
		if status == "closed" {
			row["closure"] = legacyClosure(obj(data["closure"], "closure"), "successor")
		}
		rows(s, "milestones")[identifier] = row
		mapping[path.Join(path.Dir(record.Path), "MILESTONE.md")] = "documents/milestones/" + identifier + "/DESCRIPTION.md"
	}
	need(active == "" || has(rows(s, "milestones"), active), "Selected active milestone missing")
	for _, record := range source.Records["line"] {
		data := clone(record.Data).(Object)
		identifier := str(data["id"])
		need(identifier != "mainline", "Legacy Line ID conflicts with reserved default mainline; explicitly map before import")
		name := data["title"]
		if !has(data, "title") {
			name = identifier
		}
		var target any
		if data["kind"] == "development" {
			target = "mainline"
		}
		row := legacyRecord(s, identifier, name, data["created_on"], str(data["status"]), Object{"type": data["kind"], "target_line_id": target, "legacy": data})
		row["extensions"] = Object{"git": Object{"branch": data["branch"], "base_commit": data["base_commit"]}}
		if row["status"] == "closed" {
			old := obj(data["closure"], "closure")
			c := legacyClosure(old, "verification delivery_record retention discard_decision successor last_release extensions")
			c["evidence"] = old["verification"]
			var tip any
			history := legacyList(data, "state_history")
			for i := len(history) - 1; i >= 0; i-- {
				event := obj(history[i], "event")
				if event["to"] == "closed" && truth(event["observed_tip"]) {
					tip = event["observed_tip"]
					break
				}
			}
			if old["outcome"] != "integrated" {
				tip = nil
			}
			c["delivery_record"] = old["integration_commit"]
			c["retention"] = old["retained_ref"]
			c["last_release"] = old["last_release_ref"]
			c["extensions"] = Object{"git": Object{"integration_commit": old["integration_commit"], "expect_tip": tip, "retained_ref": old["retained_ref"], "last_release_ref": old["last_release_ref"], "last_release_commit": old["last_release_commit"]}}
			row["closure"] = c
		}
		rows(s, "lines")[identifier] = row
		mapping[path.Join(path.Dir(record.Path), "LINE_CONTEXT.md")] = "documents/lines/" + identifier + "/DESCRIPTION.md"
	}
	rounds := []Object{}
	for _, record := range source.Records["iteration"] {
		data := clone(record.Data).(Object)
		identifier := str(data["id"])
		rounds = append(rounds, data)
		activated := data["started_on"] != nil
		proposed := []any{}
		if data["status"] == "draft" {
			proposed = clone(data["topics"]).([]any)
		}
		var revision any
		if activated {
			revision = 1
		}
		row := legacyRecord(s, identifier, data["title"], data["created_on"], str(data["status"]), Object{"type": "iteration", "proposed_topics": proposed, "initial_topics": clone(data["initial_topics"]), "activation_revision": revision, "started_on": data["started_on"], "last_reviewed_on": data["last_reviewed_on"], "legacy": data})
		if row["status"] == "closed" {
			old := obj(data["closure"], "closure")
			c := legacyClosure(old, "results goal_acceptance")
			c["evidence"] = old["summary"]
			if old["outcome"] == "completed" {
				c["goal_acceptance"] = "legacy/source/" + record.Path + "#closure"
			} else {
				c["goal_acceptance"] = nil
			}
			row["closure"] = c
		}
		rows(s, "worksets")[identifier] = row
		mapping[path.Join(path.Dir(record.Path), "ITERATION.md")] = "documents/worksets/" + identifier + "/DESCRIPTION.md"
	}
	fileNames := []string{}
	for name := range rawFiles {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	for _, record := range source.Records["topic"] {
		data := clone(record.Data).(Object)
		identifier := str(data["id"])
		row := legacyRecord(s, identifier, data["title"], data["created_on"], str(data["status"]), Object{"type": data["type"], "slug": data["slug"], "dependencies": data["dependencies"], "integration_recheck": nil, "legacy": data})
		rows(s, "topics")[identifier] = row
		milestone := data["milestone"]
		line := data["line"]
		if !truth(line) {
			line = "mainline"
		}
		setRelation(s, "milestone", identifier, milestone)
		setRelation(s, "line", identifier, line)
		candidates := []Object{}
		historical := []Object{}
		for _, it := range rounds {
			topics, initial := asSet(arr(it["topics"], "topics")), asSet(arr(it["initial_topics"], "initial_topics"))
			if topics[identifier] && it["status"] == "active" {
				candidates = append(candidates, it)
			}
			if topics[identifier] || initial[identifier] {
				historical = append(historical, it)
			}
		}
		need(len(candidates) <= 1, "Topic belongs to several active iterations")
		queue := "POOL"
		if len(candidates) > 0 {
			queue = str(candidates[0]["id"])
		}
		if row["status"] != "closed" {
			setRelation(s, "workset", identifier, queue)
		} else {
			old := obj(data["closure"], "closure")
			c := legacyClosure(old, "verification memberships delivery_record integration_target delivery_scope retention successor extensions")
			sort.Slice(historical, func(i, j int) bool { return str(historical[i]["id"]) < str(historical[j]["id"]) })
			var workset any
			if len(historical) > 0 {
				workset = historical[len(historical)-1]["id"]
			}
			c["evidence"] = c["verification"]
			c["memberships"] = Object{"topic_id": identifier, "milestone_id": milestone, "workset_id": workset, "line_id": line, "revision": 1}
			c["integration_target"] = line
			c["delivery_record"] = old["integration_commit"]
			c["retention"] = old["retained_ref"]
			c["extensions"] = Object{"git": Object{"integration_commit": old["integration_commit"], "retained_ref": old["retained_ref"]}}
			row["closure"] = c
		}
		if truth(data["integration_recheck"]) {
			old := obj(data["integration_recheck"], "recheck")
			var event Object
			events := legacyList(data, "assignment_changes")
			for i := len(events) - 1; i >= 0; i-- {
				v := obj(events[i], "event")
				if v["kind"] == "target" {
					event = v
					break
				}
			}
			need(event != nil, "Explicit source target context required for baseline import")
			if event["previous_head"] == nil || event["prepared_head"] == nil {
				need(event["previous_head"] == nil && event["prepared_head"] == nil && event["previous_status"] == "planned", "Incomplete started-source code context")
			}
			from := obj(event["before"], "target")["line"]
			if !truth(from) {
				from = "mainline"
			}
			context := Object{"topic_id": identifier, "from_line_id": from, "line_id": line, "dependencies": event["dependencies"], "contracts": "legacy/source/" + record.Path + "#assignment_changes", "extensions": Object{"git": Object{"baseline": old["baseline"], "previous_head": event["previous_head"], "prepared_head": event["prepared_head"], "retained_ref": event["retained_ref"]}}}
			var proof any
			if old["required"] == false {
				p := clone(context).(Object)
				p["verification"] = Object{"git": Object{"verified_head": old["verified_head"]}}
				p["verified_on"] = old["verified_on"]
				p["report"] = old["evidence"]
				proof = p
			}
			row["integration_recheck"] = Object{"required": old["required"], "line_id": line, "evidence": old["evidence"], "context": context, "proof": proof}
		}
		prefix := path.Dir(record.Path)
		for _, relative := range fileNames {
			if strings.HasPrefix(relative, prefix+"/") && !strings.HasSuffix(relative, "/topic.json") {
				mapping[relative] = "documents/topics/" + identifier + "/" + strings.TrimPrefix(relative, prefix+"/")
			}
		}
		for _, relative := range fileNames {
			if !strings.HasPrefix(relative, prefix+"/tasks/") {
				continue
			}
			filename := strings.TrimPrefix(relative, prefix+"/tasks/")
			match(filename, `t[0-9]{3,}\.md`, "legacy Task file")
			content := strings.ReplaceAll(strings.ReplaceAll(legacyUTF8(rawFiles[relative]), "\r\n", "\n"), "\r", "\n")
			visible := legacyUnfenced(content)
			headers := regexp.MustCompile(`(?m)^# (t[0-9]+) (.+)$`).FindAllStringSubmatch(visible, -1)
			states := regexp.MustCompile(`(?m)^- Status: (\S+)$`).FindAllStringSubmatch(visible, -1)
			kinds := regexp.MustCompile(`(?m)^- Kind: (\S+)$`).FindAllStringSubmatch(visible, -1)
			owners := regexp.MustCompile(`(?m)^- Topic: (\S+)$`).FindAllStringSubmatch(visible, -1)
			stem := strings.TrimSuffix(filename, ".md")
			need(len(headers) == 1 && len(states) == 1 && len(owners) == 1 && headers[0][1] == stem && owners[0][1] == identifier, "Ambiguous legacy Task metadata")
			need(len(kinds) <= 1, "Duplicate legacy Kind")
			kind := "legacy-unknown"
			if len(kinds) == 1 {
				kind = kinds[0][1]
			}
			key := identifier + "/" + stem
			var result, destination any
			if states[0][1] == "done" {
				result = "legacy/source/" + relative
			}
			if states[0][1] == "deferred" {
				destination = "See original deferred activity record"
			}
			rows(s, "tasks")[key] = legacyRecord(s, key, headers[0][2], nil, states[0][1], Object{"topic_id": identifier, "kind": kind, "result": result, "destination": destination, "legacy": Object{"source": relative, "kind_known": len(kinds) > 0}})
			lines, unfenced := strings.SplitAfter(content, "\n"), strings.Split(visible, "\n")
			managed := regexp.MustCompile(`^- (Status|Kind|Topic): `)
			for i, line := range unfenced {
				if i < len(lines) && managed.MatchString(line) {
					lines[i] = ""
				}
			}
			rawFiles[relative] = []byte(strings.Join(lines, ""))
		}
	}
	mapping["PROJECT_CONTEXT.md"] = "documents/project/" + id + "/DESCRIPTION.md"
	mapping["workflow/BACKLOG.md"] = "documents/worksets/POOL/DESCRIPTION.md"
	broken := map[string]bool{}
	for old, next := range mapping {
		raw, ok := rawFiles[old]
		if !ok {
			continue
		}
		if strings.HasSuffix(old, ".md") {
			content := legacyUTF8(raw)
			links := regexp.MustCompile(`\]\(([^)]+)\)`)
			content = links.ReplaceAllStringFunc(content, func(original string) string {
				dest := links.FindStringSubmatch(original)[1]
				if regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`).MatchString(dest) || strings.HasPrefix(dest, "#") {
					return original
				}
				address, anchor, hasAnchor := strings.Cut(dest, "#")
				target := path.Clean(path.Join(path.Dir(old), address))
				if strings.HasPrefix(address, "/") {
					root := strings.TrimSuffix(source.Root, "/")
					if !strings.HasPrefix(address, root+"/") {
						broken[old+": "+dest] = true
						return original
					}
					target = strings.TrimPrefix(path.Clean(address), root+"/")
				}
				if target == ".." || strings.HasPrefix(target, "../") {
					broken[old+": "+dest] = true
					return original
				}
				mapped, ok := mapping[target]
				if !ok {
					broken[old+": "+dest] = true
					return original
				}
				relative := legacyRel(path.Dir(next), mapped)
				if hasAnchor {
					relative += "#" + anchor
				}
				return "](" + relative + ")"
			})
			raw = []byte(content)
		}
		payload[next] = raw
		if executable["legacy/source/"+old] {
			executable[next] = true
		}
	}
	for _, it := range rounds {
		record := entity(s, "worksets", str(it["id"]))
		if record["activation_revision"] != nil {
			touched := asSet(arr(record["initial_topics"], "initial_topics"))
			for _, v := range legacyList(it, "changes") {
				event := obj(v, "change")
				if event["action"] == "add" {
					for _, id := range arr(event["topics"], "topics") {
						touched[str(id)] = true
					}
				}
			}
			names := []string{}
			for id := range touched {
				names = append(names, id)
			}
			sort.Strings(names)
			obj(record["legacy"], "legacy")["imported_touched_topics"] = stringList(names)
		}
	}
	hashes := Object{}
	for name, data := range payload {
		hashes[name] = jsonvalue.HashBytes(data)
	}
	modes := []string{}
	for name := range executable {
		modes = append(modes, name)
	}
	sort.Strings(modes)
	digest, e := jsonvalue.Hash(Object{"files": hashes, "executable_files": stringList(modes)})
	need(e == nil, errorText(e))
	req := Request("migration.import", Object{"source_version": source.Config["protocol_version"], "source_digest": digest}, when, "Explicitly adopt reconciled legacy records", "legacy/source/", nil)
	s, _, e = seal(nil, s, req, id)
	need(e == nil, errorText(e))
	e = Validate(s)
	need(e == nil, errorText(e))
	links := []string{}
	for link := range broken {
		links = append(links, link)
	}
	sort.Strings(links)
	report := Object{"project_id": id, "source_digest": digest, "executable_files": stringList(modes), "document_links_requiring_review": stringList(links), "limits": migrationLimits}
	// The report's field order is part of the original byte/token contract.
	ordered := struct {
		Project    string   `json:"project_id"`
		Digest     string   `json:"source_digest"`
		Executable []string `json:"executable_files"`
		Links      []string `json:"document_links_requiring_review"`
		Limits     string   `json:"limits"`
	}{id, digest, modes, links, migrationLimits}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	e = encoder.Encode(ordered)
	need(e == nil, errorText(e))
	payload["migration-report.json"] = buffer.Bytes()
	return LegacyResult{s, payload, report}, nil
}
func errorText(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
func legacyRel(from, to string) string {
	a, b := strings.Split(from, "/"), strings.Split(to, "/")
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a = a[1:]
		b = b[1:]
	}
	parts := []string{}
	for range a {
		parts = append(parts, "..")
	}
	parts = append(parts, b...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}
