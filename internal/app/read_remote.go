package app

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/process"
	"rumsdlc/internal/store"
	"strings"
	"time"
)

// ReadRemoteProjects discovers metadata using GET requests and an existing
// credential. It deliberately bypasses Device.Call, its leases and registry CAS.
func (r *Reader) ReadRemoteProjects(ctx context.Context) (view View, err error) {
	defer guard(&err)
	if ctx.Err() != nil {
		return View{}, discoveryError(ctx, ctx.Err())
	}
	db, err := r.ReadRegistry()
	if err != nil {
		return View{}, err
	}
	policy, ok := db["meta_auth"].(Object)
	if !ok {
		return View{}, NewReadError("identity-invalid", "设备管理账号策略无效")
	}
	if e := attempt(func() { validateAuth(policy) }); e != nil {
		return View{}, NewReadError("identity-invalid", "设备管理账号策略无效")
	}
	login := str(policy["login"])
	hub := r.RemoteHub
	credentialSource := "fixture"
	if hub == nil {
		token, source, e := readDiscoveryCredential(ctx, login)
		if e != nil {
			return View{}, e
		}
		g := NewGitHub()
		g.Account, g.readToken = login, token
		hub, credentialSource = g, source
	} else if login != "" {
		hub = hub.ForLogin(login)
	}
	identity, e := hub.User(ctx)
	if e != nil {
		return View{}, discoveryError(ctx, e)
	}
	if ctx.Err() != nil {
		return View{}, discoveryError(ctx, ctx.Err())
	}
	if str(identity["login"]) == "" || idString(identity["id"]) == "" {
		return View{}, NewReadError("authentication", "GitHub 未返回可确认的账号身份")
	}
	if login != "" && !strings.EqualFold(login, str(identity["login"])) {
		return View{}, NewReadError("account-changed", "当前凭据账号与设备管理账号不一致；请通过已有 CLI 核对")
	}
	repositories, e := hub.Repositories(ctx)
	if e != nil {
		return View{}, discoveryError(ctx, e)
	}
	if ctx.Err() != nil {
		return View{}, discoveryError(ctx, ctx.Err())
	}
	rows, verified := []any{}, 0
	readAt := fsutil.Now()
	for _, repo := range repositories {
		if ctx.Err() != nil {
			return View{}, discoveryError(ctx, ctx.Err())
		}
		if !strings.HasPrefix(str(repo["name"]), RepositoryPrefix) {
			continue
		}
		branch := str(repo["default_branch"])
		if branch == "" {
			branch = "main"
		}
		row := Object{"repository_id": repo["id"], "name": repo["name"], "full_name": repo["full_name"], "url": nil,
			"display_name": repo["name"], "project_id": nil, "manifest_status": "unavailable", "branch": branch,
			"private": repo["private"], "archived": truth(repo["archived"]), "local": Object{"status": "not-imported"}, "code": unknownRemoteCode(readAt)}
		rows = append(rows, row)
		name := str(repo["full_name"])
		if e := fullName(name); e != nil {
			row["manifest_status"], row["error"] = "invalid", Object{"code": "identity-invalid", "message": "仓库身份格式无效"}
			continue
		}
		row["url"] = "https://github.com/" + name
		manifest, e := hub.Manifest(ctx, name, branch)
		if e != nil {
			if ctx.Err() != nil {
				return View{}, discoveryError(ctx, ctx.Err())
			}
			row["manifest_status"] = manifestReadKind(e)
			if manifestReadKind(e) == "invalid" {
				row["error"] = Object{"code": "identity-invalid", "message": "项目身份文件无效"}
			} else if manifestReadKind(e) == "missing" {
				row["error"] = Object{"code": "identity-invalid", "message": "尚无项目身份文件"}
			} else {
				row["error"] = discoveryErrorObject(ctx, e)
			}
			continue
		}
		if e := attempt(func() { validateManifestIdentity(manifest) }); e != nil {
			row["manifest_status"], row["error"] = "invalid", Object{"code": "identity-invalid", "message": "项目身份文件格式无效"}
			continue
		}
		verified++
		row["manifest_status"], row["project_id"], row["genesis"] = "valid", manifest["project_id"], manifest["genesis"]
		if truth(manifest["name"]) {
			row["display_name"] = manifest["name"]
		}
		if configReader, ok := hub.(remoteConfigReader); ok {
			if cfg, e := configReader.ProjectConfig(ctx, name, branch); e == nil {
				row["code"] = remoteCode(cfg, "remote-project-config", readAt)
				if !truth(manifest["name"]) && truth(cfg["name"]) {
					row["display_name"] = cfg["name"]
				}
			}
		}
		if ctx.Err() != nil {
			return View{}, discoveryError(ctx, ctx.Err())
		}
		local, cfg := matchRemoteLocal(db, repo, manifest)
		row["local"] = local
		if local["status"] == "available" && cfg != nil {
			row["code"] = remoteCode(cfg, "local-project-config", readAt)
			if !truth(manifest["name"]) && truth(cfg["name"]) {
				row["display_name"] = cfg["name"]
			}
		}
	}
	data := Object{"account": Object{"id": identity["id"], "login": identity["login"], "credential_source": credentialSource},
		"scope": "当前凭据有权访问、名称以 RumSDLC_ 开头的管理仓库", "rows": rows, "total": len(rows), "verified_total": verified}
	return NewView("", nil, "remote-discovery", data), nil
}

