package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/release"
	"rumsdlc/internal/update"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeHelperWaitsForLeaseAndVerifiesInstalledIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("native process integration")
	}
	tempRoot := ""
	if runtime.GOOS == "darwin" {
		tempRoot = "/private/tmp"
	}
	temp, e := os.MkdirTemp(tempRoot, "ru-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(temp)
	temp, e = filepath.EvalSymlinks(temp)
	if e != nil {
		t.Fatal(e)
	}
	name := "rum"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(temp, name)
	source := filepath.Join("..", "..")
	commit := strings.Repeat("a", 40)
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-X rumsdlc/internal/release.Channel=stable -X rumsdlc/internal/release.Commit="+commit, "-o", binary, "./cmd/rum")
	build.Dir = source
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", out, e)
	}
	home := filepath.Join(temp, "home")
	bin := filepath.Join(temp, "bin")
	old := filepath.Join(temp, "versions", "old")
	if out, e := exec.Command(binary, "--home", home, "install", "--destination", old, "--bin-dir", bin).CombinedOutput(); e != nil {
		t.Fatalf("install: %s %v", out, e)
	}
	m := update.New(home)
	id := strings.Repeat("b", 32)
	p, e := m.OperationPath(id)
	if e != nil {
		t.Fatal(e)
	}
	helper := filepath.Join(filepath.Dir(p), name)
	raw, e := os.ReadFile(binary)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(helper, raw, 0700); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	active, e := binaryHash(filepath.Join(home, "active.json"))
	if e != nil {
		t.Fatal(e)
	}
	o := update.Operation{Format: "rum-update-operation/1", ID: id, Nonce: strings.Repeat("c", 32), Home: home, ExpectedActive: active, Binary: helper, BinaryHash: hex.EncodeToString(sum[:]), Target: release.Identity{Format: "rum-build/1", Version: release.Version, Channel: "stable", Commit: commit}, Destination: filepath.Join(temp, "versions", release.Version), BinDir: bin, Phase: "queued"}
	if e = m.SaveOperation(o); e != nil {
		t.Fatal(e)
	}
	if e = m.SetCurrent(id); e != nil {
		t.Fatal(e)
	}
	lease, e := fsutil.Lease(home, false)
	if e != nil {
		t.Fatal(e)
	}
	defer lease.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "--home", home, "update-apply", "--operation", id, "--nonce", o.Nonce)
	log := filepath.Join(temp, "out")
	f, e := os.Create(log)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	cmd.Stdout = f
	cmd.Stderr = f
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(250 * time.Millisecond)
	current, e := binaryHash(filepath.Join(home, "active.json"))
	if e != nil || current != active {
		t.Fatal("installation changed under held lease", e)
	}
	lease.Close()
	if e = cmd.Wait(); e != nil {
		out, _ := os.ReadFile(log)
		t.Fatalf("helper: %s %v", out, e)
	}
	result, e := m.Operation(id)
	if e != nil || result.Phase != "installed" {
		t.Fatal(result, e)
	}
	probe, e := exec.Command(filepath.Join(bin, name), "version").Output()
	if e != nil {
		t.Fatal(e)
	}
	var info struct {
		Result release.Identity `json:"result"`
	}
	if e = json.Unmarshal(probe, &info); e != nil || info.Result != o.Target {
		t.Fatal(string(probe), e)
	}
	// The native stable launcher itself must allow read-only status during recovery.
	if e = os.WriteFile(filepath.Join(home, "install-transaction.json"), []byte(`{"pending":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(filepath.Join(bin, name), "update-status").CombinedOutput(); e != nil {
		t.Fatalf("status unavailable during recovery: %s %v", out, e)
	}
	if out, e := exec.Command(helper, "--home", home, "update-apply", "--operation", id, "--nonce", o.Nonce).CombinedOutput(); e == nil {
		t.Fatalf("consumed operation replayed: %s", out)
	}
}

func TestAbandonedPreparationCanBeRetried(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	home := filepath.Join(root, "d")
	if e = os.MkdirAll(home, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(home, "active.json"), []byte(`{"runtime":"old","bin":"old"}`), 0600); e != nil {
		t.Fatal(e)
	}
	m := update.New(home)
	m.Current = release.Identity{Version: "1.2.3", Channel: "stable"}
	id := strings.Repeat("d", 32)
	if e = m.SaveOperation(update.Operation{Format: "rum-update-operation/1", Home: home, ID: id, Phase: "queued"}); e != nil {
		t.Fatal(e)
	}
	m.SetCurrent(id)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(update.Target{ID: 1, Tag: "v1.2.3", Assets: []update.Asset{{Name: "checksums.json"}, {Name: update.PackageName("1.2.3", runtime.GOOS, runtime.GOARCH)}}})
	}))
	defer server.Close()
	m.Client = &update.Client{Base: server.URL, HTTP: server.Client()}
	if _, e = prepareUpdate(context.Background(), m, map[string]any{}); e != nil {
		t.Fatal(e)
	}
	o, e := m.Operation(id)
	if e != nil || o.Phase != "failed" {
		t.Fatal(o, e)
	}
}
