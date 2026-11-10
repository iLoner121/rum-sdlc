package app

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

func repositoryName(input string) string {
	name := input
	if !strings.HasPrefix(name, RepositoryPrefix) {
		name = RepositoryPrefix + name
	}
	suffix := strings.TrimPrefix(name, RepositoryPrefix)
	require(len(name) <= 100 && suffix != "" && suffix != "." && suffix != ".." && regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(suffix), "Choose a repository name with a nonempty suffix, ASCII letters, numbers, '.', '_' or '-'; maximum 100 characters including RumSDLC_")
	return name
}
func suggestedRepositoryName(source string) string {
	var out strings.Builder
	dash := false
	for _, c := range source {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
		if valid {
			out.WriteRune(c)
			dash = false
		} else if !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	suffix := strings.Trim(out.String(), ".-_")
	if suffix == "" {
		suffix = "project"
	}
	if len(suffix) > 100-len(RepositoryPrefix) {
		suffix = strings.TrimRight(suffix[:100-len(RepositoryPrefix)], ".-_")
	}
	return repositoryName(suffix)
}
func codeRepositoryName(source string) string {
	if source == "" {
		return ""
	}
	path := strings.ReplaceAll(source, "\\", "/")
	if strings.Contains(path, "://") {
		parsed, err := url.Parse(path)
		if err != nil {
			return ""
		}
		path = parsed.Path
	}
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, ":"); i >= 0 {
		path = path[i+1:]
	}
	return strings.TrimSuffix(path, ".git")
}
func (o *operation) repositoryPlan(p Object) Object {
	record := o.local(str(p["project_id"]))
	cfg := loadConfig(str(record["path"]), true)
	source, raw := "project-name", str(object(readState(str(record["path"]))["project"], "Project")["name"])
	code := object(object(cfg["repositories"], "repositories")["code"], "code")
	if name := codeRepositoryName(str(code["url"])); name != "" {
		source, raw = "code-repository", name
	}
	suggested := suggestedRepositoryName(raw)
	if p["name"] != nil {
		source, raw = "user", text(p["name"], "Repository name")
		suggested = repositoryName(raw)
	}
	return Object{"project_id": record["id"], "repository_name": suggested, "source": source, "source_value": raw, "editable": true, "prefix": RepositoryPrefix, "note": "Suggestion only; enable --repository-name accepts the edited name. Existing repositories are selected with --repository owner/name."}
}

type projectDisplayNameReader interface {
	ProjectDisplayName(context.Context, string, string) (string, error)
}

// ManifestReadError distinguishes an absent file from failed network/auth reads.
type ManifestReadError struct {
	Kind  string
	Cause error
}

func (e *ManifestReadError) Error() string { return "Project identity file " + e.Kind }
func (e *ManifestReadError) Unwrap() error { return e.Cause }
func manifestReadKind(err error) string {
	var failure *ManifestReadError
	if errors.As(err, &failure) {
		return failure.Kind
	}
	return "unavailable"
}
func validateManifestIdentity(value Object) {
	require(value["format"] == "rumsdlc-project/1", "Unsupported project identity file")
	require(regexp.MustCompile(`^P-[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(str(value["project_id"])), "Invalid manifest Project ID")
	require(regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(str(value["genesis"])), "Invalid manifest genesis")
	require(len(value) == 3 || len(value) == 4 && has(value, "name"), "Unknown manifest fields")
	if has(value, "name") {
		text(value["name"], "Manifest project display name")
	}
}
func (o *operation) inspectRepository(row, repo Object, hub Hub) {
	manifest, err := hub.Manifest(o.ctx, str(repo["full_name"]), str(row["branch"]))
	if err != nil {
		row["manifest_status"] = manifestReadKind(err)
		return
	}
	if err = attempt(func() { validateManifestIdentity(manifest) }); err != nil {
		row["manifest_status"] = "invalid"
		return
	}
	id := str(manifest["project_id"])
	local := getObject(o.db, "projects")[id]
	if local != nil {
		record := object(local, "local project")
		if err = attempt(func() { validateManifest(manifest, readState(str(record["path"]))) }); err != nil {
			row["manifest_status"] = "invalid"
			return
		}
	}
	row["verified_project"], row["manifest_status"], row["project_id"], row["genesis"] = true, "valid", id, manifest["genesis"]
	if truth(manifest["name"]) {
		row["project_name"], row["display_name"], row["name_source"] = manifest["name"], manifest["name"], "manifest"
	} else if local != nil {
		row["project_name"], row["display_name"], row["name_source"] = object(local, "record")["name"], object(local, "record")["name"], "local-project"
	} else if reader, ok := hub.(projectDisplayNameReader); ok {
		if name, e := reader.ProjectDisplayName(o.ctx, str(repo["full_name"]), str(row["branch"])); e == nil && strings.TrimSpace(name) != "" {
			row["project_name"], row["display_name"], row["name_source"] = name, name, "project-config"
		}
	}
}
