package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"strings"
	"testing"
)

type readRemoteHub struct {
	Hub       // Unimplemented/write operations panic if the reader ever invokes them.
	user      Object
	repos     []Object
	manifests map[string]Object
	failures  map[string]error
	configs   map[string]Object
	listError error
	reads     []string
	selected  string
}

func (h *readRemoteHub) ForLogin(login string) Hub            { h.selected = login; return h }
func (h *readRemoteHub) User(context.Context) (Object, error) { return h.user, nil }
func (h *readRemoteHub) Repositories(context.Context) ([]Object, error) {
	return h.repos, h.listError
}
func (h *readRemoteHub) Manifest(_ context.Context, name, _ string) (Object, error) {
	h.reads = append(h.reads, name)
	return h.manifests[name], h.failures[name]
}
func (h *readRemoteHub) ProjectConfig(_ context.Context, name, _ string) (Object, error) {
	if cfg := h.configs[name]; cfg != nil {
		return cfg, nil
	}
	return nil, errors.New("fixture configuration unavailable")
}

func remoteFixture() *readRemoteHub {
	return &readRemoteHub{user: Object{"id": 7, "login": "alice"}, manifests: map[string]Object{}, failures: map[string]error{}, configs: map[string]Object{}}
}
func remoteRepo(id int, suffix string) Object {
	return Object{"id": id, "name": RepositoryPrefix + suffix, "full_name": "alice/" + RepositoryPrefix + suffix, "default_branch": "main", "private": true}
}
func remoteManifest(id string) Object {
	return Object{"format": "rumsdlc-project/1", "project_id": id, "genesis": strings.Repeat("a", 64), "name": "Cloud " + id}
}
func remoteConfig() Object {
	return Object{"format": "rum-workspace/5", "name": "Configuration name", "rules_version": Version, "repositories": Object{
		"meta": Object{"url": nil, "auth": Object{"mode": "device"}, "visibility": "private"},
		"code": Object{"url": nil, "kind": "unspecified", "topic_branches": Object{}, "topic_reviews": Object{}}}}
}
func remoteRegistry(t *testing.T, home string, projects Object, auth Object) {
	t.Helper()
	if e := os.MkdirAll(home, 0700); e != nil {
		t.Fatal(e)
	}
	if e := fsutil.WriteJSON(filepath.Join(home, "registry.json"), Object{"format": "rumsdlc-device/1", "identity": nil, "projects": projects, "repositories": Object{}, "accounts": Object{}, "meta_auth": auth}); e != nil {
		t.Fatal(e)
	}
}
func remoteTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if d.IsDir() {
			result[rel] = "directory"
			return nil
		}
		raw, e := os.ReadFile(path)
		if e == nil {
			result[rel] = string(raw)
		}
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	return result
}
func remoteData(t *testing.T, r *Reader) Object {
	t.Helper()
	v, e := r.ReadRemoteProjects(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if v.SchemaVersion != "gui-view/1" || v.ReadAt == "" || v.Basis != "remote-discovery" {
		t.Fatal(v)
	}
	return v.Data.(Object)
}

func TestReadRemoteHasNoInitializationOrCacheWrites(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	home := filepath.Join(root, "absent", "device")
	h := remoteFixture()
	r, e := NewReader(home)
	if e != nil {
		t.Fatal(e)
	}
	r.RemoteHub = h
	before := remoteTree(t, root)
	if data := remoteData(t, r); data["total"] != 0 {
		t.Fatal(data)
	}
	if !equal(before, remoteTree(t, root)) {
		t.Fatal("empty discovery created files")
	}
	remoteRegistry(t, home, Object{}, authentication("alice"))
	before = remoteTree(t, root)
	remoteData(t, r)
	if !equal(before, remoteTree(t, root)) {
		t.Fatal("discovery changed registry or lock files")
	}
	if h.selected != "alice" {
		t.Fatal("selected policy was ignored")
	}
}

func TestReadRemoteKeepsInvalidCandidatesAndIndependentCodeState(t *testing.T) {
	h := remoteFixture()
	for i, kind := range []string{"valid", "legacy", "missing", "invalid", "denied"} {
		repo := remoteRepo(i+1, kind)
		name := str(repo["full_name"])
		h.repos = append(h.repos, repo)
		switch kind {
		case "valid", "legacy":
			h.manifests[name] = remoteManifest("P-" + kind)
			h.configs[name] = remoteConfig()
			if kind == "legacy" {
				delete(h.manifests[name], "name")
			}
		case "missing":
			h.failures[name] = &ManifestReadError{Kind: "missing"}
		case "invalid":
			h.manifests[name] = Object{"format": "bad"}
		case "denied":
			h.failures[name] = &ManifestReadError{Kind: "unavailable", Cause: &githubStatusError{Code: 403}}
		}
	}
	h.repos = append(h.repos, Object{"id": 6, "name": "ordinary", "full_name": "alice/ordinary"})
	r := &Reader{Home: filepath.Join(t.TempDir(), "missing"), RemoteHub: h}
	data := remoteData(t, r)
	if data["total"] != 5 || data["verified_total"] != 2 || len(h.reads) != 5 {
		t.Fatal(data)
	}
	for _, v := range data["rows"].([]any) {
		row := v.(Object)
		if row["manifest_status"] == "valid" {
			if getObject(row, "local")["status"] != "not-imported" || getObject(row, "code")["association"] != "unconfigured" || getObject(row, "code")["availability"] != "unknown" {
				t.Fatal(row)
			}
			if row["name"] == "RumSDLC_legacy" && row["display_name"] != "Configuration name" {
				t.Fatal(row)
			}
		} else if row["project_id"] != nil || getObject(row, "local")["project_id"] != nil {
			t.Fatal("invalid candidate exposes a project entry", row)
		}
		if row["name"] == "RumSDLC_denied" && getObject(row, "error")["code"] != "permission" {
			t.Fatal(row)
		}
	}
}

func TestReadRemoteLocalIdentityAndPathAreSeparateFromManifest(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	state, _, e := core.Dispatch(nil, core.Request("project.init", Object{"id": "P-local", "name": "Local"}, "2026-11-18", "fixture", nil, nil))
	if e != nil {
		t.Fatal(e)
	}
	local := filepath.Join(root, "local")
	if e := os.MkdirAll(local, 0700); e != nil {
		t.Fatal(e)
	}
	if e := fsutil.WriteJSON(filepath.Join(local, "state.json"), state); e != nil {
		t.Fatal(e)
	}
	if e := fsutil.WriteJSON(filepath.Join(local, configFile), remoteConfig()); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, path, genesis, status string
		remoteID                    int
	}{
		{"available", local, genesis(state), "available", 1},
		{"missing", filepath.Join(root, "gone"), genesis(state), "path-unavailable", 1},
		{"wrong-genesis", local, strings.Repeat("b", 64), "identity-conflict", 1},
		{"other-binding", local, genesis(state), "identity-conflict", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := remoteFixture()
			repo := remoteRepo(1, "local")
			h.repos = []Object{repo}
			manifest := remoteManifest("P-local")
			manifest["genesis"] = tc.genesis
			h.manifests[str(repo["full_name"])] = manifest
			home := filepath.Join(root, tc.name+"-device")
			remoteRegistry(t, home, Object{"P-local": Object{"id": "P-local", "path": tc.path, "remote": Object{"id": tc.remoteID}}}, authentication(""))
			before := remoteTree(t, root)
			data := remoteData(t, &Reader{Home: home, RemoteHub: h})
			row := data["rows"].([]any)[0].(Object)
			if row["manifest_status"] != "valid" || getObject(row, "local")["status"] != tc.status {
				t.Fatal(row)
			}
			if tc.status != "available" && getObject(row, "local")["project_id"] != nil {
				t.Fatal("unsafe local entry", row)
			}
			if !equal(before, remoteTree(t, root)) {
				t.Fatal("read changed local files")
			}
		})
	}
}

