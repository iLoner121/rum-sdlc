package app

import (
	"context"
	"os"
	"path/filepath"
	"rumsdlc/internal/fsutil"
	"strings"
	"testing"
)

func TestAgentsTemplateWorkspaceOnboarding(t *testing.T) {
	a, b, hub, root := setupDevices(t)
	init := func(name string) Object {
		return call(t, a, "workspace-init", Object{"parent": filepath.Join(root, "projects"), "name": name, "date": "2026-11-25"})
	}
	readAgents := func(record Object) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(str(record["workspace"]), "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if result := call(t, a, "agents-template", nil); result["configured"] != false {
		t.Fatal(result)
	}
	defaultProject := init("Default")
	defaultContent := readAgents(defaultProject)
	if !strings.Contains(defaultContent, "## 接管入口") {
		t.Fatal("missing default instructions")
	}

	source := filepath.Join(root, "team.md")
	must(os.WriteFile(source, []byte("# 团队规则\n\n{{project_name}} / {{project_id}} / {{literal}}"), 0600))
	call(t, a, "agents-template", Object{"file": source})
	must(os.Remove(source))
	var err error
	a, err = NewDevice(a.Home, hub)
	if err != nil {
		t.Fatal(err)
	}
	if result := call(t, a, "agents-template", nil); result["configured"] != true {
		t.Fatal(result)
	}
	one := init("First Workspace")
	adoptPath := filepath.Join(root, "adopt-me")
	must(os.Mkdir(adoptPath, 0700))
	must(os.WriteFile(filepath.Join(adoptPath, "AGENTS.md"), []byte("Original code guidance\n"), 0600))
	two := call(t, a, "workspace-adopt", Object{"path": adoptPath, "date": "2026-11-25"})
	original, err := os.ReadFile(filepath.Join(adoptPath, "code", "AGENTS.md"))
	if err != nil || string(original) != "Original code guidance\n" {
		t.Fatalf("adoption changed code guidance: %q, %v", original, err)
	}

	call(t, a, "enable", Object{"project_id": one["id"], "repository_name": "RumSDLC_Template"})
	if result := call(t, b, "agents-template", nil); result["configured"] != false {
		t.Fatal("template unexpectedly shared across devices", result)
	}
	must(os.WriteFile(source, []byte("Imported: {{project_name}} / {{project_id}}"), 0600))
	call(t, b, "agents-template", Object{"file": source})
	three := call(t, b, "workspace-import", Object{"repository": "alice/RumSDLC_Template", "parent": filepath.Join(root, "imports"), "name": "Imported Workspace"})
	for _, record := range []Object{one, two} {
		want := "# 团队规则\n\n" + str(record["workspace_name"]) + " / " + str(record["id"]) + " / {{literal}}\n"
		if content := readAgents(record); !strings.Contains(content, "## 接管入口") || !strings.HasSuffix(content, want) {
			t.Fatalf("unexpected AGENTS.md for %s", record["workspace_name"])
		}
	}
	if want := "Imported: imported-workspace / " + str(one["id"]) + "\n"; !strings.HasSuffix(readAgents(three), want) {
		t.Fatal("import did not use the receiving device template and actual project identity")
	}

	before := readAgents(one)
	must(os.WriteFile(source, []byte("Next configuration"), 0600))
	call(t, a, "agents-template", Object{"file": source})
	if got := readAgents(init("Replacement")); !strings.HasSuffix(got, "Next configuration\n") {
		t.Fatal("new workspace did not pick up the new template")
	}
	call(t, a, "agents-template", Object{"clear": true})
	if got := readAgents(init("Unconfigured")); strings.Contains(got, "Next configuration") || strings.Contains(got, "团队规则") {
		t.Fatal("clear did not restore default generation")
	}
	project := Object{"project_id": one["id"], "dry_run": true}
	health := call(t, a, "doctor", Object{"project_id": one["id"]})
	compat := object(health["project_compatibility"], "compatibility")
	if health["ok"] != true || !equal(compat["customized_files"], []any{"AGENTS.md"}) {
		t.Fatal(health)
	}
	_, err = a.Call(context.Background(), "project-update", project)
	if err == nil || !strings.Contains(err.Error(), "was customized") {
		t.Fatalf("expected existing customization protection, got %v", err)
	}
	if readAgents(one) != before || readAgents(defaultProject) != defaultContent {
		t.Fatal("existing workspace guidance changed")
	}
}

func TestAgentsTemplateErrorsPreserveConfigurationAndAdoption(t *testing.T) {
	a, _, _, root := setupDevices(t)
	source := filepath.Join(root, "rules.md")
	must(os.WriteFile(source, []byte("Keep these rules"), 0600))
	call(t, a, "agents-template", Object{"file": source})

	for _, tc := range []struct {
		name, content, message string
	}{
		{"blank", " \n\t", "empty"},
		{"binary", "a\x00b", "UTF-8"},
		{"invalid UTF-8", "\xff", "UTF-8"},
		{"large", strings.Repeat("x", agentsTemplateLimit+1), "exceeds 1 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			must(os.WriteFile(source, []byte(tc.content), 0600))
			_, err := a.Call(context.Background(), "agents-template", Object{"file": source})
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q, got %v", tc.message, err)
			}
		})
	}
	for _, params := range []Object{{"file": source, "clear": true}, {"file": root}, {"file": filepath.Join(root, "missing.md")}} {
		if _, err := a.Call(context.Background(), "agents-template", params); err == nil {
			t.Fatalf("expected error for %v", params)
		}
	}
	db, err := a.ReadDB()
	if err != nil || db[agentsTemplateKey] != "Keep these rules" {
		t.Fatalf("valid template was lost: %v", err)
	}

	db[agentsTemplateKey] = 42
	must(fsutil.WriteJSON(filepath.Join(a.Home, "registry.json"), db))
	adoptPath := filepath.Join(root, "existing")
	must(os.Mkdir(adoptPath, 0700))
	must(os.WriteFile(filepath.Join(adoptPath, "example.txt"), []byte("original"), 0600))
	_, err = a.Call(context.Background(), "workspace-adopt", Object{"path": adoptPath, "date": "2026-11-25"})
	if err == nil || !strings.Contains(err.Error(), "Invalid device AGENTS.md template") {
		t.Fatalf("expected actionable configuration error, got %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(adoptPath, "example.txt"))
	if err != nil || string(raw) != "original" || fsutil.Exists(filepath.Join(adoptPath, "meta")) {
		t.Fatalf("failed adoption did not restore original directory: %q, %v", raw, err)
	}
	call(t, a, "agents-template", Object{"clear": true})
	call(t, a, "workspace-adopt", Object{"path": adoptPath, "date": "2026-11-25"})
}
