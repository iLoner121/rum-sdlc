//go:build contracttest

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"rumsdlc/internal/app"
	"rumsdlc/internal/cli"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/process"
	"rumsdlc/internal/service"
	"syscall"
	"time"
)

// Snapshot of the original FakeHub: immutable remote control-plane fixtures,
// with real Git object reads and the actual native application/service.
type fixtureHub struct{ repositories app.Object }

func (h fixtureHub) ForLogin(string) app.Hub               { return h }
func (h fixtureHub) Login() string                         { return "" }
func (h fixtureHub) Token(context.Context) (string, error) { return "fixture", nil }
func (h fixtureHub) User(context.Context) (app.Object, error) {
	return app.Object{"id": 7, "login": "alice", "name": "Alice"}, nil
}
func (h fixtureHub) Repositories(context.Context) ([]app.Object, error) {
	result := []app.Object{}
	for _, v := range h.repositories {
		result = append(result, v.(map[string]any))
	}
	return result, nil
}
func (h fixtureHub) Repository(_ context.Context, name string) (app.Object, error) {
	v, ok := h.repositories[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Repository not found")
	}
	return v, nil
}
func (h fixtureHub) Create(context.Context, string) (app.Object, error) {
	return nil, fmt.Errorf("Immutable fixture catalog")
}
func (h fixtureHub) Manifest(ctx context.Context, name, branch string) (app.Object, error) {
	repo, e := h.Repository(ctx, name)
	if e != nil {
		return nil, e
	}
	result, e := (process.Runner{}).Run(ctx, []string{"git", "--git-dir", repo["_path"].(string), "show", branch + ":" + app.Manifest}, "", "", nil, 10*time.Second, true)
	if e != nil {
		return nil, e
	}
	v, e := jsonvalue.Decode([]byte(result.Stdout))
	if e != nil {
		return nil, e
	}
	return v.(map[string]any), nil
}
func (h fixtureHub) Empty(ctx context.Context, name string) (bool, error) {
	repo, e := h.Repository(ctx, name)
	if e != nil {
		return false, e
	}
	result, e := (process.Runner{}).Run(ctx, []string{"git", "--git-dir", repo["_path"].(string), "for-each-ref", "refs/heads"}, "", "", nil, 10*time.Second, true)
	return result.Stdout == "", e
}
func (h fixtureHub) URL(repo app.Object) (string, error) { return repo["_path"].(string), nil }

func fixtureCommand(args []string) (bool, error) {
	if len(args) > 0 && args[0] == "--prepare" {
		ctx := cli.WithRequestHook(context.Background(), func(home, method string, params app.Object) (any, error) {
			return app.Object{"home": home, "method": method, "params": params}, nil
		})
		return true, cli.Run(ctx, args[1:], os.Stdin, os.Stdout, os.Stderr)
	}
	if len(args) == 1 && args[0] == "--sleep-fixture" {
		time.Sleep(30 * time.Second)
		return true, nil
	}
	if len(args) < 3 || args[0] != "--serve-fixture" {
		return false, nil
	}
	repos, e := fsutil.ReadJSON(args[2])
	if e != nil {
		return true, e
	}
	device, e := app.NewDevice(args[1], fixtureHub{repos})
	if e != nil {
		return true, e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(args) > 3 && args[3] == "slow" {
		ctx = app.WithContractHook(ctx, func(ctx context.Context, name string, _ []any, original func() (any, error)) (any, error) {
			if name != "sync.remote_head" {
				return original()
			}
			if e := json.NewEncoder(os.Stdout).Encode(app.Object{"entered": true}); e != nil {
				return nil, e
			}
			exe, e := os.Executable()
			if e != nil {
				return nil, e
			}
			_, e = (process.Runner{}).Run(ctx, []string{exe, "--sleep-fixture"}, "", "", nil, time.Minute, true)
			if e != nil {
				return nil, e
			}
			return original()
		})
	}
	return true, service.Serve(ctx, device, 5*time.Second, 5*time.Second)
}
