package app

import (
	"context"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"strings"
	"testing"
)

func TestRepositoryNamePlanPriorityAndNoWrite(t *testing.T) {
	d, _, _, root := setupDevices(t)
	record := create(t, d, root)
	statePath := filepath.Join(str(record["path"]), "state.json")
	before, _ := os.ReadFile(statePath)
	plan := call(t, d, "repository-plan", Object{"project_id": "P-demo"})
	if plan["repository_name"] != "RumSDLC_Demo" || plan["source"] != "project-name" || plan["editable"] != true {
		t.Fatal(plan)
	}
	call(t, d, "bind-code", Object{"project_id": "P-demo", "repository": "https://github.com/example/Code-App.git", "kind": "git"})
	plan = call(t, d, "repository-plan", Object{"project_id": "P-demo"})
	if plan["repository_name"] != "RumSDLC_Code-App" || plan["source"] != "code-repository" {
		t.Fatal(plan)
	}
	plan = call(t, d, "repository-plan", Object{"project_id": "P-demo", "name": "my-choice"})
	if plan["repository_name"] != "RumSDLC_my-choice" || plan["source"] != "user" {
		t.Fatal(plan)
	}
	after, _ := os.ReadFile(statePath)
	if string(before) != string(after) {
		t.Fatal("plan modified business state")
	}
	for _, name := range []string{"", "bad/name", "RumSDLC_", "..", strings.Repeat("x", 100)} {
		if _, err := d.Call(context.Background(), "repository-plan", Object{"project_id": "P-demo", "name": name}); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for source, want := range map[string]string{"git@github.com:owner/code.git": "code", "https://github.com/o/code.git?x=1": "code", `C:\projects\code.git`: "code", "/tmp/code.git/": "code"} {
		if got := codeRepositoryName(source); got != want {
			t.Fatalf("%s -> %s", source, got)
		}
	}
	if got := suggestedRepositoryName("中文 项目"); got != "RumSDLC_project" {
		t.Fatal(got)
	}
}
func TestEditableRepositoryCreationAndOccupiedName(t *testing.T) {
	for _, mode := range []string{"project", "code", "user"} {
		t.Run(mode, func(t *testing.T) {
			d, _, hub, root := setupDevices(t)
			create(t, d, root)
			params := Object{"project_id": "P-demo"}
			want := "RumSDLC_Demo"
			if mode == "code" {
				call(t, d, "bind-code", Object{"project_id": "P-demo", "repository": "https://github.com/o/application.git", "kind": "git"})
				want = "RumSDLC_application"
			}
			if mode == "user" {
				params["repository_name"] = "chosen"
				want = "RumSDLC_chosen"
			}
			call(t, d, "enable", params)
			if hub.repos["alice/"+want] == nil {
				t.Fatal(hub.repos)
			}
			// A different Project with the same display name cannot acquire this repo.
			call(t, d, "create", Object{"path": filepath.Join(root, "other"), "name": "Demo", "date": "2026-11-06", "project_id": "P-other"})
			_, err := d.Call(context.Background(), "enable", Object{"project_id": "P-other", "repository_name": want})
			if err == nil || !strings.Contains(err.Error(), "occupied") {
				t.Fatal(err)
			}
			if len(hub.repos) != 1 {
				t.Fatal("collision created another repository")
			}
		})
	}
}

type catalogManifestHub struct {
	*fakeHub
	manifests map[string]Object
	failures  map[string]string
	reads     map[string]int
}

func (h *catalogManifestHub) ForLogin(string) Hub { return h }
func (h *catalogManifestHub) Manifest(_ context.Context, name, branch string) (Object, error) {
	h.reads[name]++
	if kind := h.failures[name]; kind != "" {
		return nil, &ManifestReadError{Kind: kind}
	}
	return h.manifests[name], nil
}
func TestDiscoveryReadsIdentityAndKeepsIndividualFailures(t *testing.T) {
	d, _, base, root := setupDevices(t)
	record := create(t, d, root)
	manifest := manifestFor(readState(str(record["path"])))
	hub := &catalogManifestHub{fakeHub: base, manifests: map[string]Object{}, failures: map[string]string{}, reads: map[string]int{}}
	d.Hub = hub
	for i, name := range []string{"RumSDLC_new", "RumSDLC_legacy", "RumSDLC_missing", "RumSDLC_invalid", "RumSDLC_unavailable", "ordinary"} {
		full := "alice/" + name
		base.repos[full] = Object{"id": i + 1, "name": name, "full_name": full, "private": true, "default_branch": "main", "permissions": Object{"push": true}}
		switch name {
		case "RumSDLC_new":
			v := copyObject(manifest)
			v["name"] = "Cloud display"
			hub.manifests[full] = v
		case "RumSDLC_legacy":
			v := copyObject(manifest)
			delete(v, "name")
			hub.manifests[full] = v
		case "RumSDLC_invalid":
			hub.manifests[full] = Object{"format": "unknown"}
		case "RumSDLC_missing":
			hub.failures[full] = "missing"
		case "RumSDLC_unavailable":
			hub.failures[full] = "unavailable"
		}
	}
	result := call(t, d, "discover", nil)
	rows := array(result["remote_repositories"], "repositories")
	if len(rows) != 5 {
		t.Fatal(result)
	}
	for _, value := range rows {
		row := object(value, "row")
		name := str(row["name"])
		switch name {
		case "RumSDLC_new":
			if row["display_name"] != "Cloud display" || row["manifest_status"] != "valid" || row["local_project_id"] != "P-demo" {
				t.Fatal(row)
			}
		case "RumSDLC_legacy":
			if row["display_name"] != "Demo" || row["name_source"] != "local-project" {
				t.Fatal(row)
			}
		default:
			want := strings.TrimPrefix(name, "RumSDLC_")
			if row["manifest_status"] != want || row["verified_project"] != false {
				t.Fatal(row)
			}
		}
	}
	if hub.reads["alice/ordinary"] != 0 {
		t.Fatal("read a noncandidate manifest")
	}
}
func TestLegacyManifestAndWrongIdentity(t *testing.T) {
	d, _, _, root := setupDevices(t)
	record := create(t, d, root)
	state := readState(str(record["path"]))
	legacy := manifestFor(state)
	if len(legacy) != 4 || legacy["name"] != "Demo" {
		t.Fatal("default manifest is missing project display name", legacy)
	}
	delete(legacy, "name")
	if err := attempt(func() { validateManifest(legacy, state) }); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"project_id", "genesis"} {
		v := copyObject(legacy)
		v[field] = "wrong"
		if err := attempt(func() { validateManifest(v, state) }); err == nil {
			t.Fatal("accepted wrong " + field)
		}
	}
}
func TestBoundRepositoryRenameRetainsIdentityAndSyncs(t *testing.T) {
	d, other, hub, root := setupDevices(t)
	record := create(t, d, root)
	call(t, d, "bind-code", Object{"project_id": "P-demo", "repository": "https://github.com/o/source.git", "kind": "git"})
	enabled := call(t, d, "enable", Object{"project_id": "P-demo", "repository_name": "original"})
	if enabled["status"] != "synced" {
		t.Fatal(enabled)
	}
	old := "alice/RumSDLC_original"
	next := "alice/RumSDLC_renamed"
	repo := hub.repos[old]
	delete(hub.repos, old)
	repo["full_name"], repo["name"] = next, "RumSDLC_renamed"
	hub.repos[next] = repo
	result := call(t, d, "sync", Object{"project_id": "P-demo"})
	if result["status"] != "synced" {
		t.Fatal(result)
	}
	cfg := loadConfig(str(record["path"]), true)
	repos := object(cfg["repositories"], "repositories")
	meta := object(repos["meta"], "meta")
	if meta["url"] != "https://github.com/"+next+".git" || !equal(meta["id"], repo["id"]) || object(repos["code"], "code")["url"] != "https://github.com/o/source.git" {
		t.Fatal(cfg)
	}
	imported := call(t, other, "import", Object{"repository": next, "path": filepath.Join(root, "other")})
	if imported["id"] != "P-demo" {
		t.Fatal(imported)
	}
	state := readState(str(record["path"]))
	if state["revision"] != readState(str(imported["path"]))["revision"] {
		t.Fatal("rename changed history")
	}
}
func TestRepositoryRenameRejectsSpoofAndTamperedOrigin(t *testing.T) {
	for _, mode := range []string{"repository-id", "manifest", "prefix", "origin"} {
		t.Run(mode, func(t *testing.T) {
			d, _, hub, root := setupDevices(t)
			record := create(t, d, root)
			call(t, d, "enable", Object{"project_id": "P-demo"})
			old := "alice/RumSDLC_Demo"
			next := "alice/RumSDLC_next"
			repo := hub.repos[old]
			delete(hub.repos, old)
			repo["full_name"], repo["name"] = next, "RumSDLC_next"
			if mode == "repository-id" {
				repo["id"] = 99
			}
			if mode == "prefix" {
				next = "alice/ordinary"
				repo["full_name"], repo["name"] = next, "ordinary"
			}
			hub.repos[next] = repo
			if mode == "manifest" {
				override := &catalogManifestHub{fakeHub: hub, manifests: map[string]Object{next: {"format": "rumsdlc-project/1", "project_id": "P-wrong", "genesis": strings.Repeat("a", 64)}}, failures: map[string]string{}, reads: map[string]int{}}
				d.Hub = override
			}
			if mode == "origin" {
				codeGitChecked(context.Background(), str(record["path"]), "remote", "set-url", "origin", filepath.Join(root, "wrong.git"))
			}
			before, _ := os.ReadFile(filepath.Join(str(record["path"]), configFile))
			if _, err := d.Call(context.Background(), "sync", Object{"project_id": "P-demo"}); err == nil {
				t.Fatal("accepted " + mode)
			}
			after, _ := os.ReadFile(filepath.Join(str(record["path"]), configFile))
			if string(before) != string(after) {
				t.Fatal("failed verification changed binding")
			}
			if !fsutil.IsFile(filepath.Join(str(record["path"]), "state.json")) {
				t.Fatal("lost state")
			}
		})
	}
}