func TestReadRemoteErrorsDoNotBecomeEmptySuccess(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{{401, "authentication"}, {403, "permission"}, {429, "rate-limited"}, {500, "network"}} {
		h := remoteFixture()
		h.listError = &githubStatusError{Code: tc.code, Message: "fixture-secret"}
		v, e := (&Reader{Home: filepath.Join(t.TempDir(), "missing"), RemoteHub: h}).ReadRemoteProjects(context.Background())
		var re *ReadError
		if !errors.As(e, &re) || re.Code != tc.want || v.Data != nil || strings.Contains(e.Error(), "fixture-secret") {
			t.Fatal(v, e)
		}
	}
	h := remoteFixture()
	h.user = Object{"id": 8, "login": "bob"}
	home := t.TempDir()
	remoteRegistry(t, home, Object{}, authentication("alice"))
	_, e := (&Reader{Home: home, RemoteHub: h}).ReadRemoteProjects(context.Background())
	var re *ReadError
	if !errors.As(e, &re) || re.Code != "account-changed" {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = (&Reader{Home: home, RemoteHub: h}).ReadRemoteProjects(ctx)
	if !errors.As(e, &re) || re.Code != "cancelled" {
		t.Fatal(e)
	}
}

func TestReadRemotePinnedCredentialAndGETPagination(t *testing.T) {
	t.Setenv("GH_TOKEN", "changed-credential")
	g := NewGitHub()
	g.readToken = "initial-credential"
	pages, gets := 0, 0
	g.Client.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
		gets++
		if req.Method != "GET" || req.Header.Get("Authorization") != "Bearer initial-credential" || req.URL.Host != "api.github.com" {
			t.Fatal("unsafe request")
		}
		switch req.URL.Path {
		case "/user":
			return reply(req, Object{"id": 7, "login": "alice"}), nil
		case "/user/repos":
			pages++
			n := 100
			if pages == 2 {
				n = 1
			}
			rows := []any{}
			for i := 0; i < n; i++ {
				rows = append(rows, Object{"id": i + 100*pages, "name": "ordinary"})
			}
			return reply(req, rows), nil
		default:
			t.Fatal(req.URL)
			return nil, nil
		}
	})
	data := remoteData(t, &Reader{Home: filepath.Join(t.TempDir(), "missing"), RemoteHub: g})
	if pages != 2 || gets != 3 || data["total"] != 0 {
		t.Fatal(pages, gets, data)
	}
}

