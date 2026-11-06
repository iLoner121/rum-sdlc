package main

import (
	"encoding/json"
	"os"
	"rumsdlc/internal/cli"
)

func main() {
	if err := cli.RunWorkflow(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": err.Error(), "next": "Inspect current state; correct input or preview again. No automatic Git action."})
		os.Exit(1)
	}
}
