package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/fsutil"
	"runtime"
	"testing"
	"time"
)

type offlineHub struct{}

func (offlineHub) ForLogin(string) app.Hub                  { return offlineHub{} }
func (offlineHub) Login() string                            { return "" }
func (offlineHub) Token(context.Context) (string, error)    { return "", fmt.Errorf("offline test") }
func (offlineHub) User(context.Context) (app.Object, error) { return nil, fmt.Errorf("offline test") }
func (offlineHub) Repositories(context.Context) ([]app.Object, error) {
	return nil, fmt.Errorf("offline test")
}
func (offlineHub) Repository(context.Context, string) (app.Object, error) {
	return nil, fmt.Errorf("offline test")
}
func (offlineHub) Create(context.Context, string) (app.Object, error) {
	return nil, fmt.Errorf("offline test")
}
func (offlineHub) Manifest(context.Context, string, string) (app.Object, error) {
	return nil, fmt.Errorf("offline test")
}
func (offlineHub) Empty(context.Context, string) (bool, error) {
	return false, fmt.Errorf("offline test")
}
func (offlineHub) URL(app.Object) (string, error) { return "", fmt.Errorf("offline test") }
func TestNativeServiceMatchesOneShotAndCleansUp(t *testing.T) {
	tempRoot := ""
	if runtime.GOOS == "darwin" {
		tempRoot = "/private/tmp"
	}
	base, e := os.MkdirTemp(tempRoot, "rum-service-")
	if e != nil {
		base, e = os.MkdirTemp("", "rum-service-")
	}
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(base)
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	home := filepath.Join(base, "d")
	device, e := app.NewDevice(home, offlineHub{})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, device, 5*time.Second, 5*time.Second) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = Request(ctx, home, "list", nil); e == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			select {
			case e := <-done:
				t.Fatalf("service failed: %v", e)
			default:
				t.Fatalf("startup: %v", e)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e = fsutil.Lease(home, true); e == nil {
		t.Fatal("installer acquired lease while service runs")
	}
	secondCtx, secondCancel := context.WithTimeout(ctx, time.Second)
	defer secondCancel()
	if e = Serve(secondCtx, device, 5*time.Second, 5*time.Second); e == nil {
		t.Fatal("duplicate service started")
	}
	result, e := Request(ctx, home, "create", app.Object{"path": filepath.Join(base, "project"), "name": "Native", "date": "2026-11-06", "project_id": "P-service"})
	if e != nil {
		t.Fatal(e)
	}
	if result.(map[string]any)["id"] != "P-service" {
		t.Fatal(result)
	}
	direct, e := device.Call(ctx, "platform", app.Object{"project_id": "P-service", "operation": "check"})
	if e != nil {
		t.Fatal(e)
	}
	remote, e := Request(ctx, home, "platform", app.Object{"project_id": "P-service", "operation": "check"})
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprint(direct) != fmt.Sprint(remote) {
		t.Fatalf("%v != %v", direct, remote)
	}
	endpoint, _ := Endpoint(home)
	idle, e := net.Dial("unix", endpoint)
	if e != nil {
		t.Fatal(e)
	}
	defer idle.Close()
	if _, e = Request(ctx, home, "shutdown", nil); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown waited for abandoned client")
	}
	if fsutil.Exists(endpoint) || fsutil.Exists(filepath.Join(home, "service.json")) {
		t.Fatal("service state retained")
	}
	lease, e := fsutil.Lease(home, true)
	if e != nil {
		t.Fatal(e)
	}
	lease.Close()
}

func TestRequestCancellationClosesIdleConnection(t *testing.T) {
	tempRoot := ""
	if runtime.GOOS == "darwin" {
		tempRoot = "/private/tmp"
	}
	base, e := os.MkdirTemp(tempRoot, "rum-cancel-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(base)
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	endpoint, e := Endpoint(base)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("unix", endpoint)
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, e := listener.Accept()
		if e == nil {
			accepted <- connection
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := Request(ctx, base, "list", nil); done <- err }()
	var connection net.Conn
	select {
	case connection = <-accepted:
		defer connection.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("request did not connect")
	}
	cancel()
	select {
	case e = <-done:
		if e == nil {
			t.Fatal("cancellation reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not cancel promptly")
	}
}
