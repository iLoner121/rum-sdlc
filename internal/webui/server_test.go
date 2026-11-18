package webui

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"strings"
	"testing"
	"time"
)

func TestReadOnlyHTTPBoundaryAndEmbeddedResources(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	r, _ := app.NewReader(filepath.Join(root, "absent"))
	handler := Handler(r, "127.0.0.1:12345")
	cases := []struct {
		method, path, host, origin string
		want                       int
	}{{"GET", "/api/v1/projects", "127.0.0.1:12345", "", 200}, {"HEAD", "/api/v1/projects", "127.0.0.1:12345", "", 200}, {"GET", "/", "127.0.0.1:12345", "", 200}, {"GET", "/api/v1/projects", "evil.example", "", 403}, {"HEAD", "/api/v1/projects", "evil.example", "", 403}, {"GET", "/api/v1/projects", "127.0.0.1:12345", "https://evil.example", 403}, {"POST", "/api/v1/projects", "127.0.0.1:12345", "", 405}, {"GET", "/api/v1/command", "127.0.0.1:12345", "", 404}, {"GET", "/../reader.go", "127.0.0.1:12345", "", 404}, {"GET", "/api/v1/projects/P-x/topics?limit=0", "127.0.0.1:12345", "", 400}}
	for _, c := range cases {
		t.Run(c.method+c.path+c.host+c.origin, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "http://"+c.host+c.path, nil)
			req.Host = c.host
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != c.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if c.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD has response body")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache allowed")
			}
			if c.want == 200 && strings.HasPrefix(c.path, "/api/") && c.method != "HEAD" {
				var view app.View
				if json.Unmarshal(w.Body.Bytes(), &view) != nil || view.SchemaVersion != "gui-view/1" {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}
func TestHTTPServerLifecycleAndInstallationLease(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	r, _ := app.NewReader(filepath.Join(root, "home"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, r, Options{Ready: func(address string) { ready <- address }}) }()
	var address string
	select {
	case address = <-ready:
	case e := <-done:
		t.Fatal(e)
	case <-time.After(3 * time.Second):
		t.Fatal("start timed out")
	}
	if l, e := fsutil.Lease(r.Home, true); e == nil {
		l.Close()
		t.Fatal("installer acquired active UI lease")
	}
	resp, e := http.Get(address + "api/v1/projects")
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("shutdown timed out")
	}
	l, e := fsutil.Lease(r.Home, true)
	if e != nil {
		t.Fatal("lease leaked", e)
	}
	l.Close()
	u := strings.TrimPrefix(strings.TrimSuffix(address, "/"), "http://")
	connection, e := net.DialTimeout("tcp4", u, 100*time.Millisecond)
	if e == nil {
		connection.Close()
		t.Fatal("listener still active")
	}
}
func TestHTTPPortOccupiedAndPendingInstallation(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	r, _ := app.NewReader(filepath.Join(root, "home"))
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	if e = Serve(context.Background(), r, Options{Port: listener.Addr().(*net.TCPAddr).Port}); e == nil {
		t.Fatal("occupied port accepted")
	}
	gate, e := fsutil.Lease(r.Home, true)
	if e != nil {
		t.Fatal(e)
	}
	defer gate.Close()
	if e = Serve(context.Background(), r, Options{}); e == nil {
		t.Fatal("UI started during install")
	}
}
