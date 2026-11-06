package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"strings"
	"testing"
)

type fakeHub struct {
	base        string
	repos       map[string]Object
	failListing bool
}

func (h *fakeHub) ForLogin(string) Hub                   { return h }
func (h *fakeHub) Login() string                         { return "alice" }
func (h *fakeHub) Token(context.Context) (string, error) { return "test-credential-never-used", nil }
func (h *fakeHub) User(context.Context) (Object, error) {
	return Object{"id": 7, "login": "alice", "name": "Alice"}, nil
}
func (h *fakeHub) Repositories(context.Context) ([]Object, error) {
	if h.failListing {
		return nil, fmt.Errorf("Network unavailable")
	}
	result := []Object{}
	for _, repo := range h.repos {
		result = append(result, copyObject(repo))
	}
	return result, nil
}
func (h *fakeHub) Repository(_ context.Context, name string) (Object, error) {
	v, ok := h.repos[name]
	if !ok {
		return nil, fmt.Errorf("Repository not found")
	}
	return copyObject(v), nil
}
func (h *fakeHub) Create(_ context.Context, name string) (Object, error) {
	full := "alice/" + name
	if h.repos[full] != nil {
		return nil, fmt.Errorf("Repository exists")
	}
	path := filepath.Join(h.base, fmt.Sprintf("%d.git", len(h.repos)+1))
	cmd := exec.Command("git", "init", "-q", "--bare", "--initial-branch=main", path)
	if raw, e := cmd.CombinedOutput(); e != nil {
		return nil, fmt.Errorf("%v %s", e, raw)
	}
	repo := Object{"id": len(h.repos) + 1, "name": name, "full_name": full, "private": true, "default_branch": "main", "permissions": Object{"push": true}, "archived": false, "disabled": false, "_path": path}
	h.repos[full] = repo
	return copyObject(repo), nil
}
func (h *fakeHub) Manifest(_ context.Context, name, branch string) (Object, error) {
	repo, e := h.Repository(context.Background(), name)
	if e != nil {
		return nil, e
	}
	raw, e := exec.Command("git", "--git-dir", str(repo["_path"]), "show", branch+":"+Manifest).Output()
	if e != nil {
		return nil, e
	}
	v, e := jsonvalue.Decode(raw)
	if e != nil {
		return nil, e
	}
	return v.(Object), nil
}
func (h *fakeHub) Empty(_ context.Context, name string) (bool, error) {
	repo, e := h.Repository(context.Background(), name)
	if e != nil {
		return false, e
	}
	raw, e := exec.Command("git", "--git-dir", str(repo["_path"]), "for-each-ref", "refs/heads").Output()
	return strings.TrimSpace(string(raw)) == "", e
}
func (h *fakeHub) URL(repo Object) (string, error) { return str(repo["_path"]), nil }
func setupDevices(t *testing.T) (*Device, *Device, *fakeHub, string) {
	t.Helper()
	root, resolveErr := filepath.EvalSymlinks(t.TempDir())
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	config := filepath.Join(root, "global.gitconfig")
	if e := os.WriteFile(config, []byte("[user]\n name = Test User\n email = test@example.test\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	h := &fakeHub{base: root, repos: map[string]Object{}}
	a, e := NewDevice(filepath.Join(root, "a"), h)
	if e != nil {
		t.Fatal(e)
	}
	b, e := NewDevice(filepath.Join(root, "b"), h)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range []*Device{a, b} {
		call(t, d, "setup", Object{"confirm_login": "alice"})
	}
	return a, b, h, root
}
func call(t *testing.T, d *Device, method string, p Object) Object {
	t.Helper()
	result, e := d.Call(context.Background(), method, p)
	if e != nil {
		t.Fatalf("%s: %v", method, e)
	}
	return result.(Object)
}
func create(t *testing.T, d *Device, root string) Object {
	t.Helper()
	return call(t, d, "create", Object{"path": filepath.Join(root, "project-a"), "name": "Demo", "date": "2026-11-06", "project_id": "P-demo"})
}
func command(t *testing.T, d *Device, action string, p Object) Object {
	t.Helper()
	request := Object{"action": action, "params": p, "date": "2026-11-06", "reason": "Test", "evidence": "Evidence", "decision": "Decision"}
	preview := call(t, d, "platform", Object{"project_id": "P-demo", "operation": "command", "args": Object{"request": request, "dry_run": true}})
	return call(t, d, "platform", Object{"project_id": "P-demo", "operation": "command", "args": Object{"request": request, "expect": preview["revision"]}})
}
func doc(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, "documents", name)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestNativeDeviceLocalOnlyAndCodeAssociation(t *testing.T) {
	a, _, _, root := setupDevices(t)
	record := create(t, a, root)
	if fsutil.Exists(filepath.Join(str(record["path"]), ".git")) {
		t.Fatal("local create initialized Git")
	}
	before, e := os.ReadFile(filepath.Join(str(record["path"]), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	result := call(t, a, "bind-code", Object{"project_id": "P-demo", "path": filepath.Join(root, "missing-code"), "kind": "directory"})
	if result["status"] != "local-only" {
		t.Fatal(result)
	}
	after, _ := os.ReadFile(filepath.Join(str(record["path"]), "state.json"))
	if string(before) != string(after) {
		t.Fatal("code binding mutated workflow")
	}
	if fsutil.Exists(filepath.Join(root, "missing-code")) {
		t.Fatal("binding created code path")
	}
	if _, e = a.Call(context.Background(), "create", Object{"path": filepath.Join(root, "other"), "name": "Other", "date": "2026-11-06", "project_id": "P-demo"}); e == nil {
		t.Fatal("duplicate Project accepted")
	}
	if _, e = a.Call(context.Background(), "bind-code", Object{"project_id": "P-demo", "path": str(record["path"])}); e == nil {
		t.Fatal("meta/code overlap accepted")
	}
}
func TestNativeGitTwoDeviceSyncAndDocumentDeletion(t *testing.T) {
	a, b, _, root := setupDevices(t)
	record := create(t, a, root)
	doc(t, str(record["path"]), "notes.md", "base\n")
	call(t, a, "enable", Object{"project_id": "P-demo"})
	imported := call(t, b, "import", Object{"repository": "alice/RumSDLC_P-demo", "path": filepath.Join(root, "project-b")})
	raw, e := os.ReadFile(filepath.Join(str(imported["path"]), "documents", "notes.md"))
	if e != nil || string(raw) != "base\n" {
		t.Fatalf("%s %v", raw, e)
	}
	doc(t, str(imported["path"]), "second.md", "B\n")
	call(t, b, "sync", Object{"project_id": "P-demo"})
	call(t, a, "sync", Object{"project_id": "P-demo"})
	if !fsutil.IsFile(filepath.Join(str(record["path"]), "documents", "second.md")) {
		t.Fatal("fast forward lost document")
	}
	if e = os.Remove(filepath.Join(str(record["path"]), "documents", "notes.md")); e != nil {
		t.Fatal(e)
	}
	call(t, a, "sync", Object{"project_id": "P-demo"})
	call(t, b, "sync", Object{"project_id": "P-demo"})
	if fsutil.Exists(filepath.Join(str(imported["path"]), "documents", "notes.md")) {
		t.Fatal("deletion did not synchronize")
	}
}
func TestNativeSyncConflictRetentionAndAcceptedRecovery(t *testing.T) {
	a, b, _, root := setupDevices(t)
	record := create(t, a, root)
	doc(t, str(record["path"]), "note.md", "base\n")
	call(t, a, "enable", Object{"project_id": "P-demo"})
	imported := call(t, b, "import", Object{"repository": "alice/RumSDLC_P-demo", "path": filepath.Join(root, "project-b")})
	doc(t, str(record["path"]), "note.md", "A\n")
	doc(t, str(imported["path"]), "note.md", "B\n")
	call(t, a, "sync", Object{"project_id": "P-demo"})
	result := call(t, b, "sync", Object{"project_id": "P-demo"})
	if result["status"] != "conflict" {
		t.Fatal(result)
	}
	conflict := result["conflict"].(Object)
	candidate := str(conflict["path"])
	if !fsutil.IsDir(candidate) {
		t.Fatal("candidate not retained")
	}
	raw, _ := os.ReadFile(filepath.Join(str(imported["path"]), "documents", "note.md"))
	if string(raw) != "B\n" {
		t.Fatal("conflict overwrote live")
	}
	if _, e := b.Call(context.Background(), "resolve", Object{"project_id": "P-demo", "conflict_id": conflict["id"]}); e == nil {
		t.Fatal("unaccepted resolution applied")
	}
	doc(t, candidate, "note.md", "accepted\n")
	call(t, b, "resolve", Object{"project_id": "P-demo", "conflict_id": conflict["id"], "accept": true})
	call(t, a, "sync", Object{"project_id": "P-demo"})
	raw, _ = os.ReadFile(filepath.Join(str(record["path"]), "documents", "note.md"))
	if string(raw) != "accepted\n" {
		t.Fatal("accepted resolution missing")
	}
}
func TestNativeBusinessMergeCommutesOnlyDisjointEntities(t *testing.T) {
	a, b, _, root := setupDevices(t)
	create(t, a, root)
	milestone := command(t, a, "milestone.create", Object{"name": "Stage"})["result"]
	call(t, a, "enable", Object{"project_id": "P-demo"})
	call(t, b, "import", Object{"repository": "alice/RumSDLC_P-demo", "path": filepath.Join(root, "project-b")})
	command(t, a, "entity.rename", Object{"table": "project", "id": "P-demo", "name": "Renamed Project"})
	command(t, b, "entity.rename", Object{"table": "milestones", "id": milestone, "name": "Renamed Stage"})
	call(t, a, "sync", Object{"project_id": "P-demo"})
	result := call(t, b, "sync", Object{"project_id": "P-demo"})
	if result["status"] != "synced" {
		t.Fatal(result)
	}
	call(t, a, "sync", Object{"project_id": "P-demo"})
	state := readState(filepath.Join(root, "project-a"))
	if state["project"].(Object)["name"] != "Renamed Project" || state["milestones"].(Object)[str(milestone)].(Object)["name"] != "Renamed Stage" {
		t.Fatal("commuting merge lost edits")
	}
}
func TestNativeRegistryCASAndDiscoveryFailure(t *testing.T) {
	a, _, hub, root := setupDevices(t)
	create(t, a, root)
	before, e := a.ReadDB()
	if e != nil {
		t.Fatal(e)
	}
	after := copyObject(before)
	after["projects"].(Object)["P-demo"].(Object)["name"] = "stale"
	command(t, a, "entity.rename", Object{"table": "project", "id": "P-demo", "name": "fresh"})
	if e = a.SaveChanges(before, after); e == nil {
		t.Fatal("stale registry commit accepted")
	}
	hub.failListing = true
	if _, e = a.Call(context.Background(), "discover", nil); e == nil {
		t.Fatal("network failure accepted")
	}
	db, e := a.ReadDB()
	if e != nil {
		t.Fatal(e)
	}
	if db["projects"].(Object)["P-demo"].(Object)["name"] != "fresh" {
		t.Fatal("stale update changed registry")
	}
}

func TestNativeWorkspaceGenerationAndCustomizationGuard(t *testing.T) {
	a, _, _, root := setupDevices(t)
	record := call(t, a, "workspace-init", Object{"parent": filepath.Join(root, "projects"), "name": "Native Workspace", "date": "2026-11-06"})
	workspace := str(record["workspace"])
	if fsutil.Exists(filepath.Join(workspace, "code", ".git")) {
		t.Fatal("placeholder code was initialized")
	}
	report := call(t, a, "doctor", Object{"project_id": record["id"]})
	if report["ok"] != true {
		t.Fatal(report)
	}
	projectContext, e := fsutil.ReadJSON(filepath.Join(workspace, ".rum-context.json"))
	if e != nil {
		t.Fatal(e)
	}
	if projectContext["runtime_kind"] != "go-native" {
		t.Fatal(projectContext)
	}
	preview := call(t, a, "project-update", Object{"project_id": record["id"], "dry_run": true})
	if len(preview["files"].([]any)) != 0 {
		t.Fatal("fresh project update is not a no-op")
	}
	path := filepath.Join(workspace, "AGENTS.md")
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	custom := append(original, []byte("\nPersonal rule\n")...)
	if e = os.WriteFile(path, custom, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Call(context.Background(), "project-update", Object{"project_id": record["id"], "expect_revision": preview["revision"]}); e == nil {
		t.Fatal("custom guidance overwritten")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(custom) {
		t.Fatal("custom guidance changed")
	}
	report = call(t, a, "doctor", Object{"project_id": record["id"]})
	if report["ok"] != true {
		t.Fatal("custom valid guidance incorrectly unhealthy", report)
	}
}
func TestNativeWorkspaceAdoptionRetainsCodeAndAuthor(t *testing.T) {
	a, _, _, root := setupDevices(t)
	code := filepath.Join(root, "Existing Code")
	if e := os.Mkdir(code, 0700); e != nil {
		t.Fatal(e)
	}
	if raw, e := exec.Command("git", "init", "-q", code).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, raw)
	}
	file := filepath.Join(code, "source.txt")
	if e := os.WriteFile(file, []byte("keep this\n"), 0600); e != nil {
		t.Fatal(e)
	}
	record := call(t, a, "workspace-adopt", Object{"path": code, "date": "2026-11-06"})
	if str(record["workspace"]) != code {
		t.Fatal("adopted directory renamed")
	}
	raw, e := os.ReadFile(filepath.Join(code, "code", "source.txt"))
	if e != nil || string(raw) != "keep this\n" {
		t.Fatalf("%s %v", raw, e)
	}
	if !fsutil.IsDir(filepath.Join(code, "code", ".git")) {
		t.Fatal("Git metadata not preserved")
	}
}
func TestNativeWorkspaceRecoveryRejectsExternalEdits(t *testing.T) {
	a, _, _, root := setupDevices(t)
	record := call(t, a, "workspace-init", Object{"parent": filepath.Join(root, "projects"), "name": "recovery", "date": "2026-11-06"})
	workspace := str(record["workspace"])
	path := filepath.Join(workspace, "AGENTS.md")
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	after := []byte("generated replacement\n")
	if e = os.WriteFile(path, []byte("external edit\n"), 0600); e != nil {
		t.Fatal(e)
	}
	updates := Object{"AGENTS.md": Object{"before": fmt.Sprintf("%x", before), "after": fmt.Sprintf("%x", after)}}
	journal := filepath.Join(workspace, ".rum-agent-update.json")
	if e = fsutil.WriteJSON(journal, updates); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Call(context.Background(), "agent-install", Object{"project_id": record["id"]}); e == nil {
		t.Fatal("external edit recovery accepted")
	}
	if !fsutil.Exists(journal) {
		t.Fatal("recovery journal removed")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "external edit\n" {
		t.Fatal("external edit lost")
	}
}

func TestNativeWorkspaceHealthDoesNotNeedPython(t *testing.T) {
	a, _, _, root := setupDevices(t)
	value := call(t, a, "workspace-init", Object{"parent": filepath.Join(root, "projects"), "name": "native-health", "date": "2026-11-06"})
	record := value
	t.Setenv("PATH", "")
	report := call(t, a, "doctor", Object{"project_id": record["id"]})
	if report["ok"] != true {
		t.Fatal(report)
	}
	call(t, a, "platform", Object{"project_id": record["id"], "operation": "check"})
}
