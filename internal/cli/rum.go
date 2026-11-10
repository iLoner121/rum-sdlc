package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/installation"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/release"
	"rumsdlc/internal/service"
	"rumsdlc/internal/update"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseOptions(args []string, spec catalog.Spec) (app.Object, error) {
	p := app.Object{}
	for _, o := range spec.Options {
		if o.Kind == 'b' {
			p[o.Destination] = false
		} else {
			p[o.Destination] = o.DefaultValue
		}
	}
	for i := 0; i < len(args); i++ {
		argument := args[i]
		if argument == "--help" || argument == "-h" {
			return nil, flag.ErrHelp
		}
		if !strings.HasPrefix(argument, "--") {
			return nil, fmt.Errorf("Unexpected argument: %s", argument)
		}
		name, value, inline := strings.Cut(argument[2:], "=")
		matches := []catalog.Option{}
		for _, o := range spec.Options {
			if o.Name == name {
				matches = []catalog.Option{o}
				break
			}
			if strings.HasPrefix(o.Name, name) {
				matches = append(matches, o)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("Unknown or ambiguous option: --%s", name)
		}
		o := matches[0]
		if o.Kind == 'b' {
			if inline {
				return nil, fmt.Errorf("--%s does not accept a value", o.Name)
			}
			p[o.Destination] = true
			continue
		}
		if !inline {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--%s requires a value", o.Name)
			}
			value = args[i]
		}
		if o.Name == "root" {
			values, _ := p[o.Destination].([]any)
			p[o.Destination] = append(values, value)
		} else {
			p[o.Destination] = value
		}
	}
	for _, o := range spec.Options {
		if o.Required && p[o.Destination] == nil {
			return nil, fmt.Errorf("--%s is required", o.Name)
		}
	}
	return p, nil
}
func Run(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	home := os.Getenv("RUMSDLC_HOME")
	if home == "" {
		user, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		home = filepath.Join(user, ".rumsdlc")
	}
	for len(args) > 0 {
		name, value, inline := strings.Cut(args[0], "=")
		if name != "--home" && name != "--hom" && name != "--ho" {
			break
		}
		args = args[1:]
		if !inline {
			if len(args) == 0 {
				return fmt.Errorf("--home requires a value")
			}
			value = args[0]
			args = args[1:]
		}
		home = value
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(out, release.Current().Display())
		return nil
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		names := []string{}
		for name := range catalog.Specs {
			names = append(names, name)
		}
		names = append(names, "daemon", "tool", "install")
		sort.Strings(names)
		fmt.Fprintln(out, "RumSDLC native device application\nCommands: "+strings.Join(names, ", "))
		return nil
	}
	method := args[0]
	args = args[1:]
	if method == "credential" {
		return credential(ctx, args, in, out)
	}
	root, e := fsutil.RealPath(home)
	if e != nil {
		return e
	}
	if _, e = service.Endpoint(root); e != nil {
		return e
	}
	if method == "version" || method == "update" || method == "update-status" || method == "update-apply" || method == "update-helper-capabilities" {
		result, err := updateCommand(ctx, root, method, args, out)
		if errors.Is(err, flag.ErrHelp) {
			if spec, ok := catalog.Specs[method]; ok {
				fmt.Fprintln(out, catalog.Usage(method, spec))
				return nil
			}
		}
		if err != nil {
			return err
		}
		if method == "update-apply" {
			return nil
		}
		return json.NewEncoder(out).Encode(app.Object{"ok": true, "result": result})
	}
	if method == "install" {
		spec := catalog.Parse("destination bin-dir +recover-only +confirm-legacy-idle +prepare-only")
		params, err := parseOptions(args, spec)
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(out, catalog.Usage(method, spec))
			return nil
		}
		if err != nil {
			return err
		}
		user, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		destination := filepath.Join(user, ".local", "share", "rumsdlc", app.Version)
		binDir := filepath.Join(user, ".local", "bin")
		if v, ok := params["destination"].(string); ok {
			destination = v
		}
		if v, ok := params["bin_dir"].(string); ok {
			binDir = v
		}
		destination, err = fsutil.RealPath(destination)
		if err != nil {
			return err
		}
		binDir, err = fsutil.RealPath(binDir)
		if err != nil {
			return err
		}
		result, err := installation.Install(ctx, installation.Options{Destination: destination, Home: root, BinDir: binDir, RecoverOnly: params["recover_only"] == true, ConfirmLegacyIdle: params["confirm_legacy_idle"] == true})
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(result)
	}
	if method == "bridge" {
		return bridge(root, in, out)
	}
	if method == "tool" {
		if len(args) == 0 {
			return fmt.Errorf("Choose a tool")
		}
		tool := args[0]
		arguments := args[1:]
		if len(arguments) > 0 && arguments[0] == "--" {
			arguments = arguments[1:]
		}
		lease, e := fsutil.Lease(root, false)
		if e != nil {
			return e
		}
		defer lease.Close()
		device, e := app.NewDevice(root, nil)
		if e != nil {
			return e
		}
		storePath := ""
		for i, arg := range arguments {
			if strings.HasPrefix(arg, "--") {
				name, value, inline := strings.Cut(arg, "=")
				if name != "--" && strings.HasPrefix("--store", name) {
					if inline {
						storePath = value
					} else if i+1 < len(arguments) {
						storePath = arguments[i+1]
					}
				}
			}
		}
		if storePath != "" {
			recovery := tool == "workflow" && len(arguments) > 0 && (arguments[0] == "lock-status" || arguments[0] == "lock-recover")
			if e = device.CheckToolStore(ctx, storePath, tool, recovery); e != nil {
				return e
			}
		}
		if tool == "migrate" {
			return RunMigration(arguments, out)
		}
		if tool == "workflow" {
			return RunWorkflow(arguments, in, out, stderr)
		}
		if tool != "git_adapter" && tool != "commit_check" && tool != "repo_check" {
			return fmt.Errorf("Unknown project tool: %s", tool)
		}
		runtime, e := device.ToolRuntime()
		if e != nil {
			return e
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		command := exec.CommandContext(ctx, "python3", append([]string{"-B", filepath.Join(runtime, "resources", "project-skill", "scripts", tool+".py")}, arguments...)...)
		command.Env = append(os.Environ(), "RUMSDLC_HOME="+root, "RUMSDLC_AGENT_HOME="+filepath.Join(root, "agent-state", "git"), "RUMSDLC_NATIVE_RUNTIME="+exe)
		command.Stdin = in
		command.Stdout = out
		command.Stderr = stderr
		return command.Run()
	}
	device, e := app.NewDevice(root, nil)
	if e != nil {
		return e
	}
	var result any
	if method == "daemon" {
		result, e = daemon(ctx, device, args)
	} else {
		spec, ok := catalog.Specs[method]
		if !ok {
			return fmt.Errorf("Unknown command: %s", method)
		}
		params, err := parseOptions(args, spec)
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(out, catalog.Usage(method, spec))
			return nil
		}
		if err != nil {
			return err
		}
		for _, key := range []string{"path", "parent"} {
			if value, ok := params[key].(string); ok {
				path, e := fsutil.AbsoluteInput(value)
				if e != nil {
					return e
				}
				params[key] = path
			}
		}
		if method == "context" || method == "doctor" || method == "observe-code" {
			if params["path"] == nil && params["project_id"] == nil {
				cwd, e := os.Getwd()
				if e != nil {
					return e
				}
				params["path"] = cwd
			}
		}
		if method == "scan" {
			roots := []any{}
			for _, v := range params["root"].([]any) {
				path, e := fsutil.AbsoluteInput(v.(string))
				if e != nil {
					return e
				}
				roots = append(roots, path)
			}
			params = app.Object{"roots": roots}
		}
		if method == "platform" {
			raw := "{}"
			if params["args"] != nil {
				raw = params["args"].(string)
			}
			var data []byte
			if raw == "-" {
				data, e = io.ReadAll(in)
			} else if strings.HasPrefix(raw, "@") {
				data, e = os.ReadFile(raw[1:])
			} else {
				data = []byte(raw)
			}
			if e != nil {
				return e
			}
			v, e := jsonvalue.Decode(data)
			if e != nil {
				return e
			}
			params["args"] = v
		}
		result, e = request(ctx, root, method, params)
		if errors.Is(e, os.ErrNotExist) || errors.Is(e, connectionRefused) {
			result, e = device.Call(ctx, method, params)
		}
	}
	if e != nil {
		return e
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if e = encoder.Encode(app.Object{"ok": true, "result": result}); e != nil {
		return e
	}
	if terminal, ok := stderr.(*os.File); ok {
		if stat, err := terminal.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
			if msg := update.New(root).Notice(); msg != "" {
				fmt.Fprintln(stderr, msg)
			}
		}
	}
	return nil
}
func credential(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	login := ""
	operation := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--login" {
			i++
			if i == len(args) {
				return fmt.Errorf("--login requires a value")
			}
			login = args[i]
		} else if strings.HasPrefix(args[i], "--login=") {
			login = strings.TrimPrefix(args[i], "--login=")
		} else {
			operation = args[i]
		}
	}
	if operation != "get" && operation != "store" && operation != "erase" {
		return fmt.Errorf("Invalid credential operation")
	}
	fields := map[string]string{}
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		key, value, _ := strings.Cut(line, "=")
		fields[key] = value
	}
	if e := scanner.Err(); e != nil {
		return e
	}
	if operation != "get" || fields["protocol"] != "https" || fields["host"] != "github.com" {
		return nil
	}
	hub := app.NewGitHub()
	hub.Account = login
	token, e := hub.Token(ctx)
	if e != nil {
		return e
	}
	if login == "" {
		login = "x-access-token"
	}
	fmt.Fprintf(out, "username=%s\npassword=%s\n\n", login, token)
	return nil
}
func daemon(ctx context.Context, device *app.Device, args []string) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("Choose daemon run, start, stop or status")
	}
	action := args[0]
	spec := catalog.Parse("interval discovery-interval installation-recovery")
	params, e := parseOptions(args[1:], spec)
	if e != nil {
		return nil, e
	}
	interval, discovery := 60, 600
	if params["interval"] != nil {
		interval, e = strconv.Atoi(params["interval"].(string))
		if e != nil {
			return nil, e
		}
	}
	if params["discovery_interval"] != nil {
		discovery, e = strconv.Atoi(params["discovery_interval"].(string))
		if e != nil {
			return nil, e
		}
	}
	if action == "run" {
		if token, ok := params["installation_recovery"].(string); ok {
			e = service.ServeRecovery(ctx, device, time.Duration(interval)*time.Second, time.Duration(discovery)*time.Second, token)
			return app.Object{"stopped": true}, e
		}
		e = service.Serve(ctx, device, time.Duration(interval)*time.Second, time.Duration(discovery)*time.Second)
		return app.Object{"stopped": true}, e
	}
	if action == "stop" {
		return service.Request(ctx, device.Home, "shutdown", nil)
	}
	if action == "status" {
		result, e := service.Request(ctx, device.Home, "list", nil)
		if e != nil {
			return app.Object{"running": false}, nil
		}
		var cycle any
		if fsutil.Exists(filepath.Join(device.Home, "last-cycle.json")) {
			cycle, _ = fsutil.ReadJSON(filepath.Join(device.Home, "last-cycle.json"))
		}
		return app.Object{"running": true, "counts": result.(map[string]any)["counts"], "last_cycle": cycle}, nil
	}
	if action != "start" {
		return nil, fmt.Errorf("Unknown daemon action")
	}
	if interval < 5 || discovery < interval {
		return nil, fmt.Errorf("Invalid synchronization intervals")
	}
	if _, e = service.Request(ctx, device.Home, "list", nil); e == nil {
		return nil, fmt.Errorf("Device service is already running")
	}
	executable, e := os.Executable()
	if e != nil {
		return nil, e
	}
	log, e := os.OpenFile(filepath.Join(device.Home, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	defer log.Close()
	cmd := exec.Command(executable, "--home", device.Home, "daemon", "run", "--interval", strconv.Itoa(interval), "--discovery-interval", strconv.Itoa(discovery))
	cmd.Stdout = log
	cmd.Stderr = log
	detach(cmd)
	if e = cmd.Start(); e != nil {
		return nil, e
	}
	pid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for i := 0; i < 50; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return nil, fmt.Errorf("Service startup failed; inspect %s", filepath.Join(device.Home, "service.log"))
		case <-ticker.C:
			if _, e = service.Request(ctx, device.Home, "list", nil); e == nil {
				return app.Object{"running": true, "pid": pid, "home": device.Home}, nil
			}
		}
	}
	return nil, fmt.Errorf("Service is still starting; inspect daemon status")
}
