// Package store persists workflow snapshots and free-form documents. All writes
// use the same cross-process directory lock as the project Python tools.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"rumsdlc/internal/core"
	"rumsdlc/internal/jsonvalue"
)

type Object = jsonvalue.Object
type Store struct{ Root string }

func New(root string) (*Store, error) {
	if root == "~" || strings.HasPrefix(root, "~/") {
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		if root == "~" {
			root = home
		} else {
			root = filepath.Join(home, strings.TrimPrefix(root, "~/"))
		}
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if _, e = Safe(root, ""); e != nil {
		return nil, e
	}
	return &Store{Root: root}, nil
}

// Safe rejects symlinks in existing components and paths outside the store,
// including components of a not-yet-created document path.
func Safe(root, relative string) (string, error) {
	absolute, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	// Preserve raw relative components until symlink checks have run. Cleaning
	// "link/../file" first would hide the link from the checks below.
	rawPath := absolute + string(filepath.Separator) + relative
	if filepath.IsAbs(relative) {
		rawPath = relative
	}
	for part := strings.TrimRight(rawPath, "/"+string(filepath.Separator)); part != ""; {
		info, err := os.Lstat(part)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Symlink is not allowed: %s", part)
		}
		if part == absolute {
			break
		}
		index := strings.LastIndexAny(part, "/"+string(filepath.Separator))
		if index < 0 {
			break
		}
		part = strings.TrimRight(part[:index], "/"+string(filepath.Separator))
	}
	path := filepath.Join(absolute, relative)
	if filepath.IsAbs(relative) {
		path = filepath.Clean(relative)
	}
	rel, e := filepath.Rel(absolute, path)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("Path escapes workflow root")
	}
	for part := path; ; part = filepath.Dir(part) {
		info, e := os.Lstat(part)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Symlink is not allowed: %s", part)
		}
		if part == absolute {
			break
		}
		if filepath.Dir(part) == part {
			return "", errors.New("Path escapes workflow root")
		}
	}
	return path, nil
}
func token() (string, error) {
	var raw [16]byte
	if _, e := rand.Read(raw[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(raw[:]), nil
}

func Atomic(path string, value any) error {
	data, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return atomicBytes(path, append(data, '\n'))
}
func atomicBytes(path string, data []byte) (err error) {
	suffix, e := token()
	if e != nil {
		return e
	}
	temp := path + "." + suffix + ".tmp"
	f, e := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() { f.Close(); os.Remove(temp) }()
	if _, e = f.Write(data); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(temp, path)
}
func (s *Store) Read(optional bool) (Object, error) {
	path, e := Safe(s.Root, "state.json")
	if e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) && optional {
		return nil, nil
	}
	if e != nil {
		return nil, fmt.Errorf("Missing state.json; initialize or explicitly import a project: %w", e)
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		return nil, fmt.Errorf("state.json: %w", e)
	}
	state, ok := value.(Object)
	if !ok {
		return nil, errors.New("state must be a JSON object")
	}
	if e = core.Validate(state); e != nil {
		return nil, e
	}
	return state, nil
}
func (s *Store) locked(fn func() error) (err error) {
	if _, err = Safe(s.Root, ""); err != nil {
		return err
	}
	if err = os.MkdirAll(s.Root, 0700); err != nil {
		return err
	}
	lock, err := Safe(s.Root, ".workflow.lock")
	if err != nil {
		return err
	}
	if err = os.Mkdir(lock, 0700); err != nil {
		if os.IsExist(err) {
			return errors.New("Workflow is busy; inspect lock-status and verify the writer before lock-recover")
		}
		return err
	}
	defer func() {
		e := os.Remove(filepath.Join(lock, "owner.json"))
		if e != nil && !os.IsNotExist(e) && err == nil {
			err = e
		}
		e = os.Remove(lock)
		if e != nil && err == nil {
			err = e
		}
	}()
	owner, e := token()
	if e != nil {
		return e
	}
	host, e := os.Hostname()
	if e != nil {
		return e
	}
	if e = Atomic(filepath.Join(lock, "owner.json"), Object{"token": owner, "pid": os.Getpid(), "host": host}); e != nil {
		return e
	}
	return fn()
}
func (s *Store) LockStatus() (Object, error) {
	lock, e := Safe(s.Root, ".workflow.lock")
	if e != nil {
		return nil, e
	}
	_, e = os.Stat(lock)
	if os.IsNotExist(e) {
		return Object{"locked": false}, nil
	}
	if e != nil {
		return nil, e
	}
	path, e := Safe(s.Root, ".workflow.lock/owner.json")
	if e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return Object{"locked": true, "owner": nil}, nil
	}
	if e != nil {
		return nil, e
	}
	owner, e := jsonvalue.Decode(raw)
	if e != nil {
		return nil, e
	}
	return Object{"locked": true, "owner": owner}, nil
}
func (s *Store) RecoverLock(expected string, idle bool) (Object, error) {
	if !idle {
		return nil, errors.New("Explicit confirmed_idle is required after checking all writers")
	}
	status, e := s.LockStatus()
	if e != nil {
		return nil, e
	}
	owner, ok := status["owner"].(Object)
	if status["locked"] != true || !ok || owner["token"] != expected {
		return nil, errors.New("Lock owner changed or is incomplete; inspect manually")
	}
	host, e := os.Hostname()
	if e != nil {
		return nil, e
	}
	if owner["host"] == host {
		n, ok := owner["pid"].(json.Number)
		if !ok {
			return nil, errors.New("Invalid writer PID")
		}
		pid, e := n.Int64()
		if e != nil || pid <= 0 {
			return nil, errors.New("Invalid writer PID")
		}
		running, e := processRunning(int(pid))
		if e != nil {
			return nil, errors.New("Cannot establish whether the recorded writer is running")
		}
		if running {
			return nil, errors.New("Recorded writer is still running; refusing lock recovery")
		}
	}
	// Recheck the owner immediately before removing its marker.
	current, e := s.LockStatus()
	if e != nil {
		return nil, e
	}
	if !jsonvalue.Equal(current, status) {
		return nil, errors.New("Lock owner changed or is incomplete; inspect manually")
	}
	path, e := Safe(s.Root, ".workflow.lock/owner.json")
	if e != nil {
		return nil, e
	}
	if e = os.Remove(path); e != nil {
		return nil, e
	}
	if e = os.Remove(filepath.Dir(path)); e != nil {
		return nil, e
	}
	return Object{"changed": true, "recovered": owner}, nil
}
func (s *Store) calculate(request Object, underLock bool) (before, after Object, result any, revision string, err error) {
	before, err = s.Read(request["action"] == "project.init")
	if err != nil {
		return
	}
	if before == nil {
		var entries []os.DirEntry
		entries, err = os.ReadDir(s.Root)
		if os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			return
		}
		for _, entry := range entries {
			if underLock && entry.Name() == ".workflow.lock" {
				continue
			}
			err = errors.New("Initialization requires an empty directory")
			return
		}
	}
	after, result, err = core.Dispatch(before, request)
	if err != nil {
		return
	}
	err = core.Validate(after)
	if err != nil {
		return
	}
	revision, err = jsonvalue.Hash(Object{"before": before, "request": request, "after": after})
	return
}
func (s *Store) Execute(request Object, dryRun bool, expect string) (result Object, err error) {
	if dryRun {
		before, after, id, token, e := s.calculate(request, false)
		if e != nil {
			return nil, e
		}
		return Object{"changed": false, "would_change": !jsonvalue.Equal(before, after), "revision": token, "result": id, "dry_run": true}, nil
	}
	if strings.TrimSpace(expect) == "" {
		return nil, errors.New("expect from a fresh dry-run must be nonempty text")
	}
	err = s.locked(func() error {
		before, after, id, token, e := s.calculate(request, true)
		if e != nil {
			return e
		}
		if token != expect {
			return errors.New("Stale preview or changed request; preview again, no business write performed")
		}
		changed := !jsonvalue.Equal(before, after)
		if changed {
			path, e := Safe(s.Root, "state.json")
			if e != nil {
				return e
			}
			if e = Atomic(path, after); e != nil {
				return e
			}
		}
		result = Object{"changed": changed, "revision": token, "result": id, "committed": false}
		return nil
	})
	return
}

