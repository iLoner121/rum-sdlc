package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/release"
	"runtime"
	"time"
)

type Config struct {
	Format     string `json:"format"`
	Enabled    bool   `json:"enabled"`
	Interval   int    `json:"interval_seconds"`
	Channel    string `json:"channel"`
	Generation int64  `json:"generation"`
}
type State struct {
	RetryAfter   time.Time        `json:"retry_after,omitempty"`
	Format       string           `json:"format"`
	Current      release.Identity `json:"current"`
	Target       *Target          `json:"target,omitempty"`
	Status       string           `json:"status"`
	CheckedAt    time.Time        `json:"checked_at"`
	AttemptedAt  time.Time        `json:"attempted_at"`
	NextDue      time.Time        `json:"next_due"`
	ETag         string           `json:"etag,omitempty"`
	LastError    string           `json:"last_error,omitempty"`
	Notified     string           `json:"notified,omitempty"`
	Acknowledged string           `json:"acknowledged,omitempty"`
}
type Manager struct {
	Home    string
	Client  *Client
	Current release.Identity
	Now     func() time.Time
}

func New(home string) *Manager {
	return &Manager{Home: home, Client: NewClient(), Current: release.Current(), Now: time.Now}
}
func (m *Manager) path(name string) (string, error) {
	p, e := fsutil.RealPath(filepath.Join(m.Home, "updates", name))
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return "", e
	}
	return p, nil
}
func (m *Manager) lock(name string) (*fsutil.Lock, error) {
	p, e := m.path(name)
	if e != nil {
		return nil, e
	}
	return fsutil.Acquire(p, true, false)
}
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (m *Manager) Config() (Config, error) {
	c := Config{Format: "rum-update-config/1", Interval: 3600, Channel: "stable"}
	p, e := m.path("config.json")
	if e != nil {
		return c, e
	}
	e = read(p, &c)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if c.Format != "rum-update-config/1" || c.Interval != 3600 || c.Channel != "stable" {
		return c, fail("invalid-config", "Unknown update configuration; preserve this file")
	}
	return c, nil
}
func (m *Manager) State() (State, error) {
	s := State{Format: "rum-update-state/1", Current: m.Current, Status: "not-checked"}
	p, e := m.path("state.json")
	if e != nil {
		return s, e
	}
	e = read(p, &s)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	if s.Format != "rum-update-state/1" {
		return s, fail("invalid-state", "Unknown update state")
	}
	s.Current = m.Current
	if s.Target != nil && s.LastError == "" {
		if status, err := Classify(m.Current, *s.Target); err == nil {
			s.Status = status
		}
	}
	return s, nil
}
func (m *Manager) save(s State) error {
	p, e := m.path("state.json")
	if e != nil {
		return e
	}
	return fsutil.WriteJSON(p, s)
}
func (m *Manager) Configure(enabled bool) (Config, error) {
	l, e := m.lock("state.lock")
	if e != nil {
		return Config{}, e
	}
	defer l.Close()
	c, e := m.Config()
	if e != nil {
		return c, e
	}
	c.Enabled = enabled
	c.Generation++
	p, e := m.path("config.json")
	if e != nil {
		return c, e
	}
	if e = fsutil.WriteJSON(p, c); e != nil {
		return c, e
	}
	s, e := m.State()
	if e != nil {
		return c, e
	}
	s.NextDue = time.Time{}
	return c, m.save(s)
}
func (m *Manager) Check(ctx context.Context) (State, error) {
	l, e := m.lock("check.lock")
	if e != nil {
		s, _ := m.State()
		return s, fail("check-in-progress", "Another update check is running")
	}
	defer l.Close()
	c, e := m.Config()
	if e != nil {
		return State{}, e
	}
	s, e := m.State()
	if e != nil {
		return s, e
	}
	now := m.Now()
	if now.Before(s.RetryAfter) {
		return s, fail("retry-later", "Previous request failed; retry after "+s.RetryAfter.Format(time.RFC3339))
	}
	target, etag, requestErr := m.Client.Latest(ctx, s.ETag)
	lock, e := m.lock("state.lock")
	if e != nil {
		return s, e
	}
	defer lock.Close()
	currentConfig, e := m.Config()
	if e != nil {
		return s, e
	}
	if currentConfig.Generation != c.Generation {
		return s, fail("configuration-changed", "Update configuration changed during request")
	}
	latest, e := m.State()
	if e != nil {
		return s, e
	}
	s.Notified = latest.Notified
	s.Acknowledged = latest.Acknowledged
	s.AttemptedAt = now
	s.NextDue = m.Now().Add(time.Hour)
	s.Current = m.Current
	if requestErr != nil {
		s.LastError = requestErr.Error()
		var f *Failure
		if errors.As(requestErr, &f) && !f.RetryAt.IsZero() {
			s.RetryAfter = f.RetryAt
			if f.RetryAt.After(s.NextDue) {
				s.NextDue = f.RetryAt
			}
		}
		_ = m.save(s)
		return s, requestErr
	}
	if target != nil {
		s.Target = target
	}
	if s.Target == nil {
		return s, fail("invalid-cache", "Server returned 304 without cached release")
	}
	if !Supported(runtime.GOOS, runtime.GOARCH) {
		s.Status = "unsupported"
	} else if _, e = s.Target.Asset(PackageName(s.Target.Version(), runtime.GOOS, runtime.GOARCH)); e != nil {
		s.LastError = e.Error()
		_ = m.save(s)
		return s, e
	} else {
		s.Status, e = Classify(m.Current, *s.Target)
		if e != nil {
			return s, e
		}
	}
	s.ETag = etag
	s.CheckedAt = m.Now()
	s.LastError = ""
	s.RetryAfter = time.Time{}
	if e = m.save(s); e != nil {
		return s, e
	}
	return s, nil
}
func (m *Manager) Ack(version string) (State, error) {
	if _, e := release.Parse(version); e != nil {
		return State{}, e
	}
	l, e := m.lock("state.lock")
	if e != nil {
		return State{}, e
	}
	defer l.Close()
	s, e := m.State()
	if e != nil {
		return s, e
	}
	if s.Target == nil || s.Target.Version() != version {
		return s, fmt.Errorf("Version is not the pending update")
	}
	s.Acknowledged = version
	return s, m.save(s)
}
func (m *Manager) Notice() string {
	l, e := m.lock("state.lock")
	if e != nil {
		return ""
	}
	defer l.Close()
	s, e := m.State()
	if e != nil || s.Target == nil || s.LastError != "" || s.Status != "update-available" {
		return ""
	}
	v := s.Target.Version()
	if s.Notified == v || s.Acknowledged == v {
		return ""
	}
	s.Notified = v
	if m.save(s) != nil {
		return ""
	}
	return "RumSDLC " + v + " is available; run rum update"
}
func (m *Manager) Schedule(ctx context.Context) {
	var cancel context.CancelFunc
	var done chan struct{}
	defer func() {
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
	}()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		c, e := m.Config()
		if (e != nil || !c.Enabled) && cancel != nil {
			cancel()
		}
		if e == nil && c.Enabled && done == nil {
			s, err := m.State()
			if err == nil && !m.Now().Before(s.NextDue) {
				work, stop := context.WithCancel(ctx)
				cancel = stop
				done = make(chan struct{})
				ch := done
				go func() { defer close(ch); _, _ = m.Check(work) }()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			done = nil
			cancel = nil
		case <-timer.C:
		}
	}
}
