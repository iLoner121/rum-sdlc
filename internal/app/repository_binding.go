package app

import (
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"strings"
)

// boundRepository follows a rename only after checking both remote and workflow
// identity. A recycled old name never becomes a new project binding.
func (o *operation) boundRepository(record Object, hub Hub, requested string) Object {
	remote := object(record["remote"], "remote")
	root := str(record["path"])
	checkMetaBinding(root, remote)
	lookup := requested
	if lookup == "" {
		lookup = str(remote["full_name"])
	}
	repo, err := hub.Repository(o.ctx, lookup)
	if err != nil || !equal(repo["id"], remote["id"]) {
		require(requested == "" || requested == remote["full_name"], "Selected repository differs from the pinned repository identity")
		candidates, e := hub.Repositories(o.ctx)
		must(e)
		found := []Object{}
		for _, candidate := range candidates {
			if equal(candidate["id"], remote["id"]) {
				found = append(found, candidate)
			}
		}
		require(len(found) == 1, "Pinned repository could not be uniquely located; check access and repository identity")
		repo = found[0]
	}
	checkedRepository(repo, true)
	require(equal(repo["id"], remote["id"]), "Repository identity changed")
	if repo["full_name"] == remote["full_name"] {
		return repo
	}
	manifest, e := hub.Manifest(o.ctx, str(repo["full_name"]), str(remote["branch"]))
	must(e)
	withStoreLock(root, func() Object {
		state := readState(root)
		validateManifest(manifest, state)
		checkMetaBinding(root, remote)
		engine := o.engine(record, hub)
		nextURL, e := hub.URL(repo)
		must(e)
		oldURL := "https://github.com/" + str(remote["full_name"]) + ".git"
		exists := engine.git(root, false, "remote", "get-url", "origin").Code == 0
		original := ""
		explicitPush := false
		if exists {
			fetch := strings.Fields(engine.git(root, true, "remote", "get-url", "--all", "origin").Stdout)
			push := strings.Fields(engine.git(root, true, "remote", "get-url", "--push", "--all", "origin").Stdout)
			require(len(fetch) == 1 && len(push) == 1 && fetch[0] == push[0] && (fetch[0] == oldURL || fetch[0] == nextURL), "Origin differs from the verified old/new repository; refusing rename recovery")
			original = fetch[0]
			explicitPush = engine.git(root, false, "config", "--get-all", "remote.origin.pushurl").Code == 0
		}
		changed, complete := false, false
		defer func() {
			if changed && !complete {
				codeGit(o.ctx, root, "remote", "set-url", "origin", original)
				if explicitPush {
					codeGit(o.ctx, root, "remote", "set-url", "--push", "origin", original)
				}
			}
		}()
		if exists && original != nextURL {
			engine.git(root, true, "remote", "set-url", "origin", nextURL)
			changed = true
			if explicitPush {
				engine.git(root, true, "remote", "set-url", "--push", "origin", nextURL)
			}
		}
		setMetaBinding(record, repo, str(remote["branch"]))
		updated := copyObject(remote)
		updated["full_name"] = repo["full_name"]
		record["remote"] = updated
		record["repository_rename"] = Object{"from": remote["full_name"], "to": repo["full_name"], "repository_id": repo["id"], "project_id": state["project"].(Object)["id"], "verified_at": fsutil.Now()}
		complete = true
		return Object{}
	})
	return repo
}

// Imported historical snapshots may still contain the repository's former name.
// Only normalize a canonical GitHub management URL with the same pinned numeric
// identity and branch, after the caller has verified the remote manifest.
func refreshImportedMetaBinding(root string, remote Object) {
	cfg := loadConfig(root, true)
	meta := object(object(cfg["repositories"], "repositories")["meta"], "meta")
	raw, e := fsutil.ReadJSON(filepath.Join(root, configFile))
	must(e)
	if raw["format"] == "rum-workspace/1" {
		meta["id"], meta["branch"], meta["url"] = remote["id"], remote["branch"], "https://github.com/"+str(remote["full_name"])+".git"
		saveConfig(root, cfg)
		return
	}
	require(equal(meta["id"], remote["id"]) && equal(meta["branch"], remote["branch"]), "Project meta repository identity/branch mismatch")
	old := str(meta["url"])
	name := strings.TrimSuffix(strings.TrimPrefix(old, "https://github.com/"), ".git")
	require(old == "https://github.com/"+name+".git", "Invalid historical management repository URL")
	must(fullName(name))
	require(strings.HasPrefix(strings.SplitN(name, "/", 2)[1], RepositoryPrefix), "Historical management repository prefix is missing")
	meta["url"] = "https://github.com/" + str(remote["full_name"]) + ".git"
	saveConfig(root, cfg)
}
func (e *syncEngine) checkHistoricalMetaBinding(root string, remote Object) {
	if err := attempt(func() { checkMetaBinding(root, remote) }); err != nil {
		require(e.record != nil && root != str(e.record["path"]), "Project meta binding differs from the pinned repository")
		rename := getObject(e.record, "repository_rename")
		require(equal(rename["repository_id"], remote["id"]) && rename["to"] == remote["full_name"], "Project meta binding differs from the pinned repository")
		old := copyObject(remote)
		old["full_name"] = rename["from"]
		checkMetaBinding(root, old)
	}
}
