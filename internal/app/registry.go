package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/process"
	"strings"
)

// Device is safe to share between callers: each Call owns its registry snapshot
// and releases operation locks after the final compare-and-swap persistence.
type Device struct {
	Home   string
	Hub    Hub
	Runner process.Runner
	Review ReviewReader
}

func NewDevice(home string, hub Hub) (*Device, error) {
	root, e := fsutil.RealPath(home)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	if hub == nil {
		hub = NewGitHub()
	}
	return &Device{Home: root, Hub: hub}, nil
}
func (d *Device) ReadDB() (result Object, err error) {
	defer guard(&err)
	lock, e := fsutil.Acquire(filepath.Join(d.Home, "registry.lock"), true, true)
	must(e)
	defer lock.Close()
	path := filepath.Join(d.Home, "registry.json")
	db, e := fsutil.ReadJSON(path)
	fresh := os.IsNotExist(e)
	if fresh {
		id, e := fsutil.ID()
		must(e)
		db = Object{"format": "rumsdlc-device/1", "device_id": id, "identity": nil, "projects": Object{}, "repositories": Object{}, "discovery": nil}
	} else {
		must(e)
	}
	require(db["format"] == "rumsdlc-device/1", "Unsupported device registry")
	if !has(db, "accounts") {
		db["accounts"] = Object{}
	}
	if !has(db, "meta_auth") {
		login := ""
		if db["identity"] != nil {
			login = str(object(db["identity"], "identity")["login"])
		}
		db["meta_auth"] = authentication(login)
	}
	if fresh {
		must(fsutil.WriteJSON(path, db))
	}
	return db, nil
}
func (d *Device) SaveChanges(before, after Object) (err error) {
	defer guard(&err)
	lock, e := fsutil.Acquire(filepath.Join(d.Home, "registry.lock"), true, true)
	must(e)
	defer lock.Close()
	path := filepath.Join(d.Home, "registry.json")
	current, e := fsutil.ReadJSON(path)
	must(e)
	if !has(current, "accounts") {
		current["accounts"] = Object{}
	}
	if !has(current, "meta_auth") {
		current["meta_auth"] = before["meta_auth"]
	}
	for key, value := range after {
		if equal(value, before[key]) {
			continue
		}
		if oneOf(key, "projects accounts repositories") {
			previous := getObject(before, key)
			desired := object(value, key)
			actual := object(current[key], key)
			union := Object{}
			for id := range previous {
				union[id] = true
			}
			for id := range desired {
				union[id] = true
			}
			for id := range union {
				if equal(previous[id], desired[id]) {
					continue
				}
				require(equal(actual[id], previous[id]), "Concurrent registry update; retry this operation")
				if has(desired, id) {
					actual[id] = clone(desired[id])
				} else {
					delete(actual, id)
				}
			}
		} else {
			require(equal(current[key], before[key]), "Concurrent device setting update; retry")
			current[key] = clone(value)
		}
	}
	must(fsutil.WriteJSON(path, current))
	return nil
}

type operation struct {
	device     *Device
	ctx        context.Context
	db, before Object
	locks      []*fsutil.Lock
}

func (o *operation) checkpoint() {
	must(o.device.saveChanges(o.ctx, o.before, o.db))
	o.before = copyObject(o.db)
}
func (o *operation) lock(key string) {
	lock, e := fsutil.Acquire(filepath.Join(o.device.Home, "operation-"+fingerprintBytes(key)+".lock"), true, false)
	must(e)
	o.locks = append(o.locks, lock)
}
func (o *operation) close() {
	for i := len(o.locks) - 1; i >= 0; i-- {
		o.locks[i].Close()
	}
}
func (d *Device) Call(ctx context.Context, method string, params Object) (result any, err error) {
	defer guard(&err)
	require(oneOf(method, catalog.Methods), "Unknown service method")
	lease, e := fsutil.Lease(d.Home, false)
	must(e)
	defer lease.Close()
	if params == nil {
		params = Object{}
	}
	if method == "tick" {
		return d.tick(ctx, params)
	}
	readonly := oneOf(method, "list context doctor observe-code explain identity repository-plan") || method == "platform" && oneOf(str(params["operation"]), "query check")
	op := &operation{device: d, ctx: ctx}
	defer op.close()
	topology := oneOf(method, "create register scan import bind-code") || strings.HasPrefix(method, "workspace-")
	if topology {
		lock, e := fsutil.Acquire(filepath.Join(d.Home, "bindings.lock"), true, false)
		must(e)
		op.locks = append(op.locks, lock)
	}
	key := str(params["project_id"])
	if key == "" {
		key = method
		if oneOf(method, "create register scan import") || strings.HasPrefix(method, "workspace-") {
			key = "workspace"
		}
	}
	if !readonly {
		op.lock(key)
	}
	op.db, e = d.ReadDB()
	must(e)
	op.before = copyObject(op.db)
	// Persist durable partial progress even when an operation fails, as 0.8.5 does.
	defer func() {
		if !readonly && !equal(op.before, op.db) {
			if saveErr := d.saveChanges(ctx, op.before, op.db); saveErr != nil {
				if err != nil {
					err = errors.Join(err, saveErr)
				} else {
					err = saveErr
				}
			}
		}
	}()
	result, err = op.dispatch(method, params)
	return
}
func (d *Device) tick(ctx context.Context, p Object) (any, error) {
	snapshot, e := d.ReadDB()
	if e != nil {
		return nil, e
	}
	results := []any{}
	if !has(p, "discover") || truth(p["discover"]) {
		v, e := d.Call(ctx, "discover", nil)
		if e != nil {
			results = append(results, Object{"operation": "discover", "error": e.Error()})
		} else {
			results = append(results, v)
		}
	}
	for _, id := range keys(object(snapshot["projects"], "projects")) {
		if ctx.Err() != nil {
			break
		}
		record := object(object(snapshot["projects"], "projects")[id], "record")
		if truth(record["sync_enabled"]) && !truth(record["conflict"]) {
			v, e := d.Call(ctx, "sync", Object{"project_id": id})
			if e != nil {
				results = append(results, Object{"project_id": id, "error": e.Error()})
			} else {
				results = append(results, v)
			}
		}
	}
	return Object{"results": results, "at": fsutil.Now()}, nil
}

func (d *Device) saveChanges(ctx context.Context, before, after Object) (err error) {
	defer guard(&err)
	contract(ctx, "device.save_changes", []any{before, after}, func() any { must(d.SaveChanges(before, after)); return nil })
	return nil
}
