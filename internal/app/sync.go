package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"sort"
	"strings"
)

const rootFiles = "state.json .rumsdlc-project.json project-config.json AGENTS.md WORKFLOW.md README.md"
const rootDirs = "documents custom workflow profiles templates rules assets references"

func allowed(relative string) bool {
	if relative == "" || strings.HasPrefix(relative, "/") || filepath.IsAbs(relative) {
		return false
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if oneOf(part, ". .. .git .workflow.lock __pycache__") || part == "" {
			return false
		}
	}
	name := parts[len(parts)-1]
	return (oneOf(relative, rootFiles) || len(parts) > 1 && oneOf(parts[0], rootDirs)) && !strings.HasSuffix(name, ".tmp") && !strings.HasSuffix(name, ".pyc") && !strings.HasSuffix(name, ".pyo")
}
func syncFiles(root string) []string {
	result := []string{}
	for _, name := range strings.Fields(rootFiles + " " + rootDirs) {
		start := filepath.Join(root, name)
		if !fsutil.Exists(start) {
			if i, e := os.Lstat(start); e == nil {
				require(i.Mode()&os.ModeSymlink == 0, "Symlinks cannot be synchronized: "+start)
			}
			continue
		}
		must(filepath.WalkDir(start, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return &syncPathError{path}
			}
			if !entry.IsDir() {
				relative, e := filepath.Rel(root, path)
				if e != nil {
					return e
				}
				relative = filepath.ToSlash(relative)
				if allowed(relative) && fsutil.IsFile(path) {
					result = append(result, relative)
				}
			}
			return nil
		}))
	}
	sort.Strings(result)
	return result
}

type syncPathError struct{ path string }

