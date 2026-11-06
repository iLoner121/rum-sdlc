package app

import (
	"os"
	"path/filepath"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"sort"
	"strings"
)

func (o *operation) find(p Object) Object {
	if truth(p["project_id"]) {
		return o.local(str(p["project_id"]))
	}
	path := str(p["path"])
	if path == "" {
		value, e := os.Getwd()
		must(e)
		path = value
	}
	location, e := fsutil.RealPath(path)
	must(e)
	matches := []Object{}
	for _, v := range object(o.db["projects"], "projects") {
		r := object(v, "record")
		root := str(r["path"])
		if truth(r["workspace"]) {
			root = str(r["workspace"])
		}
		matched := fsutil.Contains(root, location)
		for _, path := range array(r["code_paths"], "code_paths") {
			matched = matched || fsutil.Contains(str(path), location)
		}
		if matched {
			matches = append(matches, r)
		}
	}
	require(len(matches) == 1, "No unique registered Project matches this path; supply --project or --path")
	return matches[0]
}
func (o *operation) projectContext(p Object) Object {
	record := o.find(p)
	state := readState(str(record["path"]))
	cfg := loadConfig(str(record["path"]), true)
	runtime, e := os.Executable()
	must(e)
	var entry, skill any
	if truth(record["workspace"]) {
		entry = filepath.Join(str(record["workspace"]), "AGENTS.md")
		skill = filepath.Join(str(record["workspace"]), ".agents", "skills", "rum-project", "SKILL.md")
	}
	return Object{"project_id": record["id"], "name": object(state["project"], "Project")["name"], "meta": record["path"], "workspace": record["workspace"], "code_paths": record["code_paths"], "sync_enabled": record["sync_enabled"], "runtime": runtime, "version": Version, "core_version": state["version"], "revision": state["revision"], "agent_entry": entry, "skill": skill, "repositories": cfg["repositories"], "code_status": "not-inspected", "code": Object{"status": "not-inspected", "blocking": false, "association": object(cfg["repositories"], "repositories")["code"]}, "code_authentication": "managed by local Git; not checked by RumSDLC", "device_meta_auth": o.db["meta_auth"], "launch_directory": record["workspace"], "topic_selection": "Explicit per-session Topic ID; no global current Topic"}
}
func (o *operation) doctor(p Object) Object {
	record := o.find(p)
	root := str(record["path"])
	if truth(record["workspace"]) {
		root = str(record["workspace"])
	}
	checks := []any{}
	ok := true
	var compatibility any
	check := func(name string, value bool, detail string) {
		checks = append(checks, Object{"name": name, "ok": value, "detail": detail})
		ok = ok && value
	}
	if e := attempt(func() { readState(str(record["path"])) }); e != nil {
		check("store", false, e.Error())
	} else {
		check("store", true, str(record["path"]))
	}
	check("layout", !truth(record["workspace"]) || str(record["path"]) == filepath.Join(root, "meta"), root)
	runtime, e := os.Executable()
	runtimeDetail := runtime
	if active, err := fsutil.ReadJSON(filepath.Join(o.device.Home, "active.json")); err == nil && active["runtime_kind"] == "go-native" {
		if err = fsutil.VerifyNativeRelease(str(active["runtime"])); err != nil {
			e = err
			runtimeDetail = err.Error()
		}
	}
	check("runtime", e == nil && fsutil.IsFile(runtime), runtimeDetail)
	check("configuration", fsutil.IsFile(filepath.Join(str(record["path"]), configFile)), "Project workflow configuration")
	if truth(record["workspace"]) && fsutil.IsFile(filepath.Join(root, "meta", configFile)) {
		value, e := o.checkAgent(record)
		if e != nil {
			check("compatible rules", false, e.Error())
		} else {
			compatibility = value
			check("compatible rules", true, "Installed project contract is compatible; update is explicit")
		}
	}
	e = attempt(func() {
		loadConfig(str(record["path"]), true)
		if record["remote"] != nil {
			o.engine(record, nil).validateLocal(str(record["path"]), str(record["id"]))
			check("meta structure", true, "Store, configuration, manifest and remote binding are valid")
		}
		if truth(record["sync_enabled"]) {
			effectiveAuthor(o.ctx, record, true)
		}
		check("pending conflict", !truth(record["conflict"]), "Resolve retained candidate before resuming synchronization")
		check("generation recovery", !fsutil.Exists(filepath.Join(root, ".rum-agent-update.json")), "agent-install recovers interrupted generation")
	})
	if e != nil {
		check("project configuration", false, e.Error())
	}
	dataStatus := record["data_status"]
	if !has(record, "data_status") {
		dataStatus = record["status"]
	}
	return Object{"ok": ok, "project_compatibility": compatibility, "checks": checks, "launch_directory": root, "code_status": "not-inspected", "reports": []any{}, "data_status": dataStatus, "status": record["status"], "device_meta_auth": o.db["meta_auth"], "note": "ok describes RumSDLC health; code reports never block it. Launch from this root. Starting inside code may stop discovery at code/.git. Actual host loading must be checked in the agent session."}
}
func explain(device *Device, action string) Object {
	if action != "" {
		parameters, ok := core.Parameters[action]
		require(ok, "Unknown platform action")
		allowed := strings.Fields(parameters)
		sort.Strings(allowed)
		return Object{"action": action, "allowed_parameters": allowed, "request_envelope": []any{"action", "params", "date", "reason", "evidence", "decision"}, "write_protocol": "platform command: dry_run=true, then expect=<returned revision>", "rules": "Use runtime validation and project workflow; allowed parameters are not all required."}
	}
	actions := []string{}
	for action := range core.Parameters {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	methods := strings.Fields(catalog.Methods)
	sort.Strings(methods)
	runtime, e := device.ToolRuntime()
	must(e)
	return Object{"version": Version, "core_version": core.Version, "actions": actions,
		"commands": methods, "command_help": catalog.Help(), "manual": filepath.Join(runtime, "docs", "usage.md"),
		"names":         Object{"workspace": "lowercase-hyphen; existing adopted directory name is preserved", "Project": "P-<uuid>", "Milestone": "M0, M1, ...", "Iteration": "ITER-001, ...", "Pool": "POOL", "Line": "lowercase-hyphen slug; mainline is the default", "Topic": "T-YYYYMMDD-xxxxxxxx", "Task": "t001, ... scoped by Topic", "github_repository": "RumSDLC_<Project-ID>"},
		"optional_sync": []any{"Configure existing Git credentials or optionally gh auth login", "optional: setup --confirm-login META_LOGIN or setup --global-credentials", "project-identity --project ID", "enable --project ID"},
		"help":          "tool workflow -- --help; tool git_adapter -- --help; tool commit_check -- --help",
		"limits":        []any{"No GUI in this release", "Native skill discovery depends on the agent working directory", "Readiness checks do not claim a real Codex session loaded the guidance"}, "layout": Object{"code": "External code, never inspected by context/doctor", "meta": "Project state, documents and independent GitHub sync repository", "AGENTS.md": "Generated session guidance", ".agents/skills": "Generated project skills"}, "onboarding": []any{"workspace-init or workspace-adopt", "context", "doctor"}, "code_binding": "bind-code records a path/source; --topic branch/review associations are explicit; no local code discovery.", "identity": "Meta uses the device GitHub account. Code uses local Git credentials and author settings, with no RumSDLC account selection.", "code_observation": "Use observe-code --topic ID --remote to query an explicitly registered remote review. Agent handles all code/Git work; observations never change workflow state."}
}