// No credential helper is invoked: arbitrary helpers can refresh and persist
// credentials even with interactive prompts disabled.
func readDiscoveryCredential(ctx context.Context, login string) (string, string, error) {
	if login == "" {
		for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value, key, nil
			}
		}
	}
	if gh, e := exec.LookPath("gh"); e == nil {
		env := process.Environment(true)
		for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_DEBUG"} {
			delete(env, key)
		}
		args := []string{gh, "auth", "token", "--hostname", "github.com"}
		if login != "" {
			args = append(args, "--user", login)
		}
		result, e := (process.Runner{}).Run(ctx, args, "", "", env, 20*time.Second, false)
		if ctx.Err() != nil {
			return "", "", discoveryError(ctx, ctx.Err())
		}
		if e == nil && result.Code == 0 && strings.TrimSpace(result.Stdout) != "" {
			return strings.TrimSpace(result.Stdout), "gh-existing-login", nil
		}
	}
	return "", "", NewReadError("authentication", "没有可只读使用的 GitHub 凭据；请在现有 CLI 配置环境凭据或 gh 登录")
}

type remoteConfigReader interface {
	ProjectConfig(context.Context, string, string) (Object, error)
}

func (g *GitHub) ProjectConfig(ctx context.Context, name, branch string) (result Object, err error) {
	defer guard(&err)
	must(fullName(name))
	v, e := g.api(ctx, "repos/"+name+"/contents/"+configFile+"?ref="+url.QueryEscape(branch), "GET", nil)
	must(e)
	file := object(v, "Project configuration response")
	require(file["type"] == "file" && file["encoding"] == "base64", "Invalid project configuration response")
	raw, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(str(file["content"]), "\n", ""))
	must(e)
	v, e = jsonvalue.Decode(raw)
	must(e)
	return validateConfig(object(v, "Project configuration")), nil
}

func unknownRemoteCode(at string) Object {
	return Object{"association": "unknown", "availability": "unknown", "source": "no-verified-configuration", "read_at": at}
}

func remoteCode(cfg Object, source, at string) Object {
	code := getObject(getObject(cfg, "repositories"), "code")
	association := "unconfigured"
	if str(code["kind"]) == "directory" {
		association = "directory"
	} else if truth(code["url"]) {
		association = "git"
	}
	return Object{"association": association, "availability": "unknown", "source": source, "read_at": at}
}

func matchRemoteLocal(db, repo, manifest Object) (result, cfg Object) {
	result = Object{"status": "not-imported"}
	for pid, candidate := range getObject(db, "projects") {
		record, ok := candidate.(Object)
		if !ok || pid == str(manifest["project_id"]) {
			continue
		}
		if remote, ok := record["remote"].(Object); ok && remote["id"] != nil && idString(remote["id"]) == idString(repo["id"]) {
			return Object{"status": "identity-conflict", "reason": "该管理仓库已登记为另一本机项目"}, nil
		}
	}
	value := getObject(db, "projects")[str(manifest["project_id"])]
	if value == nil {
		return result, nil
	}
	record, ok := value.(Object)
	if !ok {
		return Object{"status": "identity-conflict", "reason": "本机登记格式无效"}, nil
	}
	root := str(record["path"])
	if root == "" {
		return Object{"status": "path-unavailable", "reason": "本机管理工程路径不可用"}, nil
	}
	s, e := store.New(root)
	if e != nil {
		return Object{"status": "path-unavailable", "reason": "本机管理工程路径不可读"}, nil
	}
	state, e := s.Read(false)
	if e != nil {
		return Object{"status": "path-unavailable", "reason": "本机管理工程数据不可读"}, nil
	}
	if e := attempt(func() { validateManifest(manifest, state) }); e != nil {
		return Object{"status": "identity-conflict", "reason": "本机项目 ID 或 genesis 与远端不符"}, nil
	}
	boundID := ""
	if remote, ok := record["remote"].(Object); ok {
		boundID = idString(remote["id"])
		if boundID != "" && boundID != idString(repo["id"]) {
			return Object{"status": "identity-conflict", "reason": "本机已绑定另一管理仓库"}, nil
		}
	}
	// Configuration failures do not make a verified local Store unavailable.
	if e := attempt(func() { cfg = loadConfig(s.Root, false) }); e != nil {
		cfg = nil
	}
	if cfg != nil {
		meta := getObject(getObject(cfg, "repositories"), "meta")
		if meta["id"] != nil && idString(meta["id"]) != idString(repo["id"]) {
			return Object{"status": "identity-conflict", "reason": "本机配置绑定另一管理仓库"}, nil
		}
		if boundID == "" && meta["id"] == nil && truth(meta["url"]) {
			configured := strings.TrimSuffix(strings.TrimRight(str(meta["url"]), "/"), ".git")
			if !strings.EqualFold(configured, "https://github.com/"+str(repo["full_name"])) {
				return Object{"status": "identity-conflict", "reason": "本机配置的管理仓库地址与发现结果不符"}, nil
			}
		}
	}
	return Object{"status": "available", "project_id": manifest["project_id"], "path": filepath.Clean(root)}, cfg
}

func discoveryError(ctx context.Context, err error) error {
	code, message := "network", "GitHub 只读查询失败，请检查网络后重试"
	var readErr *ReadError
	if errors.As(err, &readErr) {
		return readErr
	}
	var status *githubStatusError
	if errors.As(err, &status) {
		switch status.Code {
		case 401:
			code, message = "authentication", "GitHub 凭据验证失败"
		case 403, 404:
			code, message = "permission", "当前账号没有读取权限或资源已不可访问"
		case 429:
			code, message = "rate-limited", "GitHub 查询达到频率限制，请稍后重试"
		}
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		code, message = "cancelled", "已取消本次读取"
	}
	return NewReadError(code, message)
}

func discoveryErrorObject(ctx context.Context, err error) Object {
	value := discoveryError(ctx, err).(*ReadError)
	return Object{"code": value.Code, "message": value.Message}
}
