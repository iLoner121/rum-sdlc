// Package catalog describes the public command interface for parsing and introspection.
package catalog

import "strings"

type Option struct {
	Name, Destination string
	Kind              byte
	Required          bool
	DefaultValue      any
}
type Spec struct{ Options []Option }

func Parse(text string) Spec {
	s := Spec{}
	for _, field := range strings.Fields(text) {
		required := strings.HasSuffix(field, "!")
		field = strings.TrimSuffix(field, "!")
		kind := byte('s')
		if strings.HasPrefix(field, "+") {
			kind = 'b'
			field = field[1:]
		}
		destination := strings.ReplaceAll(field, "-", "_")
		if field == "project" {
			destination = "project_id"
		}
		s.Options = append(s.Options, Option{field, destination, kind, required, nil})
	}
	return s
}

var Specs = map[string]Spec{
	"identity": Parse(""), "setup": Parse("confirm-login +global-credentials"), "project-identity": Parse("project! name email +inherit"), "list": Parse(""), "discover": Parse(""), "explain": Parse("action"),
	"context": Parse("path project"), "doctor": Parse("path project"), "observe-code": Parse("path project +remote topic"), "project-update": Parse("project! +dry-run expect-revision"), "agent-install": Parse("project!"),
	"workspace-import": Parse("repository! parent! name"), "workspace-adopt": Parse("path! date! name"), "workspace-init": Parse("parent! date! name"), "workspace-recover": Parse("operation-id!"),
	"create": Parse("path! name! date! project-id"), "register": Parse("path!"), "scan": Parse("root!"), "bind-code": Parse("project! path repository kind topic branch +clear-branch +clear-repository review-url target-branch +clear-review"),
	"repository-plan": Parse("project! name"), "enable": Parse("project! repository repository-name +accept-empty"), "disable": Parse("project!"), "sync": Parse("project!"), "resolve": Parse("project! conflict-id! +accept"), "conflict-retry": Parse("project! conflict-id!"), "import": Parse("repository! path!"), "platform": Parse("project! operation! args"),
}

func Usage(name string, spec Spec) string {
	var b strings.Builder
	b.WriteString("rum " + name)
	for _, o := range spec.Options {
		token := "--" + o.Name
		if o.Kind != 'b' {
			token += " VALUE"
		}
		if !o.Required {
			token = "[" + token + "]"
		}
		b.WriteString(" " + token)
	}
	return b.String()
}

const Methods = "identity setup list register create scan bind-code observe-code discover repository-plan enable disable import sync resolve conflict-retry platform tick workspace-adopt workspace-import workspace-recover workspace-init agent-install project-update context doctor explain project-identity"

func Help() map[string]any {
	help := map[string]any{}
	for name, spec := range Specs {
		required := []any{}
		for _, option := range spec.Options {
			if option.Required {
				required = append(required, option.Destination)
			}
		}
		help[name] = map[string]any{"usage": Usage(name, spec), "required": required, "help": Usage(name, spec)}
	}
	help["daemon"] = map[string]any{"usage": "rum daemon {run,start,stop,status} [--interval SECONDS] [--discovery-interval SECONDS]", "required": []any{"action"}, "help": "Control the optional local synchronization service."}
	help["tool"] = map[string]any{"usage": "rum tool {workflow,migrate,git_adapter,repo_check,commit_check} -- [ARGUMENTS]", "required": []any{"tool"}, "help": "Run native workflow commands or project-level Python tools."}
	return help
}
