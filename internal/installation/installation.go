// Package installation activates native binaries transactionally and offline.
// Existing projects are never scanned or rewritten during normal activation.
package installation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/service"
	"runtime"
	"sort"
	"strings"
)

const journalName = "install-transaction.json"
const marker = "# RumSDLC managed launcher v1"

type content struct {
	Data string `json:"data"`
	Mode uint32 `json:"mode"`
}
type update struct {
	Path   string   `json:"path"`
	Before *content `json:"before"`
	After  *content `json:"after"`
}
type transaction struct {
	Format  string   `json:"format"`
	Home    string   `json:"home"`
	Phase   string   `json:"phase"`
	Updates []update `json:"updates"`
}
type Options struct {
	Destination, Home, BinDir      string
	RecoverOnly, ConfirmLegacyIdle bool
}

func safe(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Invalid installation transaction path")
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", fmt.Errorf("Invalid installation transaction path")
		}
	}
	value, e := fsutil.RealPath(path)
	if e != nil {
		return "", fmt.Errorf("Installation path contains a symlink: %s: %w", path, e)
	}
	return value, nil
}
func snapshot(path string) (*content, error) {
	path, e := safe(path)
	if e != nil {
		return nil, e
	}
	info, e := os.Stat(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Installation target is not a regular file: %s", path)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return &content{hex.EncodeToString(raw), uint32(info.Mode().Perm())}, nil
}
func contents(raw []byte, mode uint32) *content {
	// Windows exposes the read-only bit, rather than Unix executable permissions.
	if runtime.GOOS == "windows" {
		if mode&0200 != 0 {
			mode = 0666
		} else {
			mode = 0444
		}
	}
	return &content{hex.EncodeToString(raw), mode}
}
func same(a, b *content) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func replace(path string, value *content) error {
	path, e := safe(path)
	if e != nil {
		return e
	}
	if value == nil {
		e = os.Remove(path)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		return syncDir(filepath.Dir(path))
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	raw, e := hex.DecodeString(value.Data)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".rum-install-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() { f.Close(); os.Remove(name) }()
	if e = f.Chmod(os.FileMode(value.Mode)); e != nil {
		return e
	}
	if _, e = f.Write(raw); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	return syncDir(filepath.Dir(path))
}
func jsonContent(value any) *content {
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		panic(e)
	}
	return contents(append(raw, '\n'), 0600)
}
func save(home string, tx transaction) error {
	return replace(filepath.Join(home, journalName), jsonContent(tx))
}
func clear(home string) error {
	if e := os.Remove(filepath.Join(home, journalName)); e != nil {
		return e
	}
	return syncDir(home)
}
func recoverActivation(home string, explicit bool) (app.Object, error) {
	path := filepath.Join(home, journalName)
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return app.Object{"recovered": false}, nil
	}
	if e != nil {
		return nil, e
	}
	tx, _, e := parseTransaction(raw, home)
	if e != nil {
		return nil, e
	}
	if tx.Format == "rum-install-transaction/1" {
		if !explicit {
			return nil, fmt.Errorf("Legacy installation transaction requires explicit --recover-only before upgrading")
		}
		return nil, fmt.Errorf("Legacy project/service rollback requires the legacy recovery path; journal retained")
	}
	if tx.Format != "rum-runtime-activation/1" || tx.Home != home {
		return nil, fmt.Errorf("Unknown activation journal; preserve for recovery")
	}
	if tx.Phase != "applying" && tx.Phase != "committed" {
		return nil, fmt.Errorf("Unknown activation recovery phase")
	}
	if tx.Phase == "committed" {
		if e = clear(home); e != nil {
			return nil, e
		}
		return app.Object{"recovered": true, "outcome": "committed"}, nil
	}
	for _, item := range tx.Updates {
		current, e := snapshot(item.Path)
		if e != nil {
			return nil, e
		}
		if !same(current, item.Before) && !same(current, item.After) {
			return nil, fmt.Errorf("Recovery blocked by external edit: %s", item.Path)
		}
	}
	for i := len(tx.Updates) - 1; i >= 0; i-- {
		item := tx.Updates[i]
		current, e := snapshot(item.Path)
		if e != nil {
			return nil, e
		}
		if !same(current, item.Before) {
			if e = replace(item.Path, item.Before); e != nil {
				return nil, e
			}
		}
	}
	if e = clear(home); e != nil {
		return nil, e
	}
	return app.Object{"recovered": true, "outcome": "rolled-back"}, nil
}
func idle(home string, confirm bool) ([]*fsutil.Lock, error) {
	old, e := fsutil.ReadJSON(filepath.Join(home, "active.json"))
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	legacy := (old != nil && !jsonvalue.Equal(old["runtime_gate"], 1)) || (old == nil && fsutil.Exists(filepath.Join(home, "registry.json")))
	paths := []string{filepath.Join(home, "daemon.lock"), filepath.Join(home, "bindings.lock"), filepath.Join(home, "registry.lock"), filepath.Join(home, "agent-state", "git", "adapter.lock")}
	operations, e := filepath.Glob(filepath.Join(home, "operation-*.lock"))
	if e != nil {
		return nil, e
	}
	paths = append(paths, operations...)
	sort.Strings(paths)
	locks := []*fsutil.Lock{}
	fail := func(e error) ([]*fsutil.Lock, error) {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
		return nil, e
	}
	for _, path := range paths {
		if !fsutil.Exists(path) {
			continue
		}
		if _, e = safe(path); e != nil {
			return fail(e)
		}
		lock, e := fsutil.Acquire(path, true, false)
		if e != nil {
			return fail(fmt.Errorf("RumSDLC is running; close commands and stop the service before upgrading: %s", path))
		}
		locks = append(locks, lock)
	}
	if legacy {
		if !confirm {
			return fail(fmt.Errorf("Legacy runtime has no upgrade gate; close its independent scripts, then pass --confirm-legacy-idle"))
		}
		if fsutil.Exists(filepath.Join(home, "registry.json")) {
			db, e := fsutil.ReadJSON(filepath.Join(home, "registry.json"))
			if e != nil {
				return fail(e)
			}
			projects, _ := db["projects"].(map[string]any)
			for _, value := range projects {
				record, ok := value.(map[string]any)
				if !ok {
					return fail(fmt.Errorf("Invalid legacy project record"))
				}
				root, _ := record["path"].(string)
				lock := filepath.Join(root, ".workflow.lock")
				if fsutil.Exists(lock) {
					return fail(fmt.Errorf("Legacy project has an active/unresolved lock; resolve it before upgrading: %s", lock))
				}
			}
		}
	}
	return locks, nil
}
func Install(ctx context.Context, o Options) (app.Object, error) {
	for _, path := range []string{o.Destination, o.Home, o.BinDir} {
		if _, e := safe(path); e != nil {
			return nil, e
		}
	}
	if _, e := service.Endpoint(o.Home); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(o.Home, 0700); e != nil {
		return nil, e
	}
	lease, e := fsutil.Lease(o.Home, true)
	if e != nil {
		return nil, e
	}
	defer lease.Close()
	if raw, err := os.ReadFile(filepath.Join(o.Home, journalName)); err == nil {
		tx, journal, err := parseTransaction(raw, o.Home)
		if err != nil {
			return nil, err
		}
		if tx.Format == "rum-install-transaction/1" {
			if !o.RecoverOnly {
				return nil, fmt.Errorf("Legacy installation transaction requires explicit --recover-only before upgrading")
			}
			return recoverLegacy(ctx, o.Home, tx, journal)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	locks, e := idle(o.Home, o.ConfirmLegacyIdle)
	if e != nil {
		return nil, e
	}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}()
	recovered, e := recoverActivation(o.Home, o.RecoverOnly)
	if e != nil {
		return nil, e
	}
	if o.RecoverOnly {
		return recovered, nil
	}
	source, e := os.Executable()
	if e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		return nil, e
	}
	version, e := exec.CommandContext(ctx, source, "--version").Output()
	if e != nil || strings.TrimSpace(string(version)) != app.Version {
		return nil, fmt.Errorf("New runtime failed version preflight")
	}
	name := "rum"
	launcher := "launch"
	if runtime.GOOS == "windows" {
		name += ".exe"
		launcher += ".exe"
	}
	runtimePath := filepath.Join(o.Destination, name)
	if fsutil.Exists(o.Destination) {
		if e = fsutil.VerifyNativeRelease(runtimePath); e != nil {
			return nil, fmt.Errorf("Existing installation differs; install a new version in a new directory: %w", e)
		}
		current, e := os.ReadFile(runtimePath)
		if e != nil || jsonvalue.HashBytes(current) != jsonvalue.HashBytes(raw) {
			return nil, fmt.Errorf("Existing installation differs; install a new version in a new directory")
		}
	} else {
		if e = os.MkdirAll(filepath.Dir(o.Destination), 0700); e != nil {
			return nil, e
		}
		stage, e := os.MkdirTemp(filepath.Dir(o.Destination), ".rum-release-")
		if e != nil {
			return nil, e
		}
		defer os.RemoveAll(stage)
		if e = replace(filepath.Join(stage, name), contents(raw, 0755)); e != nil {
			return nil, e
		}
		if e = fsutil.WriteJSON(filepath.Join(stage, "MANIFEST.json"), app.Object{"name": "rum-sdlc-native", "version": app.Version, "files": app.Object{name: jsonvalue.HashBytes(raw)}}); e != nil {
			return nil, e
		}
		if e = os.Rename(stage, o.Destination); e != nil {
			return nil, e
		}
	}
	result, e := activate(ctx, o, runtimePath, name, launcher, raw)
	if e != nil {
		return nil, e
	}
	result["recovery"] = recovered
	return result, nil
}
func activate(ctx context.Context, o Options, runtimePath, name, launcher string, binary []byte) (app.Object, error) {
	entry := filepath.Join(o.BinDir, name)
	stable := filepath.Join(o.Home, launcher)
	old, _ := fsutil.ReadJSON(filepath.Join(o.Home, "active.json"))
	known, _ := old["native_launchers"].(map[string]any)
	for _, path := range []string{entry, stable, filepath.Join(o.Home, "launch.py"), entry + ".launch.json", stable + ".launch.json"} {
		current, e := snapshot(path)
		if e != nil {
			return nil, e
		}
		if current != nil {
			raw, _ := hex.DecodeString(current.Data)
			if !strings.Contains(string(raw), marker) && known[path] != jsonvalue.HashBytes(raw) {
				return nil, fmt.Errorf("Refusing to replace an unowned launcher: %s", path)
			}
		}
	}
	sidecar := app.Object{"format": "rum-native-launcher/1", "home": o.Home}
	legacyBridge := []byte("#!/usr/bin/env python3\n" + marker + "\nimport os,sys\nfrom pathlib import Path\np=str(Path(__file__).resolve().parent/" + strconvQuote(launcher) + ")\nos.execv(p,[p,*sys.argv[1:]])\n")
	desired := []update{{Path: stable, After: contents(binary, 0755)}, {Path: stable + ".launch.json", After: jsonContent(sidecar)}, {Path: entry, After: contents(binary, 0755)}, {Path: entry + ".launch.json", After: jsonContent(sidecar)}, {Path: filepath.Join(o.Home, "launch.py"), After: contents(legacyBridge, 0600)}}
	hashes := app.Object{}
	for _, item := range desired {
		data, err := hex.DecodeString(item.After.Data)
		if err != nil {
			return nil, err
		}
		hashes[item.Path] = jsonvalue.HashBytes(data)
	}
	active := app.Object{"version": app.Version, "runtime": runtimePath, "bin": entry, "runtime_gate": 1, "runtime_kind": "go-native", "native_launchers": hashes}
	desired = append(desired, update{Path: filepath.Join(o.Home, "active.json"), After: jsonContent(active)})
	updates := []update{}
	for _, item := range desired {
		before, e := snapshot(item.Path)
		if e != nil {
			return nil, e
		}
		item.Before = before
		if !same(before, item.After) {
			if e = probeParent(item.Path); e != nil {
				return nil, e
			}
			updates = append(updates, item)
		}
	}
	tx := transaction{"rum-runtime-activation/1", o.Home, "applying", updates}
	if e := save(o.Home, tx); e != nil {
		return nil, e
	}
	applyErr := func() error {
		for _, item := range updates {
			current, e := snapshot(item.Path)
			if e != nil {
				return e
			}
			if !same(current, item.Before) {
				return fmt.Errorf("Launcher changed during activation: %s", item.Path)
			}
			if e = testEffect(ctx, "installation.replace", []any{item.Path, item.After}, func() error { return replace(item.Path, item.After) }); e != nil {
				return e
			}
		}
		for _, item := range updates {
			current, e := snapshot(item.Path)
			if e != nil {
				return e
			}
			if !same(current, item.After) {
				return fmt.Errorf("Launcher changed before commit: %s", item.Path)
			}
		}
		tx.Phase = "committed"
		return save(o.Home, tx)
	}()
	if applyErr != nil {
		recovery, e := recoverActivation(o.Home, false)
		if e != nil {
			return nil, fmt.Errorf("Activation failed; recovery incomplete. Preserve journal and run --recover-only: %w", e)
		}
		if recovery["outcome"] != "committed" {
			return nil, fmt.Errorf("Activation failed; previous launchers restored: %w", applyErr)
		}
	}
	if fsutil.Exists(filepath.Join(o.Home, journalName)) {
		if e := testEffect(ctx, "installation.clear", []any{o.Home}, func() error { return clear(o.Home) }); e != nil {
			return nil, e
		}
	}
	pathReady := false
	for _, path := range filepath.SplitList(os.Getenv("PATH")) {
		pathReady = pathReady || path == o.BinDir
	}
	return app.Object{"installed": o.Destination, "version": app.Version, "home": o.Home, "command": entry, "program_ready": true, "service_started": false, "projects": "not-inspected-or-modified", "path_ready": pathReady, "next": "Run doctor for a selected project; use project-update --dry-run for an explicit project update. Start the service explicitly if wanted."}, nil
}
func strconvQuote(value string) string { raw, _ := json.Marshal(value); return string(raw) }
func probeParent(path string) error {
	parent := filepath.Dir(path)
	if e := os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(parent, ".rum-preflight-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write([]byte("preflight")); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Remove(name); e != nil {
		return e
	}
	return syncDir(parent)
}

