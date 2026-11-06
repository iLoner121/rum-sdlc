// Package migration imports one explicitly reconciled legacy snapshot into a
// new empty Store. Source files are captured read-only and preserved verbatim.
package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"sort"
	"strings"
)

// writeState is the final persistence boundary, kept replaceable by package tests.
var writeState = store.Atomic

type Options struct {
	Source, Destination, ProjectID, Name, Date, ActiveMilestone, Expected string
	DryRun                                                                bool
}

func Prepare(o Options) (core.LegacyResult, error) {
	source, e := fsutil.RealPath(o.Source)
	if e != nil {
		return core.LegacyResult{}, e
	}
	cfg, e := fsutil.ReadJSON(filepath.Join(source, "workflow", "config.json"))
	if e != nil {
		return core.LegacyResult{}, e
	}
	input := core.LegacySource{Root: filepath.ToSlash(source), Config: cfg, Records: map[string][]core.LegacyRecord{}, Files: map[string][]byte{}, Executable: map[string]bool{}}
	for _, item := range []struct{ collection, kind, filename string }{{"topics", "topic", "topic.json"}, {"milestones", "milestone", "milestone.json"}, {"iterations", "iteration", "iteration.json"}, {"lines", "line", "line.json"}} {
		folder, e := store.Safe(source, "workflow/"+item.collection)
		if e != nil {
			return core.LegacyResult{}, e
		}
		entries, e := os.ReadDir(folder)
		if os.IsNotExist(e) && item.kind == "line" {
			continue
		}
		if e != nil {
			return core.LegacyResult{}, fmt.Errorf("Missing legacy collection: %s: %w", item.collection, e)
		}
		for _, entry := range entries {
			root, e := store.Safe(source, filepath.Join("workflow", item.collection, entry.Name()))
			if e != nil {
				return core.LegacyResult{}, e
			}
			if !entry.IsDir() && (entry.Name() == ".gitkeep" || entry.Name() == ".DS_Store") {
				continue
			}
			if !entry.IsDir() {
				return core.LegacyResult{}, fmt.Errorf("Incomplete legacy record: %s", root)
			}
			metadata, e := store.Safe(source, filepath.Join("workflow", item.collection, entry.Name(), item.filename))
			if e != nil {
				return core.LegacyResult{}, e
			}
			if item.kind == "topic" && fsutil.Exists(filepath.Join(root, "item.json")) {
				return core.LegacyResult{}, fmt.Errorf("Ambiguous topic.json/item.json source metadata")
			}
			data, e := fsutil.ReadJSON(metadata)
			if e != nil {
				return core.LegacyResult{}, fmt.Errorf("Incomplete legacy record: %s: %w", root, e)
			}
			if e = core.ValidateLegacyRecord(data, item.kind, entry.Name(), stringValue(cfg["main_branch"])); e != nil {
				return core.LegacyResult{}, e
			}
			relative, _ := filepath.Rel(source, metadata)
			input.Records[item.kind] = append(input.Records[item.kind], core.LegacyRecord{Path: filepath.ToSlash(relative), Data: data})
			if item.kind == "topic" && fsutil.Exists(filepath.Join(root, "tasks")) {
				tasks, e := os.ReadDir(filepath.Join(root, "tasks"))
				if e != nil {
					return core.LegacyResult{}, e
				}
				for _, task := range tasks {
					if task.IsDir() || !regexp.MustCompile(`^t[0-9]{3,}\.md$`).MatchString(task.Name()) {
						return core.LegacyResult{}, fmt.Errorf("Invalid legacy Task file")
					}
				}
			}
		}
	}
	capture := func(path string) error {
		relative, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		if _, e = store.Safe(source, relative); e != nil {
			return e
		}
		info, e := os.Stat(path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Import only regular files")
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(relative)
		input.Files[name] = raw
		if info.Mode().Perm()&0111 != 0 {
			input.Executable[name] = true
		}
		return nil
	}
	e = filepath.WalkDir(filepath.Join(source, "workflow"), func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		return capture(path)
	})
	if e != nil {
		return core.LegacyResult{}, e
	}
	for _, name := range []string{"AGENTS.md", "PROJECT_CONTEXT.md"} {
		path := filepath.Join(source, name)
		if fsutil.Exists(path) {
			if e = capture(path); e != nil {
				return core.LegacyResult{}, e
			}
		}
	}
	return core.ConvertLegacy(input, o.ProjectID, o.Name, o.Date, o.ActiveMilestone)
}
func stringValue(v any) string { s, _ := v.(string); return s }

// Run binds preview to all source bytes, executable flags and import arguments.
func Run(o Options) (result jsonvalue.Object, err error) {
	prepared, e := Prepare(o)
	if e != nil {
		return nil, e
	}
	hashes := jsonvalue.Object{}
	for name, data := range prepared.Files {
		hashes[name] = jsonvalue.HashBytes(data)
	}
	revision, e := jsonvalue.Hash(jsonvalue.Object{"state": prepared.State, "files": hashes})
	if e != nil {
		return nil, e
	}
	destination, e := store.New(o.Destination)
	if e != nil {
		return nil, e
	}
	empty := func(locked bool) error {
		entries, e := os.ReadDir(destination.Root)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if locked && entry.Name() == ".workflow.lock" {
				continue
			}
			return fmt.Errorf("Import requires an empty new destination; never overwrite")
		}
		return nil
	}
	if e = empty(false); e != nil {
		return nil, e
	}
	result = jsonvalue.Clone(prepared.Report).(map[string]any)
	result["revision"] = revision
	if o.DryRun {
		counts := jsonvalue.Object{}
		for _, table := range []string{"lines", "milestones", "tasks", "topics", "worksets"} {
			counts[table] = len(prepared.State[table].(map[string]any))
		}
		result["changed"] = false
		result["counts"] = counts
		return result, nil
	}
	if o.Expected != revision {
		return nil, fmt.Errorf("Source or request changed; preview import again")
	}
	e = destination.WithLock(func() (err error) {
		if err = empty(true); err != nil {
			return err
		}
		created := []string{}
		defer func() {
			if err == nil {
				return
			}
			for i := len(created) - 1; i >= 0; i-- {
				_ = os.Remove(created[i])
			}
			folders := []string{}
			_ = filepath.WalkDir(destination.Root, func(path string, entry fs.DirEntry, e error) error {
				if e == nil && entry.IsDir() && path != destination.Root && !strings.HasSuffix(path, string(filepath.Separator)+".workflow.lock") {
					folders = append(folders, path)
				}
				return nil
			})
			sort.Slice(folders, func(i, j int) bool { return len(folders[i]) > len(folders[j]) })
			for _, path := range folders {
				_ = os.Remove(path)
			}
		}()
		names := []string{}
		for name := range prepared.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		executable := map[string]bool{}
		for _, v := range prepared.Report["executable_files"].([]any) {
			executable[v.(string)] = true
		}
		for _, name := range names {
			path, e := store.Safe(destination.Root, filepath.FromSlash(name))
			if e != nil {
				return e
			}
			if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return e
			}
			f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			created = append(created, path)
			_, e = f.Write(prepared.Files[name])
			if e == nil {
				e = f.Sync()
			}
			closeErr := f.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			mode := os.FileMode(0644)
			if executable[name] {
				mode = 0755
			}
			if e = os.Chmod(path, mode); e != nil {
				return e
			}
		}
		return writeState(filepath.Join(destination.Root, "state.json"), prepared.State)
	})
	if e != nil {
		return nil, e
	}
	result["changed"] = true
	result["committed"] = false
	return result, nil
}
