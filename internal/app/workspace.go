package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/store"
	"strings"
)

func workspaceSlug(value string) string {
	value = regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(value, "${1}-${2}")
	value = strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(value), "-"), "-")
	if len(value) > 64 {
		value = strings.TrimRight(value[:64], "-")
	}
	if value == "" {
		id, e := fsutil.ID()
		must(e)
		value = "project-" + id[:8]
	}
	return value
}
func (o *operation) checkTarget(root string) {
	o.available(root, "")
	require(!fsutil.Overlap(root, o.device.Home), "Workspace and device service directory must be separate")
	for _, v := range object(o.db["projects"], "projects") {
		record := object(v, "record")
		existing := str(record["path"])
		if truth(record["workspace"]) {
			existing = str(record["workspace"])
		}
		require(!fsutil.Overlap(root, existing), "Workspace overlaps an already registered Project")
	}
	parent := filepath.Dir(root)
	for !fsutil.Exists(parent) {
		next := filepath.Dir(parent)
		require(next != parent, "No existing workspace parent")
		parent = next
	}
	result, e := o.device.Runner.Run(o.ctx, []string{"git", "-C", parent, "rev-parse", "--show-toplevel"}, "", "", nil, 0, false)
	must(e)
	require(result.Code != 0, "Workspace parent is inside another Git repository")
}
func (o *operation) verifyCode(root string) {
	info, e := os.Lstat(filepath.Join(root, ".git"))
	require(e == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0, "Adoption requires a regular repository with an internal .git directory")
	result, e := o.device.Runner.Run(o.ctx, []string{"git", "-C", root, "rev-parse", "--show-toplevel"}, "", "", nil, 0, true)
	must(e)
	actual, e := filepath.EvalSymlinks(strings.TrimSpace(result.Stdout))
	must(e)
	require(actual == root, "Select the repository root")
	require(!fsutil.Exists(filepath.Join(root, ".git", "worktrees")), "Repository has linked worktrees; detach or migrate them before adoption")
	require(!fsutil.Exists(filepath.Join(root, ".git", "modules")) && !fsutil.Exists(filepath.Join(root, ".git", "objects", "info", "alternates")), "Submodules or external object stores require a dedicated relocation procedure")
	must(filepath.WalkDir(filepath.Join(root, ".git"), func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if strings.HasSuffix(entry.Name(), ".lock") {
			return fmt.Errorf("Git is busy; finish the current Git operation")
		}
		return nil
	}))
	result, e = o.device.Runner.Run(o.ctx, []string{"git", "-C", root, "config", "--local", "--get", "core.worktree"}, "", "", nil, 0, false)
	must(e)
	require(strings.TrimSpace(result.Stdout) == "", "External worktree configuration cannot be relocated automatically")
	must(filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Name() == ".git" {
			if path == filepath.Join(root, ".git") {
				return filepath.SkipDir
			}
			return fmt.Errorf("Nested repositories require explicit relocation")
		}
		return nil
	}))
}
func (o *operation) buildWorkspace(mode string, p Object) (record Object) {
	adopting := mode == "adopt"
	root, name, repository := "", "", any(nil)
	isGit := false
	if adopting {
		value, e := fsutil.RealPath(str(p["path"]))
		must(e)
		root = value
		require(fsutil.IsDir(root), "Adoption requires an existing directory")
		isGit = fsutil.Exists(filepath.Join(root, ".git"))
		if isGit {
			o.verifyCode(root)
		} else {
			must(filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if entry.Name() == ".git" {
					return fmt.Errorf("Nested repositories require explicit relocation")
				}
				return nil
			}))
		}
		origin := ""
		if isGit {
			origin = strings.TrimSpace(codeGit(o.ctx, root, "config", "--get", "remote.origin.url").Stdout)
		}
		if origin != "" {
			_ = attempt(func() { repository = portableURL(origin) })
		}
		name = str(p["name"])
		if name == "" {
			name = filepath.Base(root)
		}
		name = workspaceSlug(name)
	} else {
		require(mode == "init", "Only init/adopt are supported; code acquisition belongs to the agent")
		name = str(p["name"])
		if name == "" {
			name = "my-project"
		}
		name = workspaceSlug(name)
		parent, e := fsutil.RealPath(str(p["parent"]))
		must(e)
		root = filepath.Join(parent, name)
		require(!fsutil.Exists(root), "Workspace already exists")
	}
	o.checkTarget(root)
	must(os.MkdirAll(filepath.Dir(root), 0700))
	stage, e := os.MkdirTemp(filepath.Dir(root), ".rum-workspace-")
	must(e)
	id, e := fsutil.ID()
	must(e)
	journal := filepath.Join(o.device.Home, "operations", id+".json")
	state := Object{"id": id, "mode": mode, "target": root, "stage": stage, "phase": "prepared"}
	must(fsutil.WriteJSON(journal, state))
	before := copyObject(o.db)
	moved, published, complete := false, false, false
	defer func() {
		if complete {
			return
		}
		o.db = before
		if published {
			must(os.Rename(root, stage))
		}
		if adopting && moved {
			must(os.Rename(filepath.Join(stage, "code"), root))
		}
		must(os.RemoveAll(stage))
		state["phase"] = "rolled-back"
		must(fsutil.WriteJSON(journal, state))
	}()
	if adopting {
		must(os.Rename(root, filepath.Join(stage, "code")))
		moved = true
	} else {
		must(os.Mkdir(filepath.Join(stage, "code"), 0700))
	}
	state["phase"] = "directory-ready"
	must(fsutil.WriteJSON(journal, state))
	uuid, e := fsutil.ID()
	must(e)
	pid := "P-" + uuid
	request := core.Request("project.init", Object{"id": pid, "name": name}, str(p["date"]), "User attached a workspace", nil, nil)
	s, e := store.New(filepath.Join(stage, "meta"))
	must(e)
	preview, e := s.Execute(request, true, "")
	must(e)
	_, e = s.Execute(request, false, str(preview["revision"]))
	must(e)
	cfg := ensureConfig(filepath.Join(stage, "meta"), name, repository)
	if adopting {
		kind := "directory"
		if isGit {
			kind = "git"
		}
		object(object(cfg["repositories"], "repositories")["code"], "code")["kind"] = kind
	}
	saveConfig(filepath.Join(stage, "meta"), cfg)
	values := Object{"id": pid, "workspace": root, "workspace_name": name}
	o.installAgent(values, stage, false, nil)
	state["phase"] = "ready"
	must(fsutil.WriteJSON(journal, state))
	must(os.Rename(stage, root))
	published = true
	record = o.register(filepath.Join(root, "meta"), readState(filepath.Join(root, "meta")))
	for key, value := range values {
		record[key] = value
	}
	record["code_paths"] = []any{filepath.Join(root, "code")}
	state["phase"] = "complete"
	must(fsutil.WriteJSON(journal, state))
	complete = true
	return record
}
func (o *operation) recoverWorkspace(p Object) Object {
	id := str(p["operation_id"])
	require(regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id), "Invalid operation ID")
	journal := filepath.Join(o.device.Home, "operations", id+".json")
	value, e := fsutil.ReadJSON(journal)
	must(e)
	root, stage := str(value["target"]), str(value["stage"])
	require(value["mode"] == "adopt" && !oneOf(str(value["phase"]), "complete rolled-back recovered"), "This operation does not need adoption recovery")
	require(!fsutil.Exists(root) && fsutil.IsDir(filepath.Join(stage, "code")), "Recovery is ambiguous; retain both paths and inspect the journal")
	must(os.Rename(filepath.Join(stage, "code"), root))
	value["phase"] = "recovered"
	must(fsutil.WriteJSON(journal, value))
	return Object{"restored": root, "retained_staging": stage}
}
func (o *operation) importWorkspace(p Object) Object {
	hub, _ := o.hubFor("")
	identity := o.authenticated("")
	repo, e := hub.Repository(o.ctx, str(p["repository"]))
	must(e)
	checkedRepository(repo, false)
	branch := str(repo["default_branch"])
	if branch == "" {
		branch = "main"
	}
	manifest, e := hub.Manifest(o.ctx, str(repo["full_name"]), branch)
	must(e)
	require(!has(object(o.db["projects"], "projects"), str(manifest["project_id"])), "Project already registered")
	parent, e := fsutil.RealPath(str(p["parent"]))
	must(e)
	uuid, e := fsutil.ID()
	must(e)
	o.checkTarget(filepath.Join(parent, ".rum-import-"+uuid))
	must(os.MkdirAll(parent, 0700))
	stage, e := os.MkdirTemp(parent, ".rum-import-")
	must(e)
	published, complete := false, false
	before := copyObject(o.db)
	defer func() {
		if !complete && !published {
			o.db = before
		}
		if fsutil.Exists(stage) {
			os.RemoveAll(stage)
		}
	}()
	state := o.engine(nil, hub).importProject(repo, filepath.Join(stage, "meta"))
	validateManifest(manifest, state)
	cfg := loadConfig(filepath.Join(stage, "meta"), true)
	name := str(p["name"])
	if name == "" {
		name = str(cfg["name"])
	}
	name = workspaceSlug(name)
	root := filepath.Join(parent, name)
	require(!fsutil.Exists(root), "Workspace already exists")
	o.checkTarget(root)
	binding := object(object(cfg["repositories"], "repositories")["meta"], "meta")
	url := "https://github.com/" + str(repo["full_name"]) + ".git"
	require(binding["url"] == nil || binding["url"] == url, "Imported meta URL differs from selected repository")
	require(!has(binding, "id") || equal(binding["id"], repo["id"]), "Imported meta repository ID differs")
	binding["url"] = url
	binding["id"] = repo["id"]
	binding["branch"] = branch
	binding["visibility"] = "public"
	if repo["private"] == true {
		binding["visibility"] = "private"
	}
	saveConfig(filepath.Join(stage, "meta"), cfg)
	must(os.Mkdir(filepath.Join(stage, "code"), 0700))
	values := Object{"id": object(state["project"], "Project")["id"], "workspace": root, "workspace_name": name}
	o.installAgent(values, stage, false, nil)
	must(os.Rename(stage, root))
	published = true
	record := o.register(filepath.Join(root, "meta"), state)
	for k, v := range values {
		record[k] = v
	}
	record["code_paths"] = []any{filepath.Join(root, "code")}
	record["remote"] = Object{"id": repo["id"], "full_name": repo["full_name"], "branch": branch, "allow_empty": false}
	record["sync_enabled"] = getObject(repo, "permissions")["push"] == true
	record["data_status"] = "synced"
	record["last_head"] = o.engine(nil, nil).head(filepath.Join(root, "meta"))
	record["last_sync"] = fsutil.Now()
	record["agent_status"] = "ready"
	record["status"] = "read-only-import"
	if record["sync_enabled"] == true {
		record["status"] = "synced"
	}
	o.remember(repo, str(record["id"]), str(identity["login"]))
	o.lock(str(record["id"]))
	o.checkpoint()
	complete = true
	result := copyObject(record)
	result["code"] = Object{"status": "not-inspected", "blocking": false, "association": object(cfg["repositories"], "repositories")["code"], "next": "Agent obtains and verifies code independently; import transfers metadata only"}
	return result
}
