// Package process runs bounded, cancellable subprocesses. Environment policies
// distinguish metadata Git from user-owned code Git.
package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type Result struct {
	Stdout, Stderr string
	Code           int
}
type Runner struct{}

func Environment(clean bool) map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, v, ok := strings.Cut(entry, "=")
		if ok && (!clean || !strings.HasPrefix(key, "GIT_")) {
			env[key] = v
		}
	}
	if clean {
		env["GIT_CONFIG_NOSYSTEM"] = "1"
		env["GIT_CONFIG_GLOBAL"] = os.DevNull
		env["GH_HOST"] = "github.com"
	}
	env["GIT_TERMINAL_PROMPT"] = "0"
	env["GCM_INTERACTIVE"] = "Never"
	env["GH_PROMPT_DISABLED"] = "1"
	return env
}
func CodeEnvironment() map[string]string {
	env := Environment(false)
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		delete(env, k)
	}
	return env
}
func (Runner) Run(ctx context.Context, args []string, cwd, input string, env map[string]string, timeout time.Duration, check bool) (Result, error) {
	if e := ctx.Err(); e != nil {
		return Result{}, fmt.Errorf("Operation cancelled because the service is stopping: %w", e)
	}
	if len(args) == 0 {
		return Result{}, fmt.Errorf("Missing command")
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(input)
	if env == nil {
		env = Environment(true)
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cmd.Env = append(cmd.Env, k+"="+env[k])
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	e := runProcess(ctx, cmd)
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.Code = cmd.ProcessState.ExitCode()
	} else {
		result.Code = -1
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("Operation cancelled or command timed out: %w", ctx.Err())
	}
	if e != nil {
		if _, ok := e.(*exec.ExitError); !ok {
			return result, fmt.Errorf("Unable to execute %s: %w", args[0], e)
		}
	}
	if check && result.Code != 0 {
		message := strings.TrimSpace(result.Stderr)
		runes := []rune(message)
		if len(runes) > 1200 {
			message = string(runes[len(runes)-1200:])
		}
		return result, fmt.Errorf("%s failed: %s", args[0], message)
	}
	return result, nil
}
