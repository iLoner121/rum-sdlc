package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"rumsdlc/internal/app"
	"rumsdlc/internal/webui"
	"runtime"
)

func runUI(ctx context.Context, home string, args []string, out, stderr io.Writer) error {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	port := flags.Int("port", 0, "Loopback port (0 selects an available port)")
	noOpen := flags.Bool("no-open", false, "Print the URL without opening a browser")
	if e := flags.Parse(args); e != nil {
		if e == flag.ErrHelp {
			return nil
		}
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("Unexpected ui argument")
	}
	reader, e := app.NewReader(home)
	if e != nil {
		return e
	}
	return webui.Serve(ctx, reader, webui.Options{Port: *port, Ready: func(address string) {
		fmt.Fprintln(out, "RumSDLC read-only UI: "+address)
		fmt.Fprintln(out, "Press Ctrl-C to stop. Stop this UI before updating RumSDLC; closing a browser tab does not stop it.")
		if !*noOpen {
			var command *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				command = exec.CommandContext(ctx, "open", address)
			case "windows":
				command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", address)
			default:
				command = exec.CommandContext(ctx, "xdg-open", address)
			}
			go func() {
				if e := command.Run(); e != nil {
					fmt.Fprintln(stderr, "Could not open a browser. Open the printed URL manually.")
				}
			}()
		}
	}})
}
