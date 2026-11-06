package app

import (
	"context"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/process"
	"strings"
)

func codeGit(ctx context.Context, root string, args ...string) process.Result {
	result, e := (process.Runner{}).Run(ctx, append([]string{"git", "-C", root}, args...), "", "", process.CodeEnvironment(), 0, false)
	must(e)
	return result
}
func codeGitChecked(ctx context.Context, root string, args ...string) process.Result {
	result := codeGit(ctx, root, args...)
	require(result.Code == 0, "git failed: "+strings.TrimSpace(result.Stderr))
	return result
}
func sharedPath(record Object) string {
	if truth(record["meta_identity_config"]) {
		return str(record["meta_identity_config"])
	}
	if truth(record["workspace"]) {
		return filepath.Join(str(record["workspace"]), ".rum-gitconfig")
	}
	return str(record["identity_config"])
}
func attachAuthor(ctx context.Context, record Object) {
	if !truth(record["workspace"]) && !truth(record["identity_config"]) && !truth(record["meta_identity_config"]) {
		return
	}
	shared := sharedPath(record)
	if !fsutil.Exists(shared) {
		return
	}
	root := str(record["path"])
	if !fsutil.Exists(filepath.Join(root, ".git")) {
		return
	}
	existing := codeGit(ctx, root, "config", "--local", "--get-all", "include.path").Stdout
	for _, line := range strings.Split(strings.TrimSpace(existing), "\n") {
		if line == shared {
			return
		}
	}
	codeGitChecked(ctx, root, "config", "--local", "--add", "include.path", shared)
}
func configureAuthor(ctx context.Context, record Object, home string, p Object) {
	name, email := p["name"], p["email"]
	inherit := truth(p["inherit"])
	require(inherit || (name == nil) == (email == nil), "Supply both --name and --email")
	require(!(inherit && (truth(name) || truth(email))), "--inherit cannot be combined with an explicit author")
	for _, v := range []any{name, email} {
		if v != nil {
			s := text(v, "Git author")
			require(!strings.ContainsAny(s, "\r\n\x00"), "Invalid Git author")
		}
	}
	if !has(record, "meta_identity_config") {
		record["meta_identity_config"] = filepath.Join(home, "identities", str(record["id"])+".author.config")
	}
	shared := sharedPath(record)
	must(os.MkdirAll(filepath.Dir(shared), 0700))
	if i, e := os.Lstat(shared); e == nil {
		require(i.Mode()&os.ModeSymlink == 0, "Shared identity configuration cannot be a symlink")
	}
	if !fsutil.Exists(shared) {
		must(os.WriteFile(shared, nil, 0600))
	}
	if inherit || name != nil {
		root := str(record["path"])
		if fsutil.Exists(filepath.Join(root, ".git")) {
			for _, key := range []string{"name", "email"} {
				codeGit(ctx, root, "config", "--local", "--unset-all", "user."+key)
			}
		}
		for key, value := range map[string]any{"name": name, "email": email} {
			_, e := (process.Runner{}).Run(ctx, []string{"git", "config", "--file", shared, "--unset-all", "user." + key}, "", "", nil, 0, false)
			must(e)
			if value != nil {
				_, e = (process.Runner{}).Run(ctx, []string{"git", "config", "--file", shared, "user." + key, str(value)}, "", "", nil, 0, true)
				must(e)
			}
		}
	}
	attachAuthor(ctx, record)
}
func effectiveAuthor(ctx context.Context, record Object, required bool) Object {
	root := str(record["path"])
	extra := []string{}
	if truth(record["meta_identity_config"]) {
		extra = append(extra, "-c", "include.path="+str(record["meta_identity_config"]))
	}
	result := Object{}
	for _, key := range []string{"name", "email"} {
		args := append(append([]string{}, extra...), "config", "--get", "user."+key)
		result["commit_"+key] = strings.TrimSpace(codeGit(ctx, root, args...).Stdout)
	}
	require(!required || (truth(result["commit_name"]) && truth(result["commit_email"])), "Git author is missing; configure global Git or meta author with project-identity")
	return result
}

type syncEngine struct {
	ctx              context.Context
	home             string
	hub              Hub
	identity, record Object
	runner           process.Runner
}

func (o *operation) engine(record Object, hub Hub) *syncEngine {
	if hub == nil {
		hub = o.device.Hub
	}
	identity := Object{"commit_name": "", "commit_email": ""}
	if record != nil {
		identity = effectiveAuthor(o.ctx, record, true)
	}
	must(os.MkdirAll(filepath.Join(o.device.Home, "empty-hooks"), 0700))
	return &syncEngine{ctx: o.ctx, home: o.device.Home, hub: hub, identity: identity, record: record, runner: o.device.Runner}
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func (e *syncEngine) git(root string, check bool, args ...string) process.Result {
	return contract(e.ctx, "sync.git", []any{root, args, check}, func() process.Result { return e.gitNative(root, check, args...) })
}
func (e *syncEngine) gitNative(root string, check bool, args ...string) process.Result {
	exe, err := os.Executable()
	must(err)
	helper := "!" + shellQuote(exe) + " credential"
	if e.hub.Login() != "" {
		helper += " --login " + shellQuote(e.hub.Login())
	}
	command := []string{"git", "-c", "core.hooksPath=" + filepath.Join(e.home, "empty-hooks"), "-c", "core.fsmonitor=false", "-c", "core.autocrlf=false", "-c", "core.attributesFile=" + os.DevNull, "-c", "commit.gpgSign=false", "-c", "merge.autoStash=false", "-c", "credential.https://github.com.helper=", "-c", "credential.https://github.com.helper=" + helper, "-c", "credential.https://github.com.username=" + e.hub.Login(), "-c", "user.name=" + str(e.identity["commit_name"]), "-c", "user.email=" + str(e.identity["commit_email"]), "-C", root}
	env := process.Environment(true)
	for _, key := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"} {
		value, ok := os.LookupEnv(key)
		if ok {
			env["RUMSDLC_CREDENTIAL_"+key] = value
		} else {
			env["RUMSDLC_CREDENTIAL_"+key] = ""
		}
	}
	result, err := e.runner.Run(e.ctx, append(command, args...), "", "", env, 0, check)
	must(err)
	return result
}
func (e *syncEngine) head(root string) any {
	result := e.git(root, false, "rev-parse", "--verify", "HEAD")
	if result.Code == 0 {
		return strings.TrimSpace(result.Stdout)
	}
	return nil
}
func (e *syncEngine) assertRoot(root string, initialize bool) bool {
	result := e.git(root, false, "rev-parse", "--show-toplevel")
	if result.Code == 0 {
		actual, err := filepath.EvalSymlinks(strings.TrimSpace(result.Stdout))
		must(err)
		expected, err := filepath.EvalSymlinks(root)
		must(err)
		require(actual == expected, "Store is inside another Git repository; use a separate management directory")
		info, err := os.Lstat(filepath.Join(root, ".git"))
		require(err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0, "Linked worktrees and external Git directories are not management Stores")
		require(fsutil.IsFile(filepath.Join(root, Manifest)), "Existing Git repository is not marked as a RumSDLC Store")
	} else if initialize {
		e.git(root, true, "init", "-q", "--initial-branch=main")
	} else {
		return false
	}
	return true
}
