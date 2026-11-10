package installation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/store"
)

func fixture(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func put(t *testing.T, path string, raw []byte) {
	t.Helper()
	if e := replace(path, contents(raw, 0600)); e != nil {
		t.Fatal(e)
	}
}
func TestActivationRollbackAndExternalEdits(t *testing.T) {
	root := fixture(t)
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0700)
	a, b := filepath.Join(home, "active.json"), filepath.Join(root, "bin", "rum")
	before, after := contents([]byte("old"), 0600), contents([]byte("new"), 0600)
	for _, p := range []string{a, b} {
		if e := replace(p, after); e != nil {
			t.Fatal(e)
		}
	}
	tx := transaction{"rum-runtime-activation/1", home, "applying", []update{{a, before, after}, {b, nil, after}}}
	if e := save(home, tx); e != nil {
		t.Fatal(e)
	}
	put(t, b, []byte("external"))
	if _, e := recoverActivation(home, true); e == nil || !strings.Contains(e.Error(), "external edit") {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(a)
	if string(raw) != "new" {
		t.Fatal("partial rollback before validating all snapshots")
	}
	if e := replace(b, after); e != nil {
		t.Fatal(e)
	}
	result, e := recoverActivation(home, true)
	if e != nil || result["outcome"] != "rolled-back" {
		t.Fatal(result, e)
	}
	raw, _ = os.ReadFile(a)
	if string(raw) != "old" || fsutil.Exists(b) {
		t.Fatal("incorrect rollback")
	}
	result, e = recoverActivation(home, true)
	if e != nil || result["recovered"] != false {
		t.Fatal(result, e)
	}
}
func TestMalformedJournalCannotDeleteTargets(t *testing.T) {
	home := fixture(t)
	path := filepath.Join(home, "entry")
	put(t, path, []byte("keep"))
	raw := []byte(`{"format":"rum-runtime-activation/1","home":` + strconvQuote(home) + `,"phase":"applying","updates":[{"path":` + strconvQuote(path) + `,"after":null}]}`)
	put(t, filepath.Join(home, journalName), raw)
	if _, e := recoverActivation(home, true); e == nil {
		t.Fatal("missing before accepted")
	}
	value, _ := os.ReadFile(path)
	if string(value) != "keep" {
		t.Fatal("target deleted")
	}
}
func TestActivationDoesNotReadProjectsAndRejectsUnownedSidecar(t *testing.T) {
	root := fixture(t)
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	os.MkdirAll(home, 0700)
	put(t, filepath.Join(home, "registry.json"), []byte("broken registry remains user data"))
	opts := Options{Home: home, BinDir: bin, Destination: filepath.Join(root, "release")}
	put(t, filepath.Join(bin, "rum.launch.json"), []byte("unowned"))
	if _, e := activate(context.Background(), opts, filepath.Join(opts.Destination, "rum"), "rum", "launch", []byte("native fixture")); e == nil {
		t.Fatal("overwrote unowned sidecar")
	}
	os.Remove(filepath.Join(bin, "rum.launch.json"))
	result, e := activate(context.Background(), opts, filepath.Join(opts.Destination, "rum"), "rum", "launch", []byte("native fixture"))
	if e != nil || result["projects"] != "not-inspected-or-modified" {
		t.Fatal(result, e)
	}
	before, _ := os.ReadFile(filepath.Join(home, "registry.json"))
	if string(before) != "broken registry remains user data" {
		t.Fatal("changed registry")
	}
	opts.Destination = filepath.Join(root, "new")
	if _, e = activate(context.Background(), opts, filepath.Join(opts.Destination, "rum"), "rum", "launch", []byte("new native fixture")); e != nil {
		t.Fatal(e)
	}
}
func TestLegacyRollbackOwnLocksAndUnknownLockProtection(t *testing.T) {
	for _, phase := range []string{"applying", "restored"} {
		t.Run(phase, func(t *testing.T) {
			root := fixture(t)
			home := filepath.Join(root, "home")
			meta := filepath.Join(root, "meta")
			os.MkdirAll(home, 0700)
			os.MkdirAll(meta, 0700)
			target := filepath.Join(root, "resource")
			before, after := contents([]byte("old"), 0600), contents([]byte("new"), 0600)
			if phase == "restored" {
				replace(target, before)
			} else {
				replace(target, after)
			}
			host, _ := os.Hostname()
			owner := app.Object{"host": host, "pid": 99999999, "token": "dead-writer"}
			os.Mkdir(filepath.Join(meta, ".workflow.lock"), 0700)
			store.Atomic(filepath.Join(meta, ".workflow.lock", "owner.json"), owner)
			fsutil.WriteJSON(filepath.Join(home, "registry.json"), app.Object{"format": "rumsdlc-device/1", "projects": app.Object{"id": app.Object{"path": meta}}})
			journal := app.Object{"format": "rum-install-transaction/1", "home": home, "phase": phase, "updates": []any{app.Object{"path": target, "before": app.Object{"data": before.Data, "mode": before.Mode}, "after": app.Object{"data": after.Data, "mode": after.Mode}}}, "stores": []any{meta}, "lock_owner": owner, "was_running": false}
			if e := saveLegacy(home, journal); e != nil {
				t.Fatal(e)
			}
			raw, _ := os.ReadFile(filepath.Join(home, journalName))
			tx, parsed, e := parseTransaction(raw, home)
			if e != nil {
				t.Fatal(e)
			}
			result, e := recoverLegacy(context.Background(), home, tx, parsed)
			if e != nil || result["outcome"] != "rolled-back" {
				t.Fatal(result, e)
			}
			actual, _ := snapshot(target)
			if !same(actual, before) {
				t.Fatal("rollback mismatch")
			}
			if fsutil.Exists(filepath.Join(meta, ".workflow.lock")) {
				t.Fatal("dead lock retained")
			}
			// A restored journal does not grant authority over a foreign lock or edits.
			os.Mkdir(filepath.Join(meta, ".workflow.lock"), 0700)
			store.Atomic(filepath.Join(meta, ".workflow.lock", "owner.json"), app.Object{"host": host, "pid": 99999998, "token": "foreign"})
			journal["phase"] = "restored"
			saveLegacy(home, journal)
			raw, _ = os.ReadFile(filepath.Join(home, journalName))
			tx, parsed, _ = parseTransaction(raw, home)
			if _, e = recoverLegacy(context.Background(), home, tx, parsed); e == nil || !strings.Contains(e.Error(), "unknown/incomplete") {
				t.Fatal(e)
			}
			if !fsutil.Exists(filepath.Join(home, journalName)) {
				t.Fatal("cleared blocked recovery")
			}
		})
	}
}
func TestCommittedRecoveryOnlyClearsJournal(t *testing.T) {
	home := fixture(t)
	path := filepath.Join(home, "entry")
	put(t, path, []byte("later edit"))
	save(home, transaction{"rum-runtime-activation/1", home, "committed", []update{{path, contents([]byte("old"), 0600), contents([]byte("new"), 0600)}}})
	r, e := recoverActivation(home, true)
	if e != nil || r["outcome"] != "committed" {
		t.Fatal(r, e)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "later edit" {
		t.Fatal("reverted committed activation")
	}
}
func TestSnapshotPreservesMode(t *testing.T) {
	home := fixture(t)
	path := filepath.Join(home, "entry")
	value := contents([]byte("native"), 0755)
	if e := replace(path, value); e != nil {
		t.Fatal(e)
	}
	actual, e := snapshot(path)
	if e != nil || !same(actual, value) {
		t.Fatal(actual, e)
	}
	decoded, _ := hex.DecodeString(actual.Data)
	var tx transaction
	raw, _ := json.Marshal(transaction{"rum-runtime-activation/1", home, "applying", []update{{path, nil, value}}})
	if e = json.Unmarshal(raw, &tx); e != nil || string(decoded) != "native" {
		t.Fatal(e)
	}
}

func TestExpectedActiveRejectsStaleUpdateBeforeActivation(t *testing.T) {
	root := ""
	if runtime.GOOS == "darwin" {
		root = "/private/tmp"
	}
	home, e := os.MkdirTemp(root, "ru-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(home)
	home, e = filepath.EvalSymlinks(home)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(home, "active.json"), []byte(`{"version":"changed"}`), 0600); e != nil {
		t.Fatal(e)
	}
	_, e = Install(context.Background(), Options{Home: home, Destination: filepath.Join(home, "payload"), BinDir: filepath.Join(home, "bin"), ExpectedActive: "stale"})
	if e == nil || !strings.Contains(e.Error(), "Active installation changed") {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(home, "install-transaction.json")); !os.IsNotExist(e) {
		t.Fatal("stale update entered activation")
	}
}
