package main

import (
	"encoding/json"
	"os"
	"rumsdlc/internal/cli"
)

func main() {
	if e := cli.RunMigration(os.Args[1:], os.Stdout); e != nil {
		json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": e.Error(), "next": "Preserve source, reconcile records and preview into a new empty destination."})
		os.Exit(1)
	}
}