// LaunchManaged holds the installation lease while the selected runtime runs.
// No shell, Python or Go runtime is needed by these native launchers.
func LaunchManaged(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) (bool, error) {
	exe, e := os.Executable()
	if e != nil {
		return false, e
	}
	sidecar := exe + ".launch.json"
	if !fsutil.Exists(sidecar) {
		return false, nil
	}
	value, e := fsutil.ReadJSON(sidecar)
	if e != nil {
		return true, e
	}
	if value["format"] != "rum-native-launcher/1" {
		return true, fmt.Errorf("Invalid native launcher metadata")
	}
	home, ok := value["home"].(string)
	if !ok {
		return true, fmt.Errorf("Invalid native launcher home")
	}
	for _, arg := range args {
		if arg == "install" {
			return true, fmt.Errorf("Run install using the unpacked release binary so the installed launcher can be replaced safely on every platform")
		}
	}
	lease, e := fsutil.Lease(home, false)
	if e != nil {
		return true, e
	}
	defer lease.Close()
	active, e := fsutil.ReadJSON(filepath.Join(home, "active.json"))
	if e != nil {
		return true, e
	}
	runtimePath, ok := active["runtime"].(string)
	if !ok || runtimePath == exe {
		return true, fmt.Errorf("Invalid active runtime")
	}
	if active["runtime_kind"] != "go-native" {
		return true, fmt.Errorf("Active runtime is not native; retain the previous launcher for legacy execution")
	}
	if e = fsutil.VerifyNativeRelease(runtimePath); e != nil {
		return true, e
	}
	hasHome := false
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		hasHome = hasHome || name == "--home" || name == "--hom" || name == "--ho"
	}
	if !hasHome {
		args = append([]string{"--home", home}, args...)
	}
	command := exec.CommandContext(ctx, runtimePath, args...)
	configureManaged(command)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = stderr
	e = command.Run()
	var exit *exec.ExitError
	if errors.As(e, &exit) {
		return true, e
	}
	return true, e
}
