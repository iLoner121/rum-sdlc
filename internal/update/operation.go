package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/release"
	"time"
)

type Operation struct {
	Format         string           `json:"format"`
	ID             string           `json:"id"`
	Nonce          string           `json:"nonce,omitempty"`
	Home           string           `json:"home"`
	ExpectedActive string           `json:"expected_active"`
	Binary         string           `json:"binary"`
	BinaryHash     string           `json:"binary_sha256"`
	Target         release.Identity `json:"target"`
	Destination    string           `json:"destination"`
	BinDir         string           `json:"bin_dir"`
	Parent         int              `json:"parent_pid"`
	Launcher       int              `json:"launcher_pid,omitempty"`
	Phase          string           `json:"phase"`
	Error          string           `json:"error,omitempty"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

var operationID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (m *Manager) OperationPath(id string) (string, error) {
	if !operationID.MatchString(id) {
		return "", fmt.Errorf("Invalid update operation ID")
	}
	return m.path(filepath.Join("operations", id, "request.json"))
}
func (m *Manager) SaveOperation(o Operation) error {
	p, e := m.OperationPath(o.ID)
	if e != nil {
		return e
	}
	o.UpdatedAt = m.Now()
	return fsutil.WriteJSON(p, o)
}
func (m *Manager) Operation(id string) (Operation, error) {
	var o Operation
	if id == "" {
		p, e := m.path("current-operation.json")
		if e != nil {
			return o, e
		}
		var v struct {
			ID string `json:"id"`
		}
		if e = read(p, &v); os.IsNotExist(e) {
			return o, nil
		} else if e != nil {
			return o, e
		}
		id = v.ID
	}
	p, e := m.OperationPath(id)
	if e != nil {
		return o, e
	}
	if e = read(p, &o); e != nil {
		return o, e
	}
	if o.Format != "rum-update-operation/1" || o.ID != id || o.Home != m.Home {
		return o, fmt.Errorf("Invalid update request")
	}
	return o, nil
}
func (m *Manager) SetCurrent(id string) error {
	p, e := m.path("current-operation.json")
	if e != nil {
		return e
	}
	return fsutil.WriteJSON(p, map[string]string{"id": id})
}
func (o Operation) Terminal() bool {
	return o.Phase == "installed" || o.Phase == "failed" || o.Phase == "verification-failed" || o.Phase == "recovery-required"
}
func Status(home, id string) (map[string]any, error) {
	m := New(home)
	c, e := m.Config()
	if e != nil {
		return nil, e
	}
	s, e := m.State()
	if e != nil {
		return nil, e
	}
	o, e := m.Operation(id)
	if e != nil {
		return nil, e
	}
	o.Nonce = ""
	var active any
	raw, err := os.ReadFile(filepath.Join(home, "active.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &active)
	}
	running := ServiceRunning(home)
	return map[string]any{"config": c, "state": s, "operation": o, "active": active, "scheduler_running": c.Enabled && running, "service_required": c.Enabled && !running}, nil
}

func ServiceRunning(home string) bool {
	p, e := fsutil.RealPath(filepath.Join(home, "daemon.lock"))
	if e != nil {
		return false
	}
	l, e := fsutil.Acquire(p, true, false)
	if e != nil {
		return true
	}
	l.Close()
	return false
}
