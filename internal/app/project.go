package app

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/fsutil"
	"strings"
	"unicode"
)

const configFile = "project-config.json"

func authentication(login string) Object {
	if login == "" {
		return Object{"mode": "global"}
	}
	require(regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`).MatchString(login), "Invalid GitHub login")
	return Object{"mode": "github", "login": login}
}
func validateAuth(value any) Object {
	v := object(value, "Invalid authentication policy")
	if equal(v, Object{"mode": "global"}) {
		return v
	}
	require(len(v) == 2 && v["mode"] == "github" && has(v, "login"), "Authentication must be global or github/login; credentials must not be saved")
	authentication(text(v["login"], "GitHub login"))
	return v
}
func codeURL(value any) string {
	s := text(value, "Repository address")
	require(!strings.HasPrefix(s, "-"), "Invalid repository address")
	require(!strings.HasPrefix(s, "ext:") && !strings.HasPrefix(s, "fd:") && !strings.HasPrefix(s, "git://"), "Use HTTPS, SSH or a local Git repository")
	if strings.Contains(s, "://") {
		p, e := url.Parse(s)
		must(e)
		password := false
		if p.User != nil {
			_, password = p.User.Password()
		}
		require(oneOf(p.Scheme, "https ssh file") && !password && p.RawQuery == "" && p.Fragment == "", "Repository addresses may not contain passwords, query tokens or fragments")
		require(p.Scheme != "https" || p.User == nil || p.User.Username() == "", "Use the credential helper, not credentials in the URL")
	}
	return s
}
func portableURL(value any) string {
	s := codeURL(value)
	require(strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "ssh://") || regexp.MustCompile(`^[^/@:\s]+@[^/:\s]+:.+$`).MatchString(s), "Portable repositories require HTTPS or SSH, not a local path or URL alias")
	return s
}
func branchName(value any) string {
	s := text(value, "Branch name")
	valid := !strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, ".") && !strings.HasSuffix(s, "/") && !strings.HasSuffix(s, ".") && s != "@" && !strings.Contains(s, "..") && !strings.Contains(s, "@{") && !strings.Contains(s, "//")
	for _, r := range s {
		valid = valid && !unicode.IsSpace(r) && r >= 32 && r != 127 && !strings.ContainsRune("~^:?*[\\", r)
	}
	for _, p := range strings.Split(s, "/") {
		valid = valid && !strings.HasPrefix(p, ".") && !strings.HasSuffix(p, ".lock")
	}
	require(valid, "Invalid branch name")
	return s
}
func reviewURL(value any) string {
	s := text(value, "Review URL")
	p, e := url.Parse(s)
	must(e)
	valid := p.Scheme == "https" && p.Hostname() != "" && p.User == nil && p.RawQuery == "" && p.Fragment == ""
	for _, r := range s {
		valid = valid && !unicode.IsSpace(r)
	}
	require(valid, "Review URL must be credential-free HTTPS without query/fragment")
	return s
}

func ValidateConfig(value Object) (result Object, err error) {
	defer guard(&err)
	return validateConfig(value), nil
}
func validateConfig(value Object) Object {
	format := str(value["format"])
	require(oneOf(format, "rum-workspace/1 rum-workspace/2 rum-workspace/3 rum-workspace/4 rum-workspace/5"), "Unsupported workspace configuration")
	text(value["name"], "Missing workspace name")
	require(value["rules_version"] == Version || oneOf(str(value["rules_version"]), "0.2.0 0.3.0 0.4.0 0.4.1 0.5.0 0.6.0 0.7.0 0.7.1 0.7.2 0.7.3 0.7.4 0.7.5 0.8.0 0.8.1 0.8.2 0.8.3 0.8.4 0.8.5"), "Requested workflow version is not installed")
	result := copyObject(value)
	if format == "rum-workspace/1" {
		result = Object{"format": "rum-workspace/2", "name": value["name"], "rules_version": value["rules_version"], "repositories": Object{"meta": Object{"url": nil, "auth": Object{"mode": "device"}, "visibility": "private"}, "code": Object{"url": value["code_repository"], "auth": Object{"mode": "global"}}}}
	} else {
		exact(value, "format name rules_version repositories", "Unknown project configuration fields")
	}
	repos := object(result["repositories"], "repositories")
	exact(repos, "meta code", "Both meta/code bindings are required")
	code := object(repos["code"], "code binding")
	if oneOf(format, "rum-workspace/1 rum-workspace/2") && has(code, "auth") {
		validateAuth(code["auth"])
		delete(code, "auth")
	}
	if !oneOf(format, "rum-workspace/4 rum-workspace/5") {
		code["kind"] = "unspecified"
		if truth(code["url"]) {
			code["kind"] = "git"
		}
		code["topic_branches"] = Object{}
	}
	if format != "rum-workspace/5" {
		code["topic_reviews"] = Object{}
	}
	result["format"] = "rum-workspace/5"
	for role, v := range repos {
		binding := object(v, "repository binding")
		allowed := "url kind topic_branches topic_reviews"
		if role == "meta" {
			allowed = "url auth visibility id branch"
		}
		only(binding, allowed, "Unknown repository binding fields")
		require(has(binding, "url"), "Repository URL field is required")
		if binding["url"] != nil {
			portableURL(binding["url"])
		}
		if role == "code" {
			require(oneOf(str(binding["kind"]), "git directory unspecified"), "Invalid code kind")
			branches := object(binding["topic_branches"], "Missing Topic branch records")
			for topic, b := range branches {
				require(validTopic(topic), "Invalid Topic ID")
				branchName(b)
			}
			reviews := object(binding["topic_reviews"], "Missing Topic review records")
			for topic, v := range reviews {
				require(validTopic(topic), "Invalid Topic ID")
				review := object(v, "review record")
				exact(review, "url target_branch", "Invalid review record")
				reviewURL(review["url"])
				branchName(review["target_branch"])
			}
		} else {
			require(equal(binding["auth"], Object{"mode": "device"}), "Meta authentication is device-wide; configure it with setup")
			require(oneOf(str(binding["visibility"]), "private public"), "Invalid meta visibility")
			require(binding["url"] == nil || strings.HasPrefix(str(binding["url"]), "https://github.com/"), "Meta repositories currently require GitHub HTTPS")
			if has(binding, "id") {
				require(integer(binding["id"]) > 0, "Invalid repository ID")
			}
			if has(binding, "branch") {
				s := text(binding["branch"], "Meta branch")
				require(!strings.HasPrefix(s, "-"), "Invalid meta branch")
			}
		}
	}
	return result
}
func validTopic(value string) bool {
	return regexp.MustCompile(`^T-[0-9]{8}-[a-f0-9]{8}$`).MatchString(value)
}
func loadConfig(root string, required bool) Object {
	path := filepath.Join(root, configFile)
	if i, e := os.Lstat(path); e == nil {
		require(i.Mode()&os.ModeSymlink == 0, "Project configuration cannot be a symlink")
	}
	value, e := fsutil.ReadJSON(path)
	if os.IsNotExist(e) {
		require(!required, "Missing project-config.json")
		return nil
	}
	must(e)
	return validateConfig(value)
}
func saveConfig(root string, value Object) {
	path := filepath.Join(root, configFile)
	if i, e := os.Lstat(path); e == nil {
		require(i.Mode()&os.ModeSymlink == 0, "Project configuration cannot be a symlink")
	}
	must(fsutil.WriteJSON(path, validateConfig(value)))
}
func ensureConfig(root, name string, repository any) Object {
	value := loadConfig(root, false)
	if value != nil {
		return value
	}
	kind := "unspecified"
	if truth(repository) {
		kind = "git"
	}
	value = Object{"format": "rum-workspace/5", "name": name, "rules_version": Version, "repositories": Object{"meta": Object{"url": nil, "auth": Object{"mode": "device"}, "visibility": "private"}, "code": Object{"url": repository, "kind": kind, "topic_branches": Object{}, "topic_reviews": Object{}}}}
	saveConfig(root, value)
	return value
}
func checkMetaBinding(root string, remote Object) {
	cfg := loadConfig(root, true)
	raw, e := fsutil.ReadJSON(filepath.Join(root, configFile))
	must(e)
	if raw["format"] == "rum-workspace/1" {
		return
	}
	b := object(object(cfg["repositories"], "repositories")["meta"], "meta")
	require(b["url"] == "https://github.com/"+str(remote["full_name"])+".git" && equal(b["id"], remote["id"]) && equal(b["branch"], remote["branch"]), "Project meta binding differs from the pinned repository")
}
func setMetaBinding(record, repo Object, branch string) {
	root := str(record["path"])
	cfg := loadConfig(root, true)
	b := object(object(cfg["repositories"], "repositories")["meta"], "meta")
	b["url"] = "https://github.com/" + str(repo["full_name"]) + ".git"
	b["id"] = repo["id"]
	b["branch"] = branch
	b["visibility"] = "public"
	if repo["private"] == true {
		b["visibility"] = "private"
	}
	saveConfig(root, cfg)
}
