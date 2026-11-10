package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/store"
	"strings"
)

func (o *operation) dispatch(method string, p Object) (result any, err error) {
	defer guard(&err)
	switch method {
	case "observe-code":
		return o.observe(p), nil
	case "workspace-init":
		return o.buildWorkspace("init", p), nil
	case "workspace-adopt":
		return o.buildWorkspace("adopt", p), nil
	case "workspace-import":
		return o.importWorkspace(p), nil
	case "workspace-recover":
		return o.recoverWorkspace(p), nil
	case "agent-install":
		return o.updateAgent(p, false), nil
	case "project-update":
		return o.updateAgent(p, true), nil
	case "context":
		return o.projectContext(p), nil
	case "doctor":
		return o.doctor(p), nil
	case "explain":
		return explain(o.device, str(p["action"])), nil
	case "list":
		return o.list(), nil
	case "identity":
		hub, _ := o.hubFor(str(p["login"]))
		current, e := hub.User(o.ctx)
		must(e)
		confirmed := o.db["identity"]
		matches := false
		if confirmed != nil {
			c := object(confirmed, "identity")
			matches = equal(current["id"], c["id"]) && equal(current["login"], c["login"])
		}
		return Object{"current": current, "confirmed": confirmed, "matches": matches}, nil
	case "setup":
		return o.setup(p), nil
	case "create":
		return o.create(p), nil
	case "register":
		root, e := fsutil.RealPath(text(p["path"], "path"))
		must(e)
		return o.register(root, readState(root)), nil
	case "scan":
		return o.scan(p), nil
	case "bind-code":
		return o.bindCode(p), nil
	case "disable":
		record := object(object(o.db["projects"], "projects")[str(p["project_id"])], "Project is not registered on this device")
		record["sync_enabled"] = false
		record["status"] = "disabled"
		record["data_status"] = "disabled"
		return record, nil
	case "platform":
		return o.platform(p), nil
	case "project-identity":
		return o.projectIdentity(p), nil
	case "repository-plan":
		return o.repositoryPlan(p), nil
	case "enable":
		return o.enable(p), nil
	case "import":
		return o.importProject(p), nil
	case "sync":
		return o.sync(str(p["project_id"])), nil
	case "resolve":
		return o.resolve(p), nil
	case "conflict-retry":
		return o.conflictRetry(p), nil
	case "discover":
		return o.discover(str(p["login"])), nil
	default:
		return nil, fmt.Errorf("Unknown service method: %s", method)
	}
}
func readState(root string) Object {
	s, e := store.New(root)
	must(e)
	state, e := s.Read(false)
	must(e)
	return state
}
func (o *operation) hubFor(login string) (Hub, Object) {
	auth := object(o.db["meta_auth"], "meta_auth")
	require(login == "" || login == str(auth["login"]), "Discovery uses the device meta account; use setup to change it")
	hub := o.device.Hub
	if truth(auth["login"]) {
		hub = hub.ForLogin(str(auth["login"]))
	}
	return hub, auth
}
func (o *operation) authenticated(login string) Object {
	hub, policy := o.hubFor(login)
	current, e := hub.User(o.ctx)
	must(e)
	require(policy["mode"] == "global" || equal(current["login"], policy["login"]), "Selected GitHub account differs from the requested identity")
	return current
}
func (o *operation) setup(p Object) Object {
	login := str(p["confirm_login"])
	global := truth(p["global_credentials"])
	require((login != "") != global, "Choose --confirm-login or --global-credentials")
	if global {
		o.db["meta_auth"] = authentication("")
		o.db["identity"] = nil
		o.db["repositories"] = Object{}
		return Object{"meta_auth": o.db["meta_auth"]}
	}
	policy := authentication(login)
	hub := o.device.Hub.ForLogin(login)
	current, e := hub.User(o.ctx)
	must(e)
	require(current["login"] == login, "Confirm the exact current GitHub login first")
	o.db["meta_auth"] = policy
	identity := copyObject(current)
	identity["confirmed_at"] = fsutil.Now()
	o.db["identity"] = identity
	object(o.db["accounts"], "accounts")[login] = identity
	o.db["repositories"] = Object{}
	return identity
}
func (o *operation) local(id string) Object {
	projects := object(o.db["projects"], "projects")
	require(has(projects, id), "Project is not registered on this device")
	record := object(projects[id], "Project record")
	require(fsutil.IsDir(str(record["path"])), "Registered Store is missing; inspect its local path")
	require(object(readState(str(record["path"]))["project"], "Project")["id"] == id, "Local Project ID changed")
	return record
}
func (o *operation) available(path, id string) {
	root, e := fsutil.RealPath(path)
	must(e)
	require(!fsutil.Overlap(root, o.device.Home), "Project paths and device home must be separate")
	for _, value := range object(o.db["projects"], "projects") {
		other := object(value, "Project record")
		if other["id"] == id {
			continue
		}
		base := str(other["path"])
		if truth(other["workspace"]) {
			base = str(other["workspace"])
		}
		claimed := []string{base}
		if other["code_paths"] != nil {
			for _, v := range array(other["code_paths"], "code_paths") {
				claimed = append(claimed, str(v))
			}
		}
		for _, value := range claimed {
			existing, e := filepath.Abs(value)
			must(e)
			require(!fsutil.Overlap(root, existing), "Local path is already owned by another Project: "+existing)
		}
	}
}
func (o *operation) register(root string, state Object) Object {
	project := object(state["project"], "Project")
	id := str(project["id"])
	o.available(root, id)
	require(!fsutil.Overlap(root, o.device.Home), "Device registry/runtime and Project Store must be separate directory trees")
	o.engine(nil, nil).assertRoot(root, false)
	projects := object(o.db["projects"], "projects")
	old := projects[id]
	if old != nil {
		record := object(old, "Project record")
		require(record["path"] == root, "This Project ID is already registered at another local path")
		record["name"] = project["name"]
		return record
	}
	for _, v := range projects {
		existing := str(object(v, "Project record")["path"])
		require(existing == root || !fsutil.Overlap(root, existing), "Project Stores may not overlap")
	}
	record := Object{"id": id, "name": project["name"], "path": root, "code_paths": []any{}, "sync_enabled": false, "remote": nil, "status": "local-only", "registered_at": fsutil.Now(), "conflict": nil, "last_sync": nil, "last_head": nil, "error": nil, "identity_version": 4}
	if filepath.Base(root) == "meta" && fsutil.IsFile(filepath.Join(root, configFile)) && fsutil.IsDir(filepath.Join(filepath.Dir(root), "code")) {
		value, e := fsutil.ReadJSON(filepath.Join(root, configFile))
		must(e)
		if oneOf(str(value["format"]), "rum-workspace/1 rum-workspace/2 rum-workspace/3 rum-workspace/4 rum-workspace/5") {
			o.available(filepath.Dir(root), id)
			record["workspace"] = filepath.Dir(root)
			record["workspace_name"] = value["name"]
			if value["format"] == "rum-workspace/1" {
				record["identity_version"] = 2
			}
			record["code_paths"] = []any{filepath.Join(filepath.Dir(root), "code")}
		}
	}
	projects[id] = record
	return record
}
func (o *operation) create(p Object) Object {
	root, e := fsutil.RealPath(text(p["path"], "path"))
	must(e)
	id := str(p["project_id"])
	if id == "" {
		value, e := fsutil.ID()
		must(e)
		id = "P-" + value
	}
	require(!has(object(o.db["projects"], "projects"), id), "Project ID is already registered")
	o.available(root, "")
	ancestor := root
	for !fsutil.Exists(ancestor) {
		parent := filepath.Dir(ancestor)
		require(parent != ancestor, "No existing ancestor")
		ancestor = parent
	}
	o.engine(nil, nil).assertRoot(ancestor, false)
	request := core.Request("project.init", Object{"id": id, "name": p["name"]}, str(p["date"]), "User created a local RumSDLC Project", nil, nil)
	s, e := store.New(root)
	must(e)
	preview, e := s.Execute(request, true, "")
	must(e)
	_, e = s.Execute(request, false, str(preview["revision"]))
	must(e)
	ensureConfig(root, str(p["name"]), nil)
	return o.register(root, readState(root))
}
func (o *operation) list() Object {
	projects, repos := []any{}, []any{}
	localRepos := map[string]Object{}
	for _, id := range keys(object(o.db["projects"], "projects")) {
		row := copyObject(object(object(o.db["projects"], "projects")[id], "record"))
		row["local_available"] = fsutil.IsFile(filepath.Join(str(row["path"]), "state.json"))
		projects = append(projects, row)
		if row["remote"] != nil {
			localRepos[idString(object(row["remote"], "remote")["id"])] = row
		}
	}
	remoteNotLocal := 0
	for _, id := range keys(object(o.db["repositories"], "repositories")) {
		row := copyObject(object(object(o.db["repositories"], "repositories")[id], "repository"))
		local := localRepos[idString(row["id"])]
		if local == nil && row["verified_project"] == true {
			if candidate, ok := getObject(o.db, "projects")[str(row["project_id"])].(Object); ok {
				local = candidate
			}
		}
		row["local_project_id"] = nil
		row["local_path"] = nil
		row["sync_enabled"] = false
		if local != nil {
			row["local_project_id"] = local["id"]
			row["local_path"] = local["path"]
			row["sync_enabled"] = local["sync_enabled"]
		} else {
			remoteNotLocal++
		}
		repos = append(repos, row)
	}
	return Object{"version": Version, "identity": o.db["identity"], "meta_auth": o.db["meta_auth"], "local_projects": projects, "remote_repositories": repos, "counts": Object{"local": len(projects), "remote": len(repos), "known_entries": len(projects) + remoteNotLocal, "remote_not_local": remoteNotLocal}, "discovery": o.db["discovery"]}
}
func (o *operation) scan(p Object) Object {
	found, failures := []any{}, []any{}
	for _, value := range array(p["roots"], "roots") {
		root, e := fsutil.RealPath(str(value))
		must(e)
		require(fsutil.IsDir(root), "Scan root is not a directory")
		e = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				if path != root && (oneOf(entry.Name(), ".git .workflow.lock node_modules __pycache__") || path == o.device.Home) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "state.json" {
				return nil
			}
			folder := filepath.Dir(path)
			var record Object
			operationErr := func() (err error) { defer guard(&err); record = o.register(folder, readState(folder)); return nil }()
			if operationErr != nil {
				failures = append(failures, Object{"path": folder, "error": operationErr.Error()})
				return nil
			}
			found = append(found, record)
			return filepath.SkipDir
		})
		must(e)
	}
	return Object{"registered": found, "errors": failures}
}
func (o *operation) platform(p Object) any {
	record := o.local(str(p["project_id"]))
	s, e := store.New(str(record["path"]))
	must(e)
	args := getObject(p, "args")
	switch p["operation"] {
	case "command":
		request := object(args["request"], "request")
		require(request["action"] != "project.init", "Use project create")
		result, e := s.Execute(request, truth(args["dry_run"]), str(args["expect"]))
		must(e)
		if !truth(args["dry_run"]) {
			record["name"] = object(readState(s.Root)["project"], "Project")["name"]
		}
		return result
	case "query":
		state, e := s.Read(false)
		must(e)
		result, e := core.Query(state, args)
		must(e)
		return result
	case "check":
		state, e := s.Read(false)
		must(e)
		return Object{"ok": true, "version": state["version"], "revision": state["revision"]}
	case "document":
		options := store.DocumentOptions{Table: str(args["table"]), ID: str(args["identifier"]), Field: str(args["field"]), ExpectedHash: str(args["expected_hash"]), Ensure: truth(args["ensure"]), Correction: str(args["correction"])}
		if args["content"] != nil {
			content, ok := args["content"].(string)
			require(ok, "Document content must be text")
			options.Content = &content
		}
		result, e := s.Document(options)
		must(e)
		return result
	}
	require(false, "Unsupported platform operation")
	return nil
}
func (o *operation) migrateIdentity(record Object) {
	if equal(record["identity_version"], 4) {
		return
	}
	require(!truth(record["workspace"]), "Legacy project configuration requires explicit project-update")
	cfg := ensureConfig(str(record["path"]), str(record["name"]), nil)
	if record["remote"] != nil {
		remote := object(record["remote"], "remote")
		b := object(object(cfg["repositories"], "repositories")["meta"], "meta")
		b["url"] = "https://github.com/" + str(remote["full_name"]) + ".git"
		b["id"] = remote["id"]
		b["branch"] = remote["branch"]
	}
	saveConfig(str(record["path"]), cfg)
	delete(record, "github_account")
	record["identity_version"] = 4
}
func (o *operation) projectIdentity(p Object) Object {
	record := o.local(str(p["project_id"]))
	o.migrateIdentity(record)
	cfg := loadConfig(str(record["path"]), true)
	if record["remote"] != nil {
		checkMetaBinding(str(record["path"]), object(record["remote"], "remote"))
	}
	if p["name"] != nil || p["email"] != nil || truth(p["inherit"]) {
		configureAuthor(o.ctx, record, o.device.Home, p)
	}
	return Object{"project_id": record["id"], "repositories": cfg["repositories"], "device_meta_auth": o.db["meta_auth"], "author": effectiveAuthor(o.ctx, record, false), "note": "Meta uses the device GitHub account. Code authentication and authors belong entirely to local Git settings."}
}
func (o *operation) discover(login string) Object {
	identity := o.authenticated(login)
	hub, _ := o.hubFor(login)
	repositories, e := hub.Repositories(o.ctx)
	must(e)
	result := Object{}
	for _, repo := range repositories {
		if !strings.HasPrefix(str(repo["name"]), RepositoryPrefix) {
			continue
		}
		branch := repo["default_branch"]
		if !truth(branch) {
			branch = "main"
		}
		row := Object{"id": repo["id"], "name": repo["name"], "full_name": repo["full_name"], "private": repo["private"], "branch": branch, "writable": truth(getObject(repo, "permissions")["push"]), "archived": truth(repo["archived"]), "pushed_at": repo["pushed_at"], "last_seen": fsutil.Now(), "verified_project": false, "accounts": []any{identity["login"]}}
		row["manifest_status"], row["project_name"], row["display_name"], row["name_source"] = "unavailable", nil, repo["name"], "repository"
		o.inspectRepository(row, repo, hub)
		result[idString(repo["id"])] = row
	}
	o.db["repositories"] = result
	o.db["discovery"] = Object{"at": fsutil.Now(), "accessible_matching_repositories": len(result), "scope": "Repositories visible to the confirmed GitHub credential"}
	return o.list()
}
