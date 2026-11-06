package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"rumsdlc/internal/catalog"
	"rumsdlc/internal/core"
	"rumsdlc/internal/migration"
)

func RunMigration(args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(out, core.Version)
		return nil
	}
	spec := catalog.Parse("source! store! project-id! name! date! active-milestone +dry-run expect-revision")
	p, e := parseOptions(args, spec)
	if errors.Is(e, flag.ErrHelp) {
		fmt.Fprintln(out, catalog.Usage("tool migrate --", spec))
		return nil
	}
	if e != nil {
		return e
	}
	str := func(key string) string { s, _ := p[key].(string); return s }
	result, e := migration.Run(migration.Options{Source: str("source"), Destination: str("store"), ProjectID: str("project_id"), Name: str("name"), Date: str("date"), ActiveMilestone: str("active_milestone"), DryRun: p["dry_run"] == true, Expected: str("expect_revision")})
	if e != nil {
		return e
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
