package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"rumsdlc/internal/release"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func target() Target {
	return Target{ID: 1, Tag: "v1.2.3", Assets: []Asset{{ID: 2, Name: "checksums.json", URL: "unused"}, {Name: PackageName("1.2.3", "darwin", "arm64")}, {Name: PackageName("1.2.3", "linux", "amd64")}, {Name: PackageName("1.2.3", "windows", "amd64")}}}
}
func TestChannelsNeverImplicitlyDowngrade(t *testing.T) {
	for _, c := range []struct{ version, channel, want string }{{"1.2.2", "stable", "update-available"}, {"1.2.3", "stable", "up-to-date"}, {"1.2.4", "stable", "local-newer"}, {"1.2.3", "dev", "channel-switch-required"}, {"1.2.3", "unknown", "channel-switch-required"}} {
		got, e := Classify(release.Identity{Version: c.version, Channel: c.channel}, target())
		if e != nil || got != c.want {
			t.Fatal(c, got, e)
		}
	}
}
func TestAnonymousConditionalCheckAndFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("public query required authentication")
		}
		n := calls.Add(1)
		if n == 2 {
			if r.Header.Get("If-None-Match") != "abc" {
				t.Error("missing ETag")
			}
			w.WriteHeader(304)
			return
		}
		if n == 3 {
			w.WriteHeader(429)
			return
		}
		w.Header().Set("ETag", "abc")
		json.NewEncoder(w).Encode(target())
	}))
	defer server.Close()
	m := New(realTemp(t))
	m.Client = &Client{Base: server.URL, HTTP: server.Client()}
	m.Current = release.Identity{Version: "1.2.2", Channel: "stable"}
	s, e := m.Check(context.Background())
	if e != nil || s.Status != "update-available" {
		t.Fatal(s, e)
	}
	s, e = m.Check(context.Background())
	if e != nil || s.Target == nil {
		t.Fatal(s, e)
	}
	_, e = m.Check(context.Background())
	if e == nil {
		t.Fatal("rate limit accepted")
	}
	saved, _ := m.State()
	if saved.LastError == "" || saved.Target == nil {
		t.Fatal(saved)
	}
}
func TestNoticeAndAcknowledgementPersist(t *testing.T) {
	m := New(realTemp(t))
	m.Current = release.Identity{Version: "1.2.2", Channel: "stable"}
	r := target()
	s := State{Format: "rum-update-state/1", Target: &r, Status: "update-available"}
	if e := m.save(s); e != nil {
		t.Fatal(e)
	}
	if m.Notice() == "" || m.Notice() != "" {
		t.Fatal("notice not deduplicated")
	}
	m2 := New(m.Home)
	m2.Current = m.Current
	if m2.Notice() != "" {
		t.Fatal("restart repeated notice")
	}
	if _, e := m2.Ack("1.2.3"); e != nil {
		t.Fatal(e)
	}
}
func TestConfigDefaultAndUnknownPreserved(t *testing.T) {
	m := New(realTemp(t))
	c, e := m.Config()
	if e != nil || c.Enabled || c.Interval != 3600 {
		t.Fatal(c, e)
	}
	p, _ := m.path("config.json")
	raw := []byte(`{"format":"future"}`)
	os.WriteFile(p, raw, 0600)
	if _, e = m.Configure(true); e == nil {
		t.Fatal("unknown config overwritten")
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(raw, got) {
		t.Fatal(string(got))
	}
}
func TestArchivePathsAndLinksRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
	}{{"../rum", tar.TypeReg}, {"rum", tar.TypeSymlink}, {"x/rum", tar.TypeReg}} {
		var b bytes.Buffer
		g := gzip.NewWriter(&b)
		w := tar.NewWriter(g)
		w.WriteHeader(&tar.Header{Name: tc.name, Mode: 0755, Typeflag: tc.kind, Size: 1})
		w.Write([]byte("x"))
		w.Close()
		g.Close()
		if _, e := extract(b.Bytes(), "rum", false); e == nil {
			t.Fatal(tc)
		}
	}
}
func TestStateSymlinkRejected(t *testing.T) {
	m := New(realTemp(t))
	p, _ := m.path("state.json")
	other := filepath.Join(t.TempDir(), "other")
	os.WriteFile(other, []byte("untouched"), 0600)
	if e := os.Symlink(other, p); e != nil {
		t.Skip(e)
	}
	if _, e := m.State(); e == nil {
		t.Fatal("symlink read")
	}
}
func TestDisabledSchedulerDoesNotQuery(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer s.Close()
	m := New(realTemp(t))
	m.Client = &Client{Base: s.URL, HTTP: s.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	m.Schedule(ctx)
	if calls.Load() != 0 {
		t.Fatal(calls.Load())
	}
}

func realTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestVerifiedDownloadAndChecksumFailure(t *testing.T) {
	var archive bytes.Buffer
	g := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(g)
	payload := []byte("native-executable")
	tarWriter.WriteHeader(&tar.Header{Name: "rum", Size: int64(len(payload)), Mode: 0755})
	tarWriter.Write(payload)
	tarWriter.Close()
	g.Close()
	sum := sha256.Sum256(archive.Bytes())
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sums" {
			digest := hex.EncodeToString(sum[:])
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			json.NewEncoder(w).Encode(Checksums{Version: "1.2.3", Files: map[string]string{PackageName("1.2.3", "linux", "amd64"): digest}})
			return
		}
		w.Write(archive.Bytes())
	}))
	defer server.Close()
	c := &Client{Base: server.URL, HTTP: server.Client()}
	target := Target{ID: 1, Tag: "v1.2.3", Assets: []Asset{{Name: "checksums.json", URL: server.URL + "/sums"}, {Name: PackageName("1.2.3", "linux", "amd64"), URL: server.URL + "/package", Size: int64(archive.Len())}}}
	path, _, e := c.Download(context.Background(), target, "linux", "amd64", realTemp(t))
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, payload) {
		t.Fatal(string(got), e)
	}
	valid = false
	dir := realTemp(t)
	if _, _, e = c.Download(context.Background(), target, "linux", "amd64", dir); e == nil {
		t.Fatal("bad checksum installed")
	}
	if _, e = os.Stat(filepath.Join(dir, "rum")); !os.IsNotExist(e) {
		t.Fatal("wrote executable before checksum")
	}
}
func TestHourlyScheduleAndDisable(t *testing.T) {
	var now atomic.Int64
	now.Store(time.Now().Unix())
	events := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(target())
		events <- struct{}{}
	}))
	defer server.Close()
	m := New(realTemp(t))
	m.Now = func() time.Time { return time.Unix(now.Load(), 0) }
	m.Client = &Client{Base: server.URL, HTTP: server.Client()}
	m.Current = release.Identity{Version: "1.2.2", Channel: "stable"}
	if _, e := m.Configure(true); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); m.Schedule(ctx) }()
	await := func() {
		t.Helper()
		select {
		case <-events:
		case <-time.After(4 * time.Second):
			t.Fatal("scheduled check missing")
		}
	}
	await()
	waitSaved(t, m)
	now.Add(3599)
	select {
	case <-events:
		t.Fatal("checked before one hour")
	case <-time.After(1100 * time.Millisecond):
	}
	now.Add(1)
	await()
	waitSaved(t, m)
	if _, e := m.Configure(false); e != nil {
		t.Fatal(e)
	}
	now.Add(7200)
	select {
	case <-events:
		t.Fatal("check after disable")
	case <-time.After(1100 * time.Millisecond):
	}
	cancel()
	<-done
}
func TestReleaseWithdrawalAndCrossHostToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x := target()
		x.Draft = true
		json.NewEncoder(w).Encode(x)
	}))
	defer server.Close()
	c := &Client{Base: server.URL, HTTP: server.Client()}
	if e := c.Confirm(context.Background(), target()); e == nil {
		t.Fatal("withdrawn release accepted")
	}
	req, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/file", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if e := redirect(req, nil); e != nil || req.Header.Get("Authorization") != "" {
		t.Fatal("credential forwarded", e)
	}
	req, _ = http.NewRequest("GET", "https://example.org/file", nil)
	if redirect(req, nil) == nil {
		t.Fatal("untrusted redirect allowed")
	}
}

func waitSaved(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s, e := m.State()
		if e == nil && s.CheckedAt.Equal(m.Now()) && s.NextDue.Equal(m.Now().Add(time.Hour)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("check did not persist its scheduled state", e)
		}
		time.Sleep(time.Millisecond)
	}
}
