// app-contract is a test-only transport that delegates remote test fixtures back
// to Python while every application operation and Git data operation runs in Go.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"rumsdlc/internal/app"
	"rumsdlc/internal/cli"
	"rumsdlc/internal/installation"
	"rumsdlc/internal/jsonvalue"
)

type callbackHub struct {
	input  *bufio.Reader
	output *json.Encoder
	login  string
}

func (h *callbackHub) ForLogin(login string) app.Hub { copy := *h; copy.login = login; return &copy }
func (h *callbackHub) Login() string                 { return h.login }
func (h *callbackHub) call(method string, args ...any) (any, error) {
	if e := h.output.Encode(map[string]any{"callback": method, "login": h.login, "args": args}); e != nil {
		return nil, e
	}
	raw, e := h.input.ReadBytes('\n')
	if e != nil {
		return nil, e
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		return nil, e
	}
	r, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Invalid fixture response")
	}
	if r["error"] != nil {
		return nil, fmt.Errorf("%v", r["error"])
	}
	return r["result"], nil
}
func (h *callbackHub) object(method string, args ...any) (app.Object, error) {
	value, e := h.call(method, args...)
	if e != nil {
		return nil, e
	}
	v, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Invalid fixture object")
	}
	return v, nil
}
func (h *callbackHub) User(context.Context) (app.Object, error) { return h.object("user") }
func (h *callbackHub) Repositories(context.Context) ([]app.Object, error) {
	value, e := h.call("repositories")
	if e != nil {
		return nil, e
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("Invalid fixture list")
	}
	result := []app.Object{}
	for _, v := range list {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid fixture row")
		}
		result = append(result, m)
	}
	return result, nil
}
func (h *callbackHub) Repository(_ context.Context, name string) (app.Object, error) {
	return h.object("repository", name)
}
func (h *callbackHub) Create(_ context.Context, name string) (app.Object, error) {
	return h.object("create", name)
}
func (h *callbackHub) Manifest(_ context.Context, name, branch string) (app.Object, error) {
	return h.object("manifest", name, branch)
}
func (h *callbackHub) Empty(_ context.Context, name string) (bool, error) {
	v, e := h.call("empty", name)
	b, _ := v.(bool)
	return b, e
}
func (h *callbackHub) URL(repo app.Object) (string, error) {
	v, e := h.call("url", repo)
	s, _ := v.(string)
	return s, e
}
func (h *callbackHub) Token(context.Context) (string, error) {
	v, e := h.call("token")
	s, _ := v.(string)
	return s, e
}
func main() {
	if handled, e := fixtureCommand(os.Args[1:]); handled {
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 {
		if e := cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); e != nil {
			json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": e.Error()})
			os.Exit(1)
		}
		return
	}
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	raw, e := reader.ReadBytes('\n')
	if e != nil {
		panic(e)
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		panic(e)
	}
	request := value.(map[string]any)
	hub := &callbackHub{input: reader, output: encoder}
	if request["method"] == "installation" {
		p, _ := request["params"].(map[string]any)
		s := func(k string) string { v, _ := p[k].(string); return v }
		ctx := configureInstallHooks(context.Background(), hub, request)
		result, e := installation.Install(ctx, installation.Options{Home: request["home"].(string), Destination: s("destination"), BinDir: s("bin_dir"), RecoverOnly: p["recover_only"] == true, ConfirmLegacyIdle: p["confirm_legacy_idle"] == true})
		if e != nil {
			encoder.Encode(map[string]any{"error": e.Error()})
		} else {
			encoder.Encode(map[string]any{"result": result})
		}
		return
	}
	device, e := app.NewDevice(request["home"].(string), hub)
	var result any
	if e == nil {
		params, _ := request["params"].(map[string]any)
		ctx := configureHooks(context.Background(), device, hub, request)
		result, e = device.Call(ctx, request["method"].(string), params)
	}
	if e != nil {
		encoder.Encode(map[string]any{"error": e.Error()})
	} else {
		encoder.Encode(map[string]any{"result": result})
	}
}
