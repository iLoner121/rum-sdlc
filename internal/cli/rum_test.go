package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"rumsdlc/internal/catalog"
	"runtime"
	"testing"
)

func TestGlobalCLIStandaloneAndNativeWorkflow(t *testing.T) {
	tempRoot := ""
	if runtime.GOOS == "darwin" {
		tempRoot = "/private/tmp"
	}
	base, e := os.MkdirTemp(tempRoot, "rum-cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(base)
	base, e = filepath.EvalSymlinks(base)
	if e != nil {
		t.Fatal(e)
	}
	home := filepath.Join(base, "d")
	var out, stderr bytes.Buffer
	args := []string{"--home", home, "create", "--path", filepath.Join(base, "p"), "--name", "Native", "--date", "2026-11-06", "--project-id", "P-cli"}
	if e = Run(context.Background(), args, bytes.NewReader(nil), &out, &stderr); e != nil {
		t.Fatal(e)
	}
	var response map[string]any
	if e = json.Unmarshal(out.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if response["ok"] != true {
		t.Fatal(response)
	}
	out.Reset()
	t.Setenv("PATH", "")
	if e = Run(context.Background(), []string{"--home", home, "platform", "--proj=P-cli", "--operation", "check"}, bytes.NewReader(nil), &out, &stderr); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e = Run(context.Background(), []string{"--home", home, "tool", "workflow", "--", "check", "--store", filepath.Join(base, "p")}, bytes.NewReader(nil), &out, &stderr); e != nil {
		t.Fatal(e)
	}
}
func TestOptionsPreserveLastExplicitValueAndRejectAmbiguity(t *testing.T) {
	p, e := parseOptions([]string{"--project=old", "--proj", "new", "--dry-run"}, catalog.Specs["project-update"])
	if e != nil {
		t.Fatal(e)
	}
	if p["project_id"] != "new" || p["dry_run"] != true {
		t.Fatal(p)
	}
	if _, e = parseOptions([]string{"--p", "value"}, catalog.Specs["create"]); e == nil {
		t.Fatal("ambiguous option accepted")
	}
}

func TestWorkflowOptionPrefixes(t *testing.T) {
	f := flag.NewFlagSet("command", flag.ContinueOnError)
	path := f.String("store", "", "")
	request := f.String("request", "", "")
	f.String("expect-revision", "", "")
	f.Bool("dry-run", false, "")
	args, e := workflowArguments(f, []string{"--sto=somewhere", "--req", "request.json", "--dry"})
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Parse(args); e != nil || *path != "somewhere" || *request != "request.json" {
		t.Fatalf("%v %v", args, e)
	}
	if _, e = workflowArguments(f, []string{"--dry-run=false"}); e == nil {
		t.Fatal("boolean option accepted an argument")
	}
}