type DocumentOptions struct {
	Table, ID, Field string
	Content          *string
	ExpectedHash     string
	Ensure           bool
	Correction       string
}

func (s *Store) Document(o DocumentOptions) (result Object, err error) {
	valid := map[string]bool{"project": true, "milestones": true, "worksets": true, "lines": true, "topics": true, "tasks": true}
	if !valid[o.Table] {
		return nil, errors.New("Unknown document entity table")
	}
	if o.Field == "" {
		o.Field = "description"
	}
	if len(o.Field) > 64 || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(o.Field) {
		return nil, errors.New("Invalid slug")
	}
	state, e := s.Read(false)
	if e != nil {
		return nil, e
	}
	var entity Object
	if o.Table == "project" {
		p := state["project"].(Object)
		if p["id"] == o.ID {
			entity = p
		}
	} else {
		entity, _ = state[o.Table].(Object)[o.ID].(Object)
	}
	if entity == nil {
		return nil, errors.New("Missing document entity")
	}
	if strings.Contains(o.ID, `\`) {
		return nil, errors.New("Invalid document entity ID")
	}
	parts := strings.Split(o.ID, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("Invalid document entity ID")
		}
	}
	if o.Table != "tasks" && len(parts) != 1 || o.Table == "tasks" && len(parts) != 2 {
		return nil, errors.New("Invalid document entity ID")
	}
	filename := map[string]string{"context": "TASK_CONTEXT", "design": "DESIGN", "prd": "PRD", "description": "DESCRIPTION"}[o.Field]
	if filename == "" {
		filename = o.Field
	}
	relative := filepath.Join("documents", o.Table, o.ID, filename+".md")
	if o.Table == "tasks" {
		relative = filepath.Join("documents", "topics", parts[0], "tasks", parts[1])
		if o.Field == "description" {
			relative += ".md"
		} else {
			relative = filepath.Join(relative, filename+".md")
		}
	}
	path, e := Safe(s.Root, relative)
	if e != nil {
		return nil, e
	}
	info := func() (Object, error) {
		path, e := Safe(s.Root, relative)
		if e != nil {
			return nil, e
		}
		raw, e := os.ReadFile(path)
		exists := e == nil
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		return Object{"path": path, "exists": exists, "hash": jsonvalue.HashBytes(raw), "metadata": Object{"id": o.ID, "name": entity["name"], "status": entity["status"]}}, nil
	}
	if o.Content == nil && !o.Ensure {
		return info()
	}
	err = s.locked(func() error {
		current, e := s.Read(false)
		if e != nil {
			return e
		}
		if !jsonvalue.Equal(current["revision"], state["revision"]) {
			return errors.New("Entity changed; inspect document again")
		}
		if entity["status"] == "closed" && strings.TrimSpace(o.Correction) == "" {
			return errors.New("Reason for correcting closed documentation must be nonempty text")
		}
		old, e := info()
		if e != nil {
			return e
		}
		if o.Ensure && old["exists"] == true {
			old["changed"] = false
			result = old
			return nil
		}
		content := "# " + o.Field + "\n\n"
		if o.Content != nil {
			if old["hash"] != o.ExpectedHash {
				return errors.New("Stale document hash; read the current file")
			}
			content = *o.Content
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		if _, e = Safe(s.Root, relative); e != nil {
			return e
		}
		if e = atomicBytes(path, []byte(content)); e != nil {
			return e
		}
		result, e = info()
		if e == nil {
			result["changed"] = true
		}
		return e
	})
	return
}

// WithLock serializes application-level metadata changes with workflow writers.
// The callback must not recursively call a writing Store method.
func (s *Store) WithLock(fn func() error) error { return s.locked(fn) }
