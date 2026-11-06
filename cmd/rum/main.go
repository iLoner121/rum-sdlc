package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"rumsdlc/internal/cli"
	"rumsdlc/internal/installation"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	managed, e := installation.LaunchManaged(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if !managed && e == nil {
		e = cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	}
	if e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) {
			os.Exit(exit.ExitCode())
		}
		json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": e.Error()})
		os.Exit(1)
	}
}
