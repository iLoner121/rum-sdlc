package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/process"
	"strconv"
	"strings"
	"time"
)

// Hub is the remote control-plane boundary. Tests can provide an in-memory
// catalog while exercising real Git against local bare repositories.
type Hub interface {
	ForLogin(string) Hub
	User(context.Context) (Object, error)
	Repositories(context.Context) ([]Object, error)
	Repository(context.Context, string) (Object, error)
	Create(context.Context, string) (Object, error)
	Manifest(context.Context, string, string) (Object, error)
	Empty(context.Context, string) (bool, error)
	URL(Object) (string, error)
	Token(context.Context) (string, error)
	Login() string
}

type GitHub struct {
	Account string
	Client  *http.Client
	Runner  process.Runner
}

func NewGitHub() *GitHub {
	return &GitHub{Client: contractHTTP(&http.Client{Timeout: 90 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("Refusing credential-bearing cross-host redirect")
		}
		if len(via) >= 10 {
			return fmt.Errorf("Too many redirects")
		}
		return nil
	}})}
}
func (g *GitHub) ForLogin(login string) Hub { copy := *g; copy.Account = login; return &copy }
func (g *GitHub) Login() string             { return g.Account }
func (g *GitHub) Token(ctx context.Context) (string, error) {
	if g.Account == "" {
		for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if token := strings.TrimSpace(os.Getenv(key)); token != "" {
				return token, nil
			}
		}
	}
	// Existing gh credentials remain usable, but gh is not a runtime requirement.
	// Without it, the user's installed Git credential helper supplies credentials.
	if gh, e := exec.LookPath("gh"); e == nil {
		env := process.Environment(true)
		for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_DEBUG"} {
			delete(env, key)
		}
		args := []string{gh, "auth", "token", "--hostname", "github.com"}
		if g.Account != "" {
			args = append(args, "--user", g.Account)
		}
		result, e := g.Runner.Run(ctx, args, "", "", env, 20*time.Second, false)
		if e == nil && result.Code == 0 && strings.TrimSpace(result.Stdout) != "" {
			return strings.TrimSpace(result.Stdout), nil
		}
	}
	input := "protocol=https\nhost=github.com\n"
	if g.Account != "" {
		input += "username=" + g.Account + "\n"
	}
	input += "\n"
	env := process.CodeEnvironment()
	for _, key := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"} {
		if value, ok := os.LookupEnv("RUMSDLC_CREDENTIAL_" + key); ok {
			if value == "" {
				delete(env, key)
			} else {
				env[key] = value
			}
		}
	}
	result, e := g.Runner.Run(ctx, []string{"git", "credential", "fill"}, "", input, env, 20*time.Second, false)
	if e == nil && result.Code == 0 {
		fields := map[string]string{}
		for _, line := range strings.Split(result.Stdout, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				fields[k] = v
			}
		}
		if fields["password"] != "" && (g.Account == "" || fields["username"] == g.Account) {
			return fields["password"], nil
		}
	}
	return "", fmt.Errorf("No usable GitHub credential for the selected account; configure a Git credential helper or an existing GitHub login")
}
func (g *GitHub) api(ctx context.Context, endpoint, method string, body any) (any, error) {
	token, e := g.Token(ctx)
	if e != nil {
		return nil, e
	}
	var input io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		input = bytes.NewReader(raw)
	}
	req, e := http.NewRequestWithContext(ctx, method, "https://api.github.com/"+endpoint, input)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "RumSDLC/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := g.Client
	if client == nil {
		client = NewGitHub().Client
	}
	response, e := client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("GitHub API request failed: %s %s", method, strings.Split(endpoint, "?")[0])
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &githubStatusError{Code: response.StatusCode, Message: fmt.Sprintf("GitHub API request failed: %s %s (HTTP %d)", method, strings.Split(endpoint, "?")[0], response.StatusCode)}
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 32*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 32*1024*1024 {
		return nil, fmt.Errorf("GitHub response exceeds limit")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	return jsonvalue.Decode(raw)
}
func fullName(value string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`).MatchString(value) {
		return fmt.Errorf("Repository must be owner/name on github.com")
	}
	return nil
}
func (g *GitHub) User(ctx context.Context) (result Object, err error) {
	defer guard(&err)
	v, e := g.api(ctx, "user", "GET", nil)
	must(e)
	user := object(v, "GitHub user")
	return Object{"id": user["id"], "login": user["login"], "name": user["name"]}, nil
}
func (g *GitHub) Repositories(ctx context.Context) (result []Object, err error) {
	defer guard(&err)
	result = []Object{}
	for page := 1; ; page++ {
		v, e := g.api(ctx, "user/repos?visibility=all&affiliation=owner,collaborator,organization_member&per_page=100&page="+strconv.Itoa(page), "GET", nil)
		must(e)
		batch := array(v, "GitHub repository listing")
		for _, v := range batch {
			result = append(result, object(v, "Repository"))
		}
		if len(batch) < 100 {
			return result, nil
		}
	}
}
func (g *GitHub) Repository(ctx context.Context, name string) (result Object, err error) {
	defer guard(&err)
	must(fullName(name))
	v, e := g.api(ctx, "repos/"+name, "GET", nil)
	must(e)
	return object(v, "Repository"), nil
}
func (g *GitHub) Create(ctx context.Context, name string) (result Object, err error) {
	defer guard(&err)
	require(strings.HasPrefix(name, RepositoryPrefix), "Not a RumSDLC repository name")
	v, e := g.api(ctx, "user/repos", "POST", Object{"name": name, "private": true, "auto_init": false, "description": "RumSDLC project management data", "has_issues": false, "has_projects": false, "has_wiki": false})
	must(e)
	return object(v, "Repository"), nil
}
func (g *GitHub) Manifest(ctx context.Context, name, branch string) (result Object, err error) {
	defer func() {
		if err != nil {
			var typed *ManifestReadError
			if !errors.As(err, &typed) {
				err = &ManifestReadError{Kind: "invalid", Cause: err}
			}
		}
	}()
	defer guard(&err)
	must(fullName(name))
	v, e := g.api(ctx, "repos/"+name+"/contents/"+Manifest+"?ref="+url.QueryEscape(branch), "GET", nil)
	if e != nil {
		kind := "unavailable"
		var status *githubStatusError
		if errors.As(e, &status) && status.Code == 404 {
			kind = "missing"
		}
		return nil, &ManifestReadError{Kind: kind, Cause: e}
	}
	file := object(v, "Manifest response")
	require(file["encoding"] == "base64" && file["type"] == "file", "Missing or invalid RumSDLC project manifest")
	raw, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(str(file["content"]), "\n", ""))
	must(e)
	value, e := jsonvalue.Decode(raw)
	must(e)
	return object(value, "Manifest"), nil
}
func (g *GitHub) Empty(ctx context.Context, name string) (bool, error) {
	if e := fullName(name); e != nil {
		return false, e
	}
	v, e := g.api(ctx, "repos/"+name+"/branches?per_page=1", "GET", nil)
	if e != nil {
		return false, e
	}
	list, ok := v.([]any)
	return ok && len(list) == 0, nil
}
func (g *GitHub) URL(repo Object) (string, error) {
	name := str(repo["full_name"])
	if e := fullName(name); e != nil {
		return "", e
	}
	return "https://github.com/" + name + ".git", nil
}
func checkedRepository(repo Object, writable bool) Object {
	must(fullName(str(repo["full_name"])))
	_, ok := repo["private"].(bool)
	require(ok, "Missing repository visibility")
	require(strings.HasPrefix(str(repo["name"]), RepositoryPrefix), "Repository name must start with RumSDLC_")
	require(!truth(repo["archived"]) && !truth(repo["disabled"]), "Repository is archived or disabled")
	if writable {
		require(getObject(repo, "permissions")["push"] == true, "This identity cannot push to the selected repository")
	}
	return repo
}

// githubStatusError keeps error classification without exposing response bodies.
type githubStatusError struct {
	Code    int
	Message string
}

func (e *githubStatusError) Error() string { return e.Message }

// ProjectDisplayName is a best-effort fallback for old three-field manifests.
// The manifest is validated first; this display metadata never establishes identity.
func (g *GitHub) ProjectDisplayName(ctx context.Context, name, branch string) (result string, err error) {
	defer guard(&err)
	must(fullName(name))
	value, e := g.api(ctx, "repos/"+name+"/contents/"+configFile+"?ref="+url.QueryEscape(branch), "GET", nil)
	must(e)
	file := object(value, "Project configuration response")
	require(file["type"] == "file" && file["encoding"] == "base64", "Missing project display name")
	raw, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(str(file["content"]), "\n", ""))
	must(e)
	value, e = jsonvalue.Decode(raw)
	must(e)
	return text(object(value, "Project configuration")["name"], "Project display name"), nil
}
