package app

import (
	"io"
	"os"
	"rumsdlc/internal/fsutil"
	"strings"
	"unicode/utf8"
)

const agentsTemplateKey = "agents_template"
const agentsTemplateLimit = 1 << 20

func validateAgentsTemplate(content string) {
	require(len(content) <= agentsTemplateLimit, "AGENTS.md template exceeds 1 MiB")
	require(utf8.ValidString(content) && !strings.ContainsRune(content, 0), "AGENTS.md template must be UTF-8 text without NUL")
	require(strings.TrimSpace(content) != "", "AGENTS.md template is empty")
}

func (o *operation) agentsTemplate() string {
	value := o.db[agentsTemplateKey]
	if value == nil {
		return ""
	}
	content, ok := value.(string)
	require(ok, "Invalid device AGENTS.md template; replace it with --file or remove it with --clear")
	validateAgentsTemplate(content)
	return content
}

func (o *operation) configureAgentsTemplate(p Object) Object {
	clear := truth(p["clear"])
	require(p["file"] == nil || !clear, "Choose --file or --clear")
	if clear {
		o.db[agentsTemplateKey] = nil
	} else if p["file"] != nil {
		source, err := fsutil.RealPath(text(p["file"], "--file requires a template path"))
		must(err)
		info, err := os.Lstat(source)
		must(err)
		require(info.Mode().IsRegular(), "AGENTS.md template must be a regular file: "+source)
		file, err := os.Open(source)
		must(err)
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, agentsTemplateLimit+1))
		must(err)
		content := string(data)
		validateAgentsTemplate(content)
		o.db[agentsTemplateKey] = content
	}
	return Object{"configured": o.agentsTemplate() != ""}
}

func (o *operation) installWorkspaceAgent(record Object, stage string) {
	template := o.agentsTemplate()
	o.installAgent(record, stage, false, nil)
	if template == "" {
		return
	}
	path := agentPath(stage, "AGENTS.md")
	builtin, err := os.ReadFile(path)
	must(err)
	rendered := strings.NewReplacer(
		"{{project_name}}", str(record["workspace_name"]),
		"{{project_id}}", str(record["id"]),
	).Replace(template)
	if !strings.HasSuffix(rendered, "\n") {
		rendered += "\n"
	}
	// Keep the built-in inventory digest so refreshes recognize the appended
	// content as a local customization and never overwrite it.
	must(fsutil.WriteBytes(path, append(builtin, "\n"+rendered...)))
}
