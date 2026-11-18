package app

import (
	"fmt"
	"os"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/store"
	"time"
)

// Reader never initializes device or project storage. RemoteHub is an optional
// read-only test boundary; production remote reads resolve credentials per call.
type Reader struct {
	Home      string
	RemoteHub Hub
}
type View struct {
	SchemaVersion string `json:"schema_version"`
	ProjectID     string `json:"project_id,omitempty"`
	Revision      any    `json:"revision"`
	Basis         string `json:"basis"`
	ReadAt        string `json:"read_at"`
	Data          any    `json:"data"`
}
type ReadError struct {
	Code    string
	Message string
}

func (e *ReadError) Error() string            { return e.Message }
func NewReadError(code, message string) error { return &ReadError{code, message} }
func NewView(pid string, revision any, basis string, data any) View {
	return View{"gui-view/1", pid, revision, basis, time.Now().UTC().Format(time.RFC3339Nano), data}
}
func NewReader(home string) (*Reader, error) {
	root, e := fsutil.RealPath(home)
	if e != nil {
		return nil, e
	}
	return &Reader{Home: root}, nil
}
func (r *Reader) ReadRegistry() (db Object, err error) {
	defer guard(&err)
	path, e := store.Safe(r.Home, "registry.json")
	if e != nil {
		return nil, NewReadError("path-unavailable", "Device registry path is unavailable")
	}
	db, e = fsutil.ReadJSON(path)
	if os.IsNotExist(e) {
		return Object{"format": "rumsdlc-device/1", "identity": nil, "projects": Object{}, "repositories": Object{}, "accounts": Object{}, "meta_auth": authentication(""), "discovery": nil}, nil
	}
	if e != nil {
		return nil, NewReadError("identity-invalid", "Device registry cannot be read")
	}
	if db["format"] != "rumsdlc-device/1" {
		return nil, NewReadError("unsupported", "Unsupported device registry")
	}
	for _, k := range []string{"projects", "repositories", "accounts"} {
		if !has(db, k) {
			db[k] = Object{}
		}
		if _, ok := db[k].(Object); !ok {
			return nil, NewReadError("identity-invalid", "Invalid device registry collection")
		}
	}
	if !has(db, "meta_auth") {
		login := ""
		if identity, ok := db["identity"].(Object); ok {
			login = str(identity["login"])
		}
		db["meta_auth"] = authentication(login)
	}
	return db, nil
}
func (r *Reader) ReadDevice() (View, error) {
	db, e := r.ReadRegistry()
	if e != nil {
		return View{}, e
	}
	return NewView("", nil, "device", Object{"version": Version, "core_version": core.Version, "account": db["identity"], "service": "foreground", "readonly": true}), nil
}
func (r *Reader) snapshot(pid, revision string) (s *store.Store, state Object, err error) {
	db, e := r.ReadRegistry()
	if e != nil {
		return nil, nil, e
	}
	record, ok := getObject(db, "projects")[pid].(Object)
	if !ok {
		return nil, nil, NewReadError("missing", "Project is not registered")
	}
	s, e = store.New(str(record["path"]))
	if e != nil {
		return nil, nil, NewReadError("path-unavailable", "Project path is unavailable")
	}
	state, e = s.Read(false)
	if e != nil {
		return nil, nil, NewReadError("path-unavailable", "Project snapshot is unavailable or invalid")
	}
	if getObject(state, "project")["id"] != pid {
		return nil, nil, NewReadError("identity-invalid", "Registered and actual Project identities differ")
	}
	if revision != "" && fmt.Sprint(state["revision"]) != revision {
		return nil, nil, NewReadError("stale", "Project changed; refresh this view")
	}
	return s, state, nil
}
func (r *Reader) ReadProjects() (view View, err error) {
	defer guard(&err)
	db, e := r.ReadRegistry()
	if e != nil {
		return View{}, e
	}
	rows := []any{}
	for _, pid := range keys(getObject(db, "projects")) {
		record, ok := getObject(db, "projects")[pid].(Object)
		if !ok {
			continue
		}
		row := Object{"id": pid, "project_id": pid, "name": record["name"], "path": record["path"], "local_available": false, "management_status": record["status"], "sync_enabled": record["sync_enabled"], "last_sync": record["last_sync"], "conflict": record["conflict"], "code_status": "not-inspected", "last_checked_at": time.Now().UTC().Format(time.RFC3339Nano)}
		s, se := store.New(str(record["path"]))
		if se == nil {
			var state Object
			state, se = s.Read(false)
			if se == nil {
				if getObject(state, "project")["id"] == pid {
					row["local_available"] = true
					row["name"] = getObject(state, "project")["name"]
				} else {
					se = NewReadError("identity-invalid", "Registered and actual Project identities differ")
				}
			}
		}
		if se != nil {
			code := "path-unavailable"
			if re, ok := se.(*ReadError); ok {
				code = re.Code
			}
			row["error"] = Object{"code": code, "message": "Project is unavailable; inspect its registered path and identity"}
		}
		rows = append(rows, row)
	}
	return NewView("", nil, "registry", Object{"rows": rows, "total": len(rows), "offset": 0, "limit": len(rows)}), nil
}

// collect queries an immutable validated snapshot, preserving core ordering.
func collect(state Object, p Object) ([]any, Object, error) {
	rows := []any{}
	var result Object
	for offset := 0; ; offset += 200 {
		q := copyObject(p)
		q["offset"] = offset
		q["limit"] = 200
		v, e := core.Query(state, q)
		if e != nil {
			return nil, nil, e
		}
		result = v
		batch, _ := v["rows"].([]any)
		rows = append(rows, batch...)
		if len(batch) < 200 {
			break
		}
	}
	return rows, result, nil
}
func entityRows(state Object, table string) ([]any, error) {
	rows, _, e := collect(state, Object{"view": "entities", "table": table, "include_closed": true})
	return rows, e
}
