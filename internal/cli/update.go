package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/installation"
	"rumsdlc/internal/release"
	"rumsdlc/internal/update"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func updateCommand(ctx context.Context, home, method string, args []string, out io.Writer) (any, error) {
	m := update.New(home)
	switch method {
	case "version":
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			return nil, flag.ErrHelp
		}
		if len(args) > 0 {
			return nil, fmt.Errorf("version takes no options")
		}
		return release.Current(), nil
	case "update-helper-capabilities":
		return map[string]any{"format": "rum-update-helper/1", "request": "rum-update-operation/1", "expected_active": true}, nil
	case "update-apply":
		p, e := parseOptions(args, catalog.Parse("operation! nonce!"))
		if e != nil {
			return nil, e
		}
		return nil, applyUpdate(ctx, m, p["operation"].(string), p["nonce"].(string))
	case "update-status":
		p, e := parseOptions(args, catalog.Parse("operation"))
		if e != nil {
			return nil, e
		}
		id, _ := p["operation"].(string)
		return update.Status(home, id)
	case "update":
		p, e := parseOptions(args, catalog.Parse("channel +allow-channel-switch +allow-downgrade"))
		if e != nil {
			return nil, e
		}
		channel, _ := p["channel"].(string)
		if channel != "" && channel != "stable" {
			return nil, fmt.Errorf("Only stable releases are supported")
		}
		return prepareUpdate(ctx, m, p)
	}
	return nil, fmt.Errorf("Unknown update command")
}
func binaryHash(p string) (string, error) {
	raw, e := os.ReadFile(p)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:]), nil
}
func prepareUpdate(ctx context.Context, m *update.Manager, p map[string]any) (any, error) {
	lock, e := fsutil.Acquire(filepath.Join(m.Home, "update-apply.lock"), true, false)
	if e != nil {
		return nil, fmt.Errorf("Another update is preparing or installing")
	}
	defer lock.Close()
	previous, e := m.Operation("")
	if e != nil {
		return nil, e
	}
	if previous.ID != "" && !previous.Terminal() {
		return map[string]any{"status": previous.Phase, "operation_id": previous.ID, "next": "rum update-status"}, nil
	}
	activePath := filepath.Join(m.Home, "active.json")
	active, e := fsutil.ReadJSON(activePath)
	if e != nil {
		return nil, fmt.Errorf("No managed installation; use the original binary's install command")
	}
	expected, e := binaryHash(activePath)
	if e != nil {
		return nil, e
	}
	s, e := m.Check(ctx)
	if e != nil {
		return nil, e
	}
	if s.Target == nil {
		return nil, fmt.Errorf("No release available")
	}
	if s.Status == "up-to-date" {
		return map[string]any{"status": "up-to-date"}, nil
	}
	if m.Current.Channel != "stable" && (p["allow_channel_switch"] != true || p["channel"] != "stable") {
		return nil, fmt.Errorf("Development/unknown build: select --channel stable --allow-channel-switch explicitly")
	}
	cmp, e := release.Compare(s.Target.Version(), m.Current.Version)
	if e != nil {
		return nil, e
	}
	if cmp < 0 && p["allow_downgrade"] != true {
		return nil, fmt.Errorf("Refusing downgrade; use --allow-downgrade explicitly")
	}
	daemon, e := fsutil.Acquire(filepath.Join(m.Home, "daemon.lock"), true, false)
	if e != nil {
		return nil, fmt.Errorf("Service is running; run rum daemon stop, then retry update")
	}
	daemon.Close()
	id, e := fsutil.ID()
	if e != nil {
		return nil, e
	}
	nonce, e := fsutil.ID()
	if e != nil {
		return nil, e
	}
	request, e := m.OperationPath(id)
	if e != nil {
		return nil, e
	}
	dir := filepath.Dir(request)
	binary, sums, e := m.Client.Download(ctx, *s.Target, runtime.GOOS, runtime.GOARCH, dir)
	if e != nil {
		return nil, e
	}
	if e = m.Client.Confirm(ctx, *s.Target); e != nil {
		return nil, e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(probeCtx, binary, "version").Output()
	if e != nil {
		return nil, fmt.Errorf("Release lacks safe updater support; retain %s and run its original install command manually", binary)
	}
	var identity struct {
		OK     bool             `json:"ok"`
		Result release.Identity `json:"result"`
	}
	if json.Unmarshal(raw, &identity) != nil || !identity.OK || identity.Result.Version != s.Target.Version() || identity.Result.Channel != "stable" || identity.Result.Validate() != nil {
		return nil, fmt.Errorf("Downloaded program identity does not match selected stable release")
	}
	if sums.Commit != "" && sums.Commit != identity.Result.Commit {
		return nil, fmt.Errorf("Downloaded program commit does not match checksums")
	}
	caps, e := exec.CommandContext(probeCtx, binary, "update-helper-capabilities").Output()
	if e != nil || !strings.Contains(string(caps), "rum-update-operation/1") {
		return nil, fmt.Errorf("Release does not support safe update handoff")
	}
	source, _ := active["runtime"].(string)
	entry, _ := active["bin"].(string)
	if source == "" || entry == "" {
		return nil, fmt.Errorf("Installation paths are missing")
	}
	root := filepath.Dir(filepath.Dir(source))
	destination := filepath.Join(root, s.Target.Version())
	if fsutil.Overlap(destination, m.Home) {
		return nil, fmt.Errorf("Invalid installation layout")
	}
	hash, e := binaryHash(binary)
	if e != nil {
		return nil, e
	}
	launcher, _ := strconv.Atoi(os.Getenv("RUMSDLC_LAUNCHER_PID"))
	o := update.Operation{Format: "rum-update-operation/1", ID: id, Nonce: nonce, Home: m.Home, ExpectedActive: expected, Binary: binary, BinaryHash: hash, Target: identity.Result, Destination: destination, BinDir: filepath.Dir(entry), Parent: os.Getpid(), Launcher: launcher, Phase: "queued"}
	if e = m.SaveOperation(o); e != nil {
		return nil, e
	}
	if e = m.SetCurrent(id); e != nil {
		return nil, e
	}
	log, e := os.OpenFile(filepath.Join(dir, "helper.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	defer log.Close()
	cmd := exec.Command(binary, "--home", m.Home, "update-apply", "--operation", id, "--nonce", nonce)
	detach(cmd)
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		o.Phase = "failed"
		o.Error = e.Error()
		_ = m.SaveOperation(o)
		return nil, e
	}
	_ = cmd.Process.Release()
	return map[string]any{"status": "queued", "operation_id": id, "target": o.Target, "next": "rum update-status --operation " + id}, nil
}
func applyUpdate(ctx context.Context, m *update.Manager, id, nonce string) (err error) {
	lock, e := fsutil.Acquire(filepath.Join(m.Home, "update-apply.lock"), true, true)
	if e != nil {
		return e
	}
	defer lock.Close()
	o, e := m.Operation(id)
	if e != nil {
		return e
	}
	if nonce == "" || o.Nonce != nonce || o.Terminal() {
		return fmt.Errorf("Invalid or already consumed update request")
	}
	defer func() {
		if err != nil {
			if o.Phase != "verification-failed" {
				o.Phase = "failed"
			}
			if fsutil.Exists(filepath.Join(m.Home, "install-transaction.json")) {
				o.Phase = "recovery-required"
			}
			o.Error = err.Error()
			_ = m.SaveOperation(o)
		}
	}()
	actual, e := os.Executable()
	if e != nil {
		return e
	}
	actual, e = fsutil.RealPath(actual)
	if e != nil {
		return e
	}
	expected, e := fsutil.RealPath(o.Binary)
	if e != nil {
		return e
	}
	p, e := m.OperationPath(id)
	if e != nil {
		return e
	}
	if actual != expected || !fsutil.Contains(filepath.Dir(p), actual) {
		return fmt.Errorf("Update helper must run from its own validated operation directory")
	}
	hash, e := binaryHash(actual)
	if e != nil || hash != o.BinaryHash {
		return fmt.Errorf("Update helper changed")
	}
	if release.Current() != o.Target {
		return fmt.Errorf("Update helper build identity changed")
	}
	o.Phase = "waiting"
	if e = m.SaveOperation(o); e != nil {
		return e
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !processAlive(o.Parent) && !processAlive(o.Launcher) {
			gate, e := fsutil.Lease(m.Home, true)
			if e == nil {
				gate.Close()
				break
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Runtime or service still running; close commands and retry using the verified original installer")
		}
		time.Sleep(100 * time.Millisecond)
	}
	o.Phase = "installing"
	if e = m.SaveOperation(o); e != nil {
		return e
	}
	_, e = installation.Install(ctx, installation.Options{Destination: o.Destination, Home: m.Home, BinDir: o.BinDir, ExpectedActive: o.ExpectedActive})
	if e != nil {
		return e
	}
	name := "rum"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	probe, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(probe, filepath.Join(o.BinDir, name), "version").Output()
	if e != nil {
		o.Phase = "verification-failed"
		return fmt.Errorf("New entrypoint verification failed: %w", e)
	}
	var info struct {
		OK     bool             `json:"ok"`
		Result release.Identity `json:"result"`
	}
	if json.Unmarshal(raw, &info) != nil || !info.OK || info.Result != o.Target {
		o.Phase = "verification-failed"
		return fmt.Errorf("New entrypoint identity differs from target")
	}
	o.Phase = "installed"
	o.Nonce = ""
	o.Error = ""
	return m.SaveOperation(o)
}

// Status remains available during an interrupted activation. This whitelist
// reads only device update files; it never launches a runtime or bypasses a
// lease for any mutating command.
func LocalUpdateStatus(args []string, out io.Writer) (bool, error) {
	home := os.Getenv("RUMSDLC_HOME")
	if home == "" {
		u, e := os.UserHomeDir()
		if e != nil {
			return false, e
		}
		home = filepath.Join(u, ".rumsdlc")
	}
	a := append([]string(nil), args...)
	if len(a) > 0 && strings.HasPrefix(a[0], "--home=") {
		home = strings.TrimPrefix(a[0], "--home=")
		a = a[1:]
	} else if len(a) > 1 && (a[0] == "--home" || a[0] == "--hom" || a[0] == "--ho") {
		home = a[1]
		a = a[2:]
	}
	if len(a) == 0 || a[0] != "update-status" {
		return false, nil
	}
	for _, arg := range a[1:] {
		if arg == "--help" || arg == "-h" {
			return false, nil
		}
	}
	root, e := fsutil.RealPath(home)
	if e != nil {
		return true, e
	}
	p, e := parseOptions(a[1:], catalog.Parse("operation"))
	if e != nil {
		return true, e
	}
	id, _ := p["operation"].(string)
	v, e := update.Status(root, id)
	if e != nil {
		return true, e
	}
	return true, json.NewEncoder(out).Encode(map[string]any{"ok": true, "result": v})
}
