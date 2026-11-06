package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"rumsdlc/internal/assets"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
)

func (d *Device) CheckToolStore(ctx context.Context, path, script string, recovery bool) (err error) {
	defer guard(&err)
	root, e := filepath.Abs(path)
	must(e)
	if !fsutil.Exists(filepath.Join(filepath.Dir(root), ".rum-context.json")) {
		return nil
	}
	db, e := d.ReadDB()
	must(e)
	var record Object
	for _, v := range object(db["projects"], "projects") {
		r := object(v, "record")
		if r["path"] == root && truth(r["workspace"]) {
			record = r
			break
		}
	}
	require(record != nil, "Managed Store is not registered on this device; use its project entry")
	if !recovery {
		require(!fsutil.Exists(filepath.Join(filepath.Dir(root), ".rum-agent-update.json")), "Interrupted project update; recover with agent-install")
		if script != "workflow" {
			o := &operation{device: d, ctx: ctx, db: db}
			_, e = o.checkAgent(record)
			must(e)
		}
	}
	return nil
}

// ToolRuntime materializes embedded project scripts only when explicitly invoked.
// The native application never uses Python to implement device operations.
func (d *Device) ToolRuntime() (result string, err error) {
	defer guard(&err)
	files := map[string][]byte{}
	digests := Object{}
	for _, source := range []string{"resources/project-skill", "native-core", "docs"} {
		must(fs.WalkDir(assets.Files, source, func(path string, entry fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if entry.IsDir() {
				return nil
			}
			raw, e := assets.Files.ReadFile(path)
			if e != nil {
				return e
			}
			name := path
			if source == "native-core" {
				name = "runtime/scripts/" + strings.TrimPrefix(path, "native-core/")
			}
			files[name] = raw
			digests[name] = jsonvalue.HashBytes(raw)
			return nil
		}))
	}
	folder := filepath.Join("tool-runtime", fingerprint(digests))
	root, e := store.Safe(d.Home, folder)
	must(e)
	lock, e := fsutil.Acquire(filepath.Join(d.Home, "tool-runtime.lock"), true, true)
	must(e)
	defer lock.Close()
	if !fsutil.Exists(root) {
		parent := filepath.Dir(root)
		must(os.MkdirAll(parent, 0700))
		stage, e := os.MkdirTemp(parent, ".prepare-")
		must(e)
		defer os.RemoveAll(stage)
		for name, raw := range files {
			must(fsutil.WriteBytes(filepath.Join(stage, filepath.FromSlash(name)), raw))
		}
		must(os.Rename(stage, root))
	}
	for name, expected := range digests {
		path, e := store.Safe(root, filepath.FromSlash(name))
		must(e)
		raw, e := os.ReadFile(path)
		must(e)
		require(jsonvalue.HashBytes(raw) == expected, "Bundled project tool cache changed; retain it for inspection: "+path)
	}
	return root, nil
}
