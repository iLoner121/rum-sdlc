// Package cli translates command-line input to application operations.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"rumsdlc/internal/core"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"strings"
)

func RunWorkflow(args []string, in io.Reader, out, stderr io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(out, core.Version)
		return nil
	}
	if len(args) == 0 {
		return errors.New("Choose command, query, check, document, lock-status or lock-recover")
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(stderr)
	root := f.String("store", "", "Store directory")
	var request, expect, table, id, field, content, hash, correction, owner, format *string
	var dry, ensure, idle *bool
	params := map[string]*string{}
	ints := map[string]*int{}
	var closed *bool
	switch command {
	case "command":
		request = f.String("request", "", "Request file; - for stdin")
		expect = f.String("expect-revision", "", "Fresh preview token")
		dry = f.Bool("dry-run", false, "Preview only")
	case "query":
		params["view"] = f.String("view", "overview", "View")
		params["table"] = f.String("table", "", "Entity table")
		params["identifier"] = f.String("id", "", "Entity ID")
		params["kind"] = f.String("kind", "", "Relation kind")
		params["date"] = f.String("date", "", "User-local date")
		params["status"] = f.String("status", "", "Status filter")
		params["milestone_id"] = f.String("milestone", "", "Milestone filter")
		params["line_id"] = f.String("line", "", "Line filter")
		params["workset_id"] = f.String("workset", "", "Workset filter")
		params["order_by"] = f.String("order-by", "id", "Order")
		params["scope"] = f.String("scope", "scope", "Iteration scope")
		ints["offset"] = f.Int("offset", 0, "Offset")
		ints["limit"] = f.Int("limit", 30, "Limit")
		ints["after_revision"] = f.Int("after-revision", 0, "History lower bound")
		closed = f.Bool("include-closed", false, "Include closed entities")
		format = f.String("format", "json", "json or markdown")
	case "document":
		table = f.String("table", "", "Table")
		id = f.String("id", "", "Entity ID")
		field = f.String("field", "description", "Document field")
		content = f.String("content-file", "", "UTF-8 file")
		hash = f.String("expect-hash", "", "Expected content hash")
		correction = f.String("correction", "", "Factual correction reason")
		ensure = f.Bool("ensure", false, "Create if missing")
	case "lock-recover":
		owner = f.String("expect-owner", "", "Expected owner token")
		idle = f.Bool("confirmed-idle", false, "All writers verified idle")
	case "check", "lock-status":
	case "--help", "-h":
		fmt.Fprintln(out, "workflow: command, query, check, document, lock-status, lock-recover; each requires --store")
		return nil
	default:
		return errors.New("Unknown command: " + command)
	}
	arguments, err := workflowArguments(f, args[1:])
	if err != nil {
		return err
	}
	if err := f.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("Unexpected positional arguments")
	}
	if *root == "" {
		return errors.New("--store is required")
	}
	s, err := store.New(*root)
	if err != nil {
		return err
	}
	var result core.Object
	switch command {
	case "command":
		if *request == "" {
			return errors.New("--request is required")
		}
		var raw []byte
		if *request == "-" {
			raw, err = io.ReadAll(in)
		} else {
			raw, err = os.ReadFile(*request)
		}
		if err != nil {
			return err
		}
		value, e := jsonvalue.Decode(raw)
		if e != nil {
			return e
		}
		r, ok := value.(map[string]any)
		if !ok {
			return errors.New("request must be a JSON object")
		}
		result, err = s.Execute(r, *dry, *expect)
	case "query":
		p := core.Object{}
		for k, v := range params {
			if *v != "" {
				p[k] = *v
			}
		}
		for k, v := range ints {
			p[k] = *v
		}
		p["include_closed"] = *closed
		state, e := s.Read(false)
		if e != nil {
			return e
		}
		result, err = core.Query(state, p)
		if err == nil && *format == "markdown" {
			fmt.Fprint(out, core.Markdown(result))
			return nil
		}
		if *format != "json" && *format != "markdown" {
			return errors.New("Unknown format")
		}
	case "check":
		state, e := s.Read(false)
		if e != nil {
			return e
		}
		counts := core.Object{}
		for _, t := range []string{"milestones", "worksets", "lines", "topics", "tasks"} {
			counts[t] = len(state[t].(map[string]any))
		}
		result = core.Object{"ok": true, "version": core.Version, "revision": state["revision"], "counts": counts, "limits": "Structure/history only; agent checks evidence, approval, design and actual code facts"}
	case "document":
		if *table == "" || *id == "" {
			return errors.New("--table and --id are required")
		}
		if *ensure && *content != "" {
			return errors.New("Choose ensure or content-file")
		}
		o := store.DocumentOptions{Table: *table, ID: *id, Field: *field, ExpectedHash: *hash, Ensure: *ensure, Correction: *correction}
		if *content != "" {
			raw, e := os.ReadFile(*content)
			if e != nil {
				return e
			}
			text := string(raw)
			o.Content = &text
		}
		result, err = s.Document(o)
	case "lock-status":
		result, err = s.LockStatus()
	case "lock-recover":
		if *owner == "" {
			return errors.New("--expect-owner is required")
		}
		result, err = s.RecoverLock(*owner, *idle)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// argparse accepted unique long-option prefixes. Preserve that interface while
// keeping the standard-library parser for values and command-specific help.
func workflowArguments(flags *flag.FlagSet, args []string) ([]string, error) {
	result := append([]string(nil), args...)
	for i := 0; i < len(result); i++ {
		arg := result[i]
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			continue
		}
		name, value, inline := strings.Cut(arg[2:], "=")
		if strings.HasPrefix("help", name) && flags.Lookup(name) == nil {
			matches := 0
			flags.VisitAll(func(f *flag.Flag) {
				if strings.HasPrefix(f.Name, name) {
					matches++
				}
			})
			if matches == 0 {
				result[i] = "--help"
				continue
			}
		}
		selected := flags.Lookup(name)
		if selected == nil {
			matches := []*flag.Flag{}
			flags.VisitAll(func(f *flag.Flag) {
				if strings.HasPrefix(f.Name, name) {
					matches = append(matches, f)
				}
			})
			if len(matches) != 1 {
				return nil, fmt.Errorf("Unknown or ambiguous option: --%s", name)
			}
			selected = matches[0]
		}
		result[i] = "--" + selected.Name
		boolean, ok := selected.Value.(interface{ IsBoolFlag() bool })
		isBoolean := ok && boolean.IsBoolFlag()
		if inline {
			if isBoolean {
				return nil, fmt.Errorf("--%s does not accept a value", selected.Name)
			}
			result[i] += "=" + value
		} else if !isBoolean {
			i++
		}
	}
	return result, nil
}