func (e *syncPathError) Error() string { return "Symlinks cannot be synchronized: " + e.path }
func genesis(state Object) string      { return fingerprint(array(state["history"], "history")[0]) }
func manifestFor(state Object) Object {
	return Object{"format": "rumsdlc-project/1", "project_id": object(state["project"], "Project")["id"], "genesis": genesis(state), "name": object(state["project"], "Project")["name"]}
}
func validateManifest(value, state Object) {
	validateManifestIdentity(value)
	require(value["project_id"] == object(state["project"], "Project")["id"] && value["genesis"] == genesis(state), "Project manifest identity/genesis does not match the Store")
}
func (e *syncEngine) ensure(root string, state Object) {
	if fsutil.Exists(filepath.Join(root, ".git")) {
		e.assertRoot(root, false)
	} else {
		e.assertRoot(root, true)
	}
	path := filepath.Join(root, Manifest)
	if fsutil.Exists(path) {
		value, err := fsutil.ReadJSON(path)
		must(err)
		validateManifest(value, state)
		if !equal(value, manifestFor(state)) {
			must(fsutil.WriteJSON(path, manifestFor(state)))
		}
	} else {
		must(fsutil.WriteJSON(path, manifestFor(state)))
	}
	if e.record != nil {
		attachAuthor(e.ctx, e.record)
	}
	e.assertTracked(root)
}
func (e *syncEngine) assertTracked(root string) {
	for _, item := range strings.Split(e.git(root, true, "ls-files", "--stage", "-z").Stdout, "\x00") {
		if item == "" {
			continue
		}
		header, path, ok := strings.Cut(item, "\t")
		require(ok, "Invalid tracked entry")
		mode := strings.Fields(header)[0]
		require(mode == "100644" || mode == "100755", "Symlink/submodule in management repository")
		require(allowed(path), "Unapproved file tracked in management repository: "+path)
	}
}
func (e *syncEngine) commit(root, message string, checkConflicts bool) any {
	e.assertTracked(root)
	all := map[string]bool{}
	for _, path := range syncFiles(root) {
		all[path] = true
	}
	for _, path := range strings.Split(e.git(root, true, "ls-files", "-z").Stdout, "\x00") {
		if path != "" {
			all[path] = true
		}
	}
	paths := []string{}
	for path := range all {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) > 0 {
		e.git(root, true, append([]string{"add", "--all", "--"}, paths...)...)
	}
	if checkConflicts {
		e.git(root, true, "-c", "core.whitespace=-blank-at-eol,-blank-at-eof,-space-before-tab", "diff", "--cached", "--check")
	}
	merging := e.git(root, false, "rev-parse", "--verify", "MERGE_HEAD").Code == 0
	if merging || e.git(root, false, "diff", "--cached", "--quiet").Code != 0 {
		e.git(root, true, "commit", "-q", "-m", message)
	}
	return e.head(root)
}
func (e *syncEngine) bindRemote(root string, repo Object) {
	url, err := e.hub.URL(repo)
	must(err)
	existing := e.git(root, false, "remote", "get-url", "origin")
	if existing.Code == 0 {
		require(strings.TrimSpace(existing.Stdout) == url, "Existing origin differs; refusing to change it")
	} else {
		e.git(root, true, "remote", "add", "origin", url)
	}
	fetch := strings.Fields(e.git(root, true, "remote", "get-url", "--all", "origin").Stdout)
	push := strings.Fields(e.git(root, true, "remote", "get-url", "--push", "--all", "origin").Stdout)
	require(len(fetch) == 1 && fetch[0] == url && len(push) == 1 && push[0] == url, "Origin fetch/push URLs differ from the pinned management repository")
}
func (e *syncEngine) remoteHead(root, branch string) any {
	return contract(e.ctx, "sync.remote_head", []any{root, branch}, func() any { return e.remoteHeadNative(root, branch) })
}
func (e *syncEngine) remoteHeadNative(root, branch string) any {
	require(branch != "" && !strings.HasPrefix(branch, "-"), "Invalid remote branch")
	e.git(root, true, "check-ref-format", "refs/heads/"+branch)
	remote := strings.TrimSpace(e.git(root, true, "ls-remote", "--heads", "origin", "refs/heads/"+branch).Stdout)
	if remote == "" {
		return nil
	}
	e.git(root, true, "fetch", "--no-tags", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch)
	return strings.TrimSpace(e.git(root, true, "rev-parse", "refs/remotes/origin/"+branch).Stdout)
}
func (e *syncEngine) validateConfiguration(root string) Object {
	cfg := loadConfig(root, true)
	if e.record != nil && e.record["remote"] != nil {
		e.checkHistoricalMetaBinding(root, object(e.record["remote"], "remote"))
	}
	return cfg
}
func (e *syncEngine) validateTree(root, id, expectedGenesis string) Object {
	e.assertTracked(root)
	state := readState(root)
	value, err := fsutil.ReadJSON(filepath.Join(root, Manifest))
	must(err)
	validateManifest(value, state)
	if id != "" {
		require(object(state["project"], "Project")["id"] == id, "Remote Store has a different Project ID")
	}
	if expectedGenesis != "" {
		require(genesis(state) == expectedGenesis, "Remote has unrelated initialization history")
	}
	e.validateConfiguration(root)
	return state
}
func (e *syncEngine) validateLocal(root, id string) Object {
	state := readState(root)
	require(object(state["project"], "Project")["id"] == id, "Local Project identity changed")
	e.validateConfiguration(root)
	if fsutil.Exists(filepath.Join(root, ".git")) {
		e.assertRoot(root, false)
		value, err := fsutil.ReadJSON(filepath.Join(root, Manifest))
		must(err)
		validateManifest(value, state)
	} else if e.record != nil && e.record["remote"] != nil {
		require(truth(object(e.record["remote"], "remote")["allow_empty"]) && !truth(e.record["last_head"]), "Management Git directory is missing; restore it before synchronization")
	}
	return state
}
func (e *syncEngine) candidate(root, remote string) (string, string) {
	base := filepath.Join(e.home, "candidates")
	must(os.MkdirAll(base, 0700))
	directory, err := os.MkdirTemp(base, "merge-")
	must(err)
	work := filepath.Join(directory, "store")
	e.git(directory, true, "clone", "-q", "--no-local", "--no-hardlinks", root, work)
	e.git(work, true, "fetch", "-q", root, remote)
	return directory, work
}
func snapshotHash(root string) string {
	hashes := Object{}
	for _, path := range syncFiles(root) {
		raw, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		must(e)
		hashes[path] = jsonvalue.HashBytes(raw)
	}
	return fingerprint(hashes)
}
func (e *syncEngine) updateLive(root, candidate string, expected any, snapshot string) {
	require(equal(e.head(root), expected) && snapshotHash(root) == snapshot, "Live files changed during synchronization; retry without overwriting them")
	require(strings.TrimSpace(e.git(root, true, "status", "--porcelain", "--untracked-files=no").Stdout) == "", "Live tracked files changed during synchronization")
	e.git(root, true, "fetch", "-q", candidate, "HEAD")
	e.git(root, true, "merge", "--ff-only", "--no-edit", "FETCH_HEAD")
}
func withStoreLock(root string, fn func() Object) Object {
	s, err := store.New(root)
	must(err)
	var result Object
	must(s.WithLock(func() (err error) { defer guard(&err); result = fn(); return nil }))
	return result
}
func (e *syncEngine) sync(record, repo Object) Object {
	checkedRepository(repo, true)
	binding := object(record["remote"], "remote")
	require(equal(repo["id"], binding["id"]), "Repository identity changed")
	root, id := str(record["path"]), str(record["id"])
	require(!truth(record["conflict"]), "Resolve the pending conflict before synchronization")
	return withStoreLock(root, func() Object {
		state := e.validateLocal(root, id)
		e.ensure(root, state)
		e.bindRemote(root, repo)
		local := e.commit(root, "RumSDLC: checkpoint "+id, false)
		snapshot := snapshotHash(root)
		branch := str(binding["branch"])
		remote := e.remoteHead(root, branch)
		if remote == nil {
			require(binding["allow_empty"] == true, "Remote branch disappeared; refusing to recreate it automatically")
			e.git(root, true, "push", "origin", "HEAD:refs/heads/"+branch)
		} else if !equal(remote, local) {
			conflict := e.mergeCandidate(root, id, state, record, local, remote, snapshot)
			if conflict != nil {
				return conflict
			}
			e.git(root, true, "push", "origin", "HEAD:refs/heads/"+branch)
		}
		binding["allow_empty"] = false
		record["status"] = "synced"
		record["last_sync"] = fsutil.Now()
		record["last_head"] = e.head(root)
		record["error"] = nil
		return Object{"status": "synced", "head": record["last_head"]}
	})
}
func (e *syncEngine) mergeCandidate(root, id string, state, record Object, local, remote any, snapshot string) Object {
	directory, work := e.candidate(root, str(remote))
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(directory)
		}
	}()
	e.git(work, true, "checkout", "-q", "--detach", str(remote))
	e.validateTree(work, id, genesis(state))
	issue := ""
	behind := e.git(work, false, "merge-base", "--is-ancestor", str(local), str(remote)).Code == 0
	ahead := e.git(work, false, "merge-base", "--is-ancestor", str(remote), str(local)).Code == 0
	if !behind {
		e.git(work, true, "checkout", "-q", "--detach", str(local))
		if !ahead {
			merged := e.git(work, false, "merge", "--no-commit", "--no-ff", "--allow-unrelated-histories", str(remote))
			unresolved := []string{}
			for _, path := range strings.Split(strings.TrimSpace(e.git(work, true, "diff", "--name-only", "--diff-filter=U").Stdout), "\n") {
				if path != "" {
					unresolved = append(unresolved, path)
				}
			}
			remaining := []string{}
			for _, path := range unresolved {
				if path != "state.json" {
					remaining = append(remaining, path)
					continue
				}
				err := attempt(func() {
					leftRaw := e.git(work, true, "show", str(local)+":state.json").Stdout
					rightRaw := e.git(work, true, "show", str(remote)+":state.json").Stdout
					left, err := jsonvalue.Decode([]byte(leftRaw))
					must(err)
					right, err := jsonvalue.Decode([]byte(rightRaw))
					must(err)
					must(fsutil.WriteJSON(filepath.Join(work, "state.json"), mergeState(object(left, "state"), object(right, "state"))))
					e.git(work, true, "add", "--", "state.json")
				})
				if err != nil {
					remaining = append(remaining, path)
					issue = "Business merge requires review: " + err.Error()
				}
			}
			if len(remaining) > 0 || (merged.Code != 0 && e.git(work, false, "rev-parse", "--verify", "MERGE_HEAD").Code != 0) {
				if issue == "" {
					issue = "Concurrent edits require resolution"
				}
			}
			if issue == "" {
				if err := attempt(func() { e.validateTree(work, id, genesis(state)); e.commit(work, "RumSDLC: merge "+id, false) }); err != nil {
					issue = "Merged project is invalid: " + err.Error()
				}
			}
		}
	}
	if issue != "" {
		keep = true
		files := []any{}
		for _, path := range strings.Split(e.git(work, true, "diff", "--name-only", "--diff-filter=U", "-z").Stdout, "\x00") {
			if path != "" {
				files = append(files, path)
			}
		}
		conflict := Object{"id": filepath.Base(directory), "path": work, "local": local, "remote": remote, "reason": issue, "created_at": fsutil.Now(), "files": files}
		record["conflict"] = conflict
		record["status"] = "conflict"
		record["data_status"] = "conflict"
		return Object{"status": "conflict", "conflict": conflict}
	}
	if truth(record["repository_rename"]) {
		current := loadConfig(work, true)
		meta := object(object(current["repositories"], "repositories")["meta"], "meta")
		binding := object(record["remote"], "remote")
		if meta["url"] != "https://github.com/"+str(binding["full_name"])+".git" {
			refreshImportedMetaBinding(work, binding)
			e.validateTree(work, id, genesis(state))
			e.commit(work, "RumSDLC: refresh verified repository name "+id, false)
		}
	}
	e.updateLive(root, work, local, snapshot)
	return nil
}
func (e *syncEngine) resolve(record, repo Object, id string, accept bool) Object {
	require(accept, "Explicit acceptance of the repaired candidate is required")
	conflict := getObject(record, "conflict")
	require(len(conflict) > 0 && conflict["id"] == id, "Conflict ID changed")
	checkedRepository(repo, true)
	binding := object(record["remote"], "remote")
	require(equal(repo["id"], binding["id"]), "Repository identity changed")
	root, work := str(record["path"]), str(conflict["path"])
	return withStoreLock(root, func() Object {
		e.validateLocal(root, str(record["id"]))
		require(equal(e.head(root), conflict["local"]), "Local branch advanced; candidate is stale")
		e.bindRemote(root, repo)
		remote := e.remoteHead(root, str(binding["branch"]))
		require(equal(remote, conflict["remote"]), "Remote branch advanced; candidate is stale")
		require(strings.TrimSpace(e.git(root, true, "status", "--porcelain", "--untracked-files=no").Stdout) == "", "Local files changed; candidate is stale")
		state := readState(root)
		e.validateTree(work, str(record["id"]), genesis(state))
		e.commit(work, "RumSDLC: accepted conflict resolution "+str(record["id"]), true)
		e.updateLive(root, work, conflict["local"], snapshotHash(root))
		record["conflict"] = nil
		record["status"] = "pending-push"
		record["data_status"] = "pending-push"
		e.git(root, true, "push", "origin", "HEAD:refs/heads/"+str(binding["branch"]))
		record["status"] = "synced"
		record["last_sync"] = fsutil.Now()
		record["last_head"] = e.head(root)
		record["error"] = nil
		return Object{"status": "synced", "head": record["last_head"]}
	})
}
func (e *syncEngine) importProject(repo Object, target string) Object {
	checkedRepository(repo, false)
	require(!fsutil.Exists(target), "Import target must not exist; no local files will be overwritten")
	must(os.MkdirAll(filepath.Dir(target), 0700))
	stage, err := os.MkdirTemp(filepath.Dir(target), ".rumsdlc-import-")
	must(err)
	defer os.RemoveAll(stage)
	work := filepath.Join(stage, "store")
	branch := str(repo["default_branch"])
	if branch == "" {
		branch = "main"
	}
	require(!strings.HasPrefix(branch, "-"), "Invalid remote branch")
	e.git(stage, true, "check-ref-format", "refs/heads/"+branch)
	url, err := e.hub.URL(repo)
	must(err)
	e.git(stage, true, "clone", "-q", "--no-checkout", "--no-tags", "--single-branch", "--branch", branch, url, work)
	for _, item := range strings.Split(e.git(work, true, "ls-tree", "-r", "-z", "HEAD").Stdout, "\x00") {
		if item == "" {
			continue
		}
		header, path, ok := strings.Cut(item, "\t")
		require(ok, "Invalid Git tree entry")
		mode := strings.Fields(header)[0]
		require((mode == "100644" || mode == "100755") && allowed(path), "Unapproved path or mode in remote project: "+path)
	}
	e.git(work, true, "checkout", "-q", "HEAD", "--", ".")
	state := e.validateTree(work, "", "")
	remote := copyObject(repo)
	remote["branch"] = branch
	refreshImportedMetaBinding(work, remote)
	checkMetaBinding(work, remote)
	must(os.Rename(work, target))
	return state
}