func TestReadRemoteGitHubConfigIsValidatedAndOnlyGET(t *testing.T) {
	g := NewGitHub()
	g.readToken = "fixture-secret"
	requests := 0
	g.Client.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != "GET" || req.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatal("unsafe request")
		}
		var value any
		switch req.URL.Path {
		case "/user":
			value = Object{"id": 7, "login": "alice"}
		case "/user/repos":
			value = []any{remoteRepo(1, "fixture")}
		case "/repos/alice/RumSDLC_fixture/contents/" + Manifest:
			raw, _ := json.Marshal(remoteManifest("P-fixture"))
			value = Object{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString(raw)}
		case "/repos/alice/RumSDLC_fixture/contents/" + configFile:
			raw, _ := json.Marshal(remoteConfig())
			value = Object{"type": "file", "encoding": "base64", "content": base64.StdEncoding.EncodeToString(raw)}
		default:
			return nil, fmt.Errorf("unexpected request")
		}
		return reply(req, value), nil
	})
	data := remoteData(t, &Reader{Home: filepath.Join(t.TempDir(), "missing"), RemoteHub: g})
	row := data["rows"].([]any)[0].(Object)
	if requests != 4 || getObject(row, "code")["association"] != "unconfigured" || row["manifest_status"] != "valid" {
		t.Fatal(data, requests)
	}
	serialized, _ := json.Marshal(data)
	if strings.Contains(string(serialized), "fixture-secret") {
		t.Fatal("credential leaked")
	}
}

func TestReadDiscoveryCredentialNeverCallsGitHelper(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", t.TempDir())
	_, _, e := readDiscoveryCredential(context.Background(), "")
	var re *ReadError
	if !errors.As(e, &re) || re.Code != "authentication" {
		t.Fatal(e)
	}
	t.Setenv("GH_TOKEN", "fixture-secret")
	value, source, e := readDiscoveryCredential(context.Background(), "")
	if e != nil || value != "fixture-secret" || source != "GH_TOKEN" {
		t.Fatal(source, e)
	}
	_, _, e = readDiscoveryCredential(context.Background(), "alice")
	if !errors.As(e, &re) || re.Code != "authentication" {
		t.Fatal("selected login accepted another environment account", e)
	}
}
