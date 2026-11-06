package installation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"rumsdlc/internal/app"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/service"
	"rumsdlc/internal/store"
)

// parseTransaction validates snapshots before either recovery path touches files.
// Extra legacy journal fields remain intact for restart and lock ownership.
func parseTransaction(raw []byte, home string) (transaction, app.Object, error) {
	var tx transaction
	v, e := jsonvalue.Decode(raw)
	if e != nil {
		return tx, nil, e
	}
	journal, ok := v.(map[string]any)
	if !ok {
		return tx, nil, fmt.Errorf("Invalid installation journal")
	}
	if journal["home"] != home {
		return tx, nil, fmt.Errorf("Installation journal belongs to another home")
	}
	items, ok := journal["updates"].([]any)
	if !ok {
		return tx, nil, fmt.Errorf("Invalid installation updates")
	}
	seen := map[string]bool{}
	for _, v := range items {
		item, ok := v.(map[string]any)
		if !ok || len(item) != 3 {
			return tx, nil, fmt.Errorf("Invalid installation update")
		}
		path, ok := item["path"].(string)
		if !ok {
			return tx, nil, fmt.Errorf("Invalid installation path")
		}
		if _, e = safe(path); e != nil {
			return tx, nil, e
		}
		if seen[path] {
			return tx, nil, fmt.Errorf("Duplicate installation update: %s", path)
		}
		seen[path] = true
		for _, key := range []string{"before", "after"} {
			value, present := item[key]
			if !present {
				return tx, nil, fmt.Errorf("Missing installation snapshot")
			}
			if value == nil {
				continue
			}
			snapshot, ok := value.(map[string]any)
			if !ok || len(snapshot) != 2 {
				return tx, nil, fmt.Errorf("Invalid installation snapshot")
			}
			data, ok := snapshot["data"].(string)
			if !ok {
				return tx, nil, fmt.Errorf("Invalid installation content")
			}
			if _, e = hex.DecodeString(data); e != nil {
				return tx, nil, e
			}
			mode, ok := snapshot["mode"].(json.Number)
			if !ok {
				return tx, nil, fmt.Errorf("Invalid installation mode")
			}
			n, e := mode.Int64()
			if e != nil || n < 0 || n > 07777 {
				return tx, nil, fmt.Errorf("Invalid installation mode")
			}
		}
	}
	e = json.Unmarshal(raw, &tx)
	return tx, journal, e
}
func saveLegacy(home string, journal app.Object) error {
	return replace(filepath.Join(home, journalName), jsonContent(journal))
}
func recoverOwned(journal app.Object, clearRequired bool) error {
	owner, _ := journal["lock_owner"].(map[string]any)
	stores, _ := journal["stores"].([]any)
	for _, v := range stores {
		path, ok := v.(string)
		if !ok {
			return fmt.Errorf("Invalid recovery Store path")
		}
		if _, e := safe(path); e != nil {
			return e
		}
		s, e := store.New(path)
		if e != nil {
			return e
		}
		status, e := s.LockStatus()
		if e != nil {
			return e
		}
		saved, _ := status["owner"].(map[string]any)
		if saved != nil && owner != nil && jsonvalue.Equal(saved["pid"], owner["pid"]) && saved["host"] == owner["host"] {
			host, e := os.Hostname()
			if e != nil {
				return e
			}
			if owner["host"] != host {
				return fmt.Errorf("Cannot recover an installation lock from another host")
			}
			token, ok := saved["token"].(string)
			if !ok {
				return fmt.Errorf("Incomplete recovery Store owner")
			}
			if _, e = s.RecoverLock(token, true); e != nil {
				return e
			}
		} else if clearRequired && status["locked"] == true {
			return fmt.Errorf("Recovery blocked by an unknown/incomplete Store lock; preserve and inspect %s", path)
		}
	}
	return nil
}
func stopLegacyService(ctx context.Context, home string) error {
	// A retained daemon lock, rather than a stale socket or PID file, proves life.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if fsutil.Exists(filepath.Join(home, "service.sock")) {
		_, _ = service.Request(ctx, home, "shutdown", nil)
	}
	for {
		lock, e := fsutil.Acquire(filepath.Join(home, "daemon.lock"), true, false)
		if e == nil {
			return lock.Close()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Service still owns this device; recovery retained: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func recoverLegacy(ctx context.Context, home string, tx transaction, journal app.Object) (app.Object, error) {
	if tx.Phase != "applying" && tx.Phase != "restored" && tx.Phase != "committed" {
		return nil, fmt.Errorf("Unknown installation recovery phase")
	}
	if tx.Phase == "committed" {
		if e := clear(home); e != nil {
			return nil, e
		}
		return app.Object{"recovered": true, "outcome": "committed"}, nil
	}
	if e := stopLegacyService(ctx, home); e != nil {
		return nil, e
	}
	if tx.Phase == "applying" {
		if e := recoverOwned(journal, false); e != nil {
			return nil, e
		}
		projects := map[string]any{}
		if fsutil.Exists(filepath.Join(home, "registry.json")) {
			db, e := fsutil.ReadJSON(filepath.Join(home, "registry.json"))
			if e != nil {
				return nil, e
			}
			if db["format"] != "rumsdlc-device/1" {
				return nil, fmt.Errorf("Unsupported device registry")
			}
			var ok bool
			projects, ok = db["projects"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid registry projects")
			}
		}
		roots := []string{}
		for _, value := range projects {
			record, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid project record")
			}
			path, ok := record["path"].(string)
			if !ok {
				return nil, fmt.Errorf("Invalid project path")
			}
			if _, e := safe(path); e != nil {
				return nil, e
			}
			if fsutil.IsDir(path) {
				roots = append(roots, path)
			}
		}
		sort.Strings(roots)
		paths := []any{}
		for _, path := range roots {
			paths = append(paths, path)
		}
		host, e := os.Hostname()
		if e != nil {
			return nil, e
		}
		journal["stores"] = paths
		journal["lock_owner"] = app.Object{"pid": os.Getpid(), "host": host}
		if e = saveLegacy(home, journal); e != nil {
			return nil, e
		}
		e = legacyWriters(ctx, home, projects, roots, func() error {
			for _, item := range tx.Updates {
				current, e := snapshot(item.Path)
				if e != nil {
					return e
				}
				if !same(current, item.Before) && !same(current, item.After) {
					return fmt.Errorf("Recovery blocked by an external edit; preserve/reconcile %s", item.Path)
				}
			}
			for i := len(tx.Updates) - 1; i >= 0; i-- {
				item := tx.Updates[i]
				current, e := snapshot(item.Path)
				if e != nil {
					return e
				}
				if !same(current, item.Before) {
					if e = replace(item.Path, item.Before); e != nil {
						return e
					}
				}
			}
			journal["phase"] = "restored"
			return saveLegacy(home, journal)
		})
		if e != nil {
			return nil, e
		}
	}
	if e := recoverOwned(journal, true); e != nil {
		return nil, e
	}
	if journal["was_running"] == true {
		if e := restartNativeService(ctx, home, journal); e != nil {
			return nil, e
		}
	}
	if e := clear(home); e != nil {
		return nil, e
	}
	return app.Object{"recovered": true, "outcome": "rolled-back"}, nil
}
func legacyWriters(ctx context.Context, home string, projects map[string]any, roots []string, fn func() error) error {
	paths := []string{filepath.Join(home, "bindings.lock")}
	keys := strings.Fields(catalog.Methods + " workspace")
	for key := range projects {
		keys = append(keys, key)
	}
	unique := map[string]bool{}
	existing, e := filepath.Glob(filepath.Join(home, "operation-*.lock"))
	if e != nil {
		return e
	}
	for _, path := range existing {
		unique[path] = true
	}
	for _, key := range keys {
		unique[filepath.Join(home, "operation-"+jsonvalue.HashBytes([]byte(key))+".lock")] = true
	}
	operations := []string{}
	for path := range unique {
		operations = append(operations, path)
	}
	sort.Strings(operations)
	paths = append(paths, operations...)
	paths = append(paths, filepath.Join(home, "registry.lock"))
	locks := []*fsutil.Lock{}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}()
	for _, path := range paths {
		if _, e := safe(path); e != nil {
			return e
		}
		for {
			lock, e := fsutil.Acquire(path, true, false)
			if e == nil {
				locks = append(locks, lock)
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	var acquire func(int) error
	acquire = func(i int) error {
		if i == len(roots) {
			return fn()
		}
		s, e := store.New(roots[i])
		if e != nil {
			return e
		}
		return s.WithLock(func() error { return acquire(i + 1) })
	}
	return acquire(0)
}
func restartNativeService(ctx context.Context, home string, journal app.Object) error {
	token, e := fsutil.ID()
	if e != nil {
		return e
	}
	journal["native_restart_token"] = token
	if e = saveLegacy(home, journal); e != nil {
		return e
	}
	interval, discovery := 60, 600
	settings, _ := journal["settings"].(map[string]any)
	for name, destination := range map[string]*int{"interval": &interval, "discovery_interval": &discovery} {
		if v, ok := settings[name].(json.Number); ok {
			n, e := v.Int64()
			if e != nil {
				return e
			}
			*destination = int(n)
		}
	}
	if interval < 5 || discovery < interval {
		return fmt.Errorf("Invalid recovered service settings")
	}
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	log, e := os.OpenFile(filepath.Join(home, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.Command(executable, "--home", home, "daemon", "run", "--interval", strconv.Itoa(interval), "--discovery-interval", strconv.Itoa(discovery), "--installation-recovery", token)
	cmd.Stdout = log
	cmd.Stderr = log
	detachRecovery(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for i := 0; i < 100; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return fmt.Errorf("Recovered service did not become ready; inspect service.log")
		case <-time.After(50 * time.Millisecond):
		}
		requestCtx, cancel := context.WithTimeout(ctx, time.Second)
		result, e := service.Request(requestCtx, home, "installation-ready", nil)
		cancel()
		if e == nil {
			value, ok := result.(map[string]any)
			if ok && value["token"] == token {
				return nil
			}
		}
	}
	return fmt.Errorf("Recovered service did not become ready; journal retained")
}
