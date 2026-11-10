package app

import (
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"sort"
	"strings"
)

func (o *operation) remember(repo Object, id, login string) {
	catalog := object(o.db["repositories"], "repositories")
	key := idString(repo["id"])
	accounts := map[string]bool{}
	if old := catalog[key]; old != nil {
		for _, a := range array(object(old, "repository")["accounts"], "accounts") {
			accounts[str(a)] = true
		}
	}
	if login != "" {
		accounts[login] = true
	}
	names := []string{}
	for name := range accounts {
		names = append(names, name)
	}
	sort.Strings(names)
	values := []any{}
	for _, name := range names {
		values = append(values, name)
	}
	branch := repo["default_branch"]
	if !truth(branch) {
		branch = "main"
	}
	catalog[key] = Object{"id": repo["id"], "name": repo["name"], "full_name": repo["full_name"], "private": repo["private"], "branch": branch, "writable": truth(getObject(repo, "permissions")["push"]), "archived": truth(repo["archived"]), "pushed_at": repo["pushed_at"], "last_seen": fsutil.Now(), "verified_project": true, "project_id": id, "accounts": values}
	row := object(catalog[key], "repository")
	row["display_name"], row["name_source"] = repo["name"], "repository"
	if local, ok := getObject(o.db, "projects")[id].(Object); ok {
		row["project_name"], row["display_name"], row["name_source"] = local["name"], local["name"], "local-project"
	}

}
func (o *operation) enable(p Object) Object {
	id := str(p["project_id"])
	record := o.local(id)
	o.migrateIdentity(record)
	o.engine(record, nil).validateLocal(str(record["path"]), id)
	identity := o.authenticated("")
	hub, _ := o.hubFor("")
	effectiveAuthor(o.ctx, record, true)
	cfg := loadConfig(str(record["path"]), true)
	binding := object(object(cfg["repositories"], "repositories")["meta"], "meta")
	require(!(truth(p["repository"]) && p["repository_name"] != nil), "Choose --repository for an existing repository or --repository-name for a new name")
	repository := str(p["repository"])
	if repository == "" && truth(binding["url"]) {
		repository = strings.TrimSuffix(strings.TrimPrefix(str(binding["url"]), "https://github.com/"), ".git")
	}
	require(record["conflict"] == nil, "Resolve the existing conflict before rebinding")
	var repo Object
	if record["remote"] != nil {
		require(p["repository_name"] == nil, "Project already has a repository; use sync to follow its verified rename")
		repo = o.boundRepository(record, hub, str(p["repository"]))
	} else {
		created := false
		if repository != "" {
			value, e := hub.Repository(o.ctx, repository)
			must(e)
			repo = value
		} else {
			o.discover("")
			planParams := Object{"project_id": id}
			if p["repository_name"] != nil {
				planParams["name"] = p["repository_name"]
			}
			name := str(o.repositoryPlan(planParams)["repository_name"])
			matches := []Object{}
			for _, v := range object(o.db["repositories"], "repositories") {
				r := object(v, "repository")
				accountMatch := false
				for _, a := range array(r["accounts"], "accounts") {
					accountMatch = accountMatch || equal(a, identity["login"])
				}
				if r["verified_project"] == true && r["project_id"] == id && accountMatch {
					if p["repository_name"] == nil || strings.EqualFold(str(r["name"]), name) {
						matches = append(matches, r)
					}
				}
			}
			require(len(matches) <= 1, "Several repositories identify this Project; select owner/name explicitly")
			if len(matches) > 0 {
				value, e := hub.Repository(o.ctx, str(matches[0]["full_name"]))
				must(e)
				repo = value
			} else {
				for _, value := range object(o.db["repositories"], "repositories") {
					candidate := object(value, "repository")
					owner := strings.SplitN(str(candidate["full_name"]), "/", 2)[0]
					require(!strings.EqualFold(owner, str(identity["login"])) || !strings.EqualFold(str(candidate["name"]), name), "Repository name is occupied: "+name+"; choose another --repository-name or explicitly select the existing repository")
				}
				value, e := hub.Create(o.ctx, name)
				if e != nil {
					require(false, "Could not create repository "+name+"; verify permissions or choose another --repository-name if occupied")
				}
				created = true
				repo, e = hub.Repository(o.ctx, str(value["full_name"]))
				must(e)
			}
		}
		checkedRepository(repo, true)
		branch := str(repo["default_branch"])
		if branch == "" {
			branch = "main"
		}
		if !created {
			if truth(p["accept_empty"]) {
				empty, e := hub.Empty(o.ctx, str(repo["full_name"]))
				must(e)
				created = empty
			}
			if !created {
				value, e := hub.Manifest(o.ctx, str(repo["full_name"]), branch)
				must(e)
				validateManifest(value, readState(str(record["path"])))
			}
		}
		o.engine(record, hub).ensure(str(record["path"]), readState(str(record["path"])))
		record["remote"] = Object{"id": repo["id"], "full_name": repo["full_name"], "branch": branch, "allow_empty": created}
	}
	setMetaBinding(record, repo, str(object(record["remote"], "remote")["branch"]))
	o.remember(repo, id, str(identity["login"]))
	record["sync_enabled"] = true
	record["status"] = "pending"
	record["error"] = nil
	return o.sync(id)
}
func (o *operation) importProject(p Object) Object {
	hub, _ := o.hubFor("")
	identity := o.authenticated("")
	repo, e := hub.Repository(o.ctx, str(p["repository"]))
	must(e)
	checkedRepository(repo, false)
	branch := str(repo["default_branch"])
	if branch == "" {
		branch = "main"
	}
	info, e := hub.Manifest(o.ctx, str(repo["full_name"]), branch)
	must(e)
	require(info["format"] == "rumsdlc-project/1", "Not a RumSDLC project repository")
	require(!has(object(o.db["projects"], "projects"), str(info["project_id"])), "Project already exists locally")
	target, e := fsutil.RealPath(str(p["path"]))
	must(e)
	require(!fsutil.Exists(target), "Import destination must not exist")
	o.available(target, "")
	ancestor := filepath.Dir(target)
	for !fsutil.Exists(ancestor) {
		ancestor = filepath.Dir(ancestor)
	}
	o.engine(nil, nil).assertRoot(ancestor, false)
	state := o.engine(nil, hub).importProject(repo, target)
	var record Object
	registrationError := attempt(func() { validateManifest(info, state); record = o.register(target, state) })
	require(registrationError == nil, "Imported files retained at "+target+"; registration failed")
	saveConfig(target, loadConfig(target, true))
	setMetaBinding(record, repo, branch)
	writable := getObject(repo, "permissions")["push"] == true
	record["remote"] = Object{"id": repo["id"], "full_name": repo["full_name"], "branch": branch, "allow_empty": false}
	record["sync_enabled"] = writable
	record["status"] = "read-only-import"
	if writable {
		record["status"] = "synced"
	}
	record["last_head"] = o.engine(nil, nil).head(target)
	record["last_sync"] = fsutil.Now()
	o.remember(repo, str(record["id"]), str(identity["login"]))
	return record
}
func (o *operation) sync(id string) Object {
	record := o.local(id)
	require(truth(record["sync_enabled"]) && record["remote"] != nil, "Project synchronization is not enabled")
	o.migrateIdentity(record)
	var result Object
	err := attempt(func() {
		o.authenticated("")
		hub, _ := o.hubFor("")
		repo := o.boundRepository(record, hub, "")
		result = o.engine(record, hub).sync(record, repo)
		result = o.finishSync(record, result)
		result["project_id"] = id
	})
	if err != nil {
		status := "error"
		if truth(record["conflict"]) {
			status = "conflict"
		}
		record["status"] = status
		record["data_status"] = status
		record["error"] = err.Error()
		must(err)
	}
	return result
}
func (o *operation) finishSync(record, result Object) Object {
	if result["status"] == "conflict" {
		record["data_status"] = "conflict"
		result["data_status"] = "conflict"
	}
	delete(record, "code_status")
	delete(record, "code_error")
	if result["status"] == "synced" {
		record["data_status"] = "synced"
		result["data_status"] = "synced"
		record["status"] = "read-only-import"
		if truth(record["sync_enabled"]) {
			record["status"] = "synced"
		}
	}
	if result["status"] == "synced" && truth(record["workspace"]) {
		compatibility, err := o.checkAgent(record)
		if err != nil {
			record["agent_status"] = "needs-regeneration"
			result["agent_status"] = "needs-regeneration"
			record["agent_error"] = err.Error()
			result["agent_error"] = err.Error()
			record["status"] = "needs-attention"
			result["status"] = "needs-attention"
		} else {
			record["agent_error"] = nil
			customized := compatibility["customized_files"]
			record["agent_customized_files"] = customized
			result["agent_customized_files"] = customized
			status := "ready"
			if truth(customized) {
				status = "needs-review"
				record["status"] = "needs-attention"
				result["status"] = "needs-attention"
			}
			record["agent_status"] = status
			result["agent_status"] = status
		}
	}
	return result
}
func (o *operation) resolve(p Object) Object {
	record := o.local(str(p["project_id"]))
	o.authenticated("")
	hub, _ := o.hubFor("")
	require(truth(record["sync_enabled"]), "Synchronization is disabled")
	repo, e := hub.Repository(o.ctx, str(object(record["remote"], "remote")["full_name"]))
	must(e)
	return o.finishSync(record, o.engine(record, hub).resolve(record, repo, str(p["conflict_id"]), truth(p["accept"])))
}
func (o *operation) conflictRetry(p Object) Object {
	record := o.local(str(p["project_id"]))
	conflict := getObject(record, "conflict")
	require(len(conflict) > 0 && equal(conflict["id"], p["conflict_id"]), "Conflict ID changed")
	previous := []any{}
	if record["previous_conflicts"] != nil {
		previous = array(record["previous_conflicts"], "previous_conflicts")
	}
	record["previous_conflicts"] = append(previous, clone(conflict))
	record["conflict"] = nil
	record["status"] = "pending"
	return o.sync(str(record["id"]))
}