func TestLegacyLongRepositoryNameIsReusedByManifestIdentity(t *testing.T) {
	a, b, hub, root := setupDevices(t)
	create(t, a, root)
	call(t, a, "enable", Object{"project_id": "P-demo", "repository_name": "P-demo"})
	call(t, b, "create", Object{"path": filepath.Join(root, "second-local"), "name": "Demo", "date": "2026-11-06", "project_id": "P-demo"})
	call(t, b, "enable", Object{"project_id": "P-demo"})
	if len(hub.repos) != 1 || hub.repos["alice/RumSDLC_P-demo"] == nil {
		t.Fatal("legacy repository was duplicated", hub.repos)
	}
	db, err := b.ReadDB()
	if err != nil {
		t.Fatal(err)
	}
	remote := object(object(object(db["projects"], "projects")["P-demo"], "record")["remote"], "remote")
	if remote["full_name"] != "alice/RumSDLC_P-demo" {
		t.Fatal(remote)
	}
}

type legacyDisplayHub struct{ *catalogManifestHub }

func (h *legacyDisplayHub) ForLogin(string) Hub { return h }
func (h *legacyDisplayHub) ProjectDisplayName(context.Context, string, string) (string, error) {
	return "Legacy remote project", nil
}
func TestRemoteOnlyLegacyProjectDisplayName(t *testing.T) {
	d, _, base, _ := setupDevices(t)
	full := "alice/RumSDLC_P-legacy"
	base.repos[full] = Object{"id": 11, "name": "RumSDLC_P-legacy", "full_name": full, "private": true, "default_branch": "main", "permissions": Object{}}
	hub := &legacyDisplayHub{&catalogManifestHub{fakeHub: base, manifests: map[string]Object{full: {"format": "rumsdlc-project/1", "project_id": "P-legacy", "genesis": strings.Repeat("a", 64)}}, failures: map[string]string{}, reads: map[string]int{}}}
	d.Hub = hub
	result := call(t, d, "discover", nil)
	row := object(array(result["remote_repositories"], "repositories")[0], "row")
	if row["display_name"] != "Legacy remote project" || row["name_source"] != "project-config" || row["manifest_status"] != "valid" || row["local_project_id"] != nil {
		t.Fatal(row)
	}
}

func TestImportedV1ConfigurationCompatibility(t *testing.T) {
	d, _, _, root := setupDevices(t)
	record := create(t, d, root)
	path := str(record["path"])
	if err := fsutil.WriteJSON(filepath.Join(path, configFile), Object{"format": "rum-workspace/1", "name": "Demo", "rules_version": Version, "code_repository": nil}); err != nil {
		t.Fatal(err)
	}
	remote := Object{"id": 123, "full_name": "alice/RumSDLC_renamed", "branch": "main"}
	if err := attempt(func() { refreshImportedMetaBinding(path, remote) }); err != nil {
		t.Fatal(err)
	}
	if err := attempt(func() { checkMetaBinding(path, remote) }); err != nil {
		t.Fatal(err)
	}
}
