package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"rumsdlc/internal/app"
	"rumsdlc/internal/core"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
)

func bridge(home string, in io.Reader, out io.Writer) error {
	lease, e := fsutil.Lease(home, false)
	if e != nil {
		return e
	}
	defer lease.Close()
	raw, e := io.ReadAll(io.LimitReader(in, 64*1024*1024+1))
	if e != nil {
		return e
	}
	if len(raw) > 64*1024*1024 {
		return fmt.Errorf("Bridge request exceeds limit")
	}
	value, e := jsonvalue.Decode(raw)
	if e != nil {
		return e
	}
	p, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("Invalid bridge request")
	}
	str := func(key string) string { v, _ := p[key].(string); return v }
	obj := func(key string) map[string]any { v, _ := p[key].(map[string]any); return v }
	var result any
	switch str("operation") {
	case "validate":
		e = core.Validate(obj("state"))
		result = app.Object{"ok": e == nil}
	case "query":
		e = core.Validate(obj("state"))
		if e == nil {
			result, e = core.Query(obj("state"), obj("params"))
		}
	case "integration-context":
		e = core.ValidateIntegrationContext(p["value"], str("topic"), p["source"], p["target"])
		result = p["value"]
	case "integration-proof":
		e = core.ValidateIntegrationProof(p["value"], str("topic"), obj("pending"))
		result = p["value"]
	default:
		s, err := store.New(str("root"))
		if err != nil {
			return err
		}
		if str("root") == "" {
			return fmt.Errorf("Missing Store root")
		}
		contextPath := filepath.Join(filepath.Dir(s.Root), ".rum-context.json")
		if fsutil.Exists(contextPath) {
			context, e := fsutil.ReadJSON(contextPath)
			if e != nil {
				return e
			}
			owner, _ := context["device_home"].(string)
			meta, _ := context["meta"].(string)
			if owner != home || meta != s.Root {
				return fmt.Errorf("Store belongs to another project/device context; use its project entry")
			}
			if fsutil.Exists(filepath.Join(filepath.Dir(s.Root), ".rum-agent-update.json")) {
				return fmt.Errorf("Interrupted project update; recover with agent-install")
			}
			if i, e := os.Lstat(contextPath); e != nil || i.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("Unsafe project context")
			}
		}
		switch str("operation") {
		case "read":
			result, e = s.Read(p["optional"] == true)
		case "execute":
			if obj("request") == nil {
				return fmt.Errorf("Missing command request")
			}
			result, e = s.Execute(obj("request"), p["dry_run"] == true, str("expect"))
		case "lock-status":
			result, e = s.LockStatus()
		case "lock-recover":
			result, e = s.RecoverLock(str("expected"), p["confirmed_idle"] == true)
		case "document":
			o := store.DocumentOptions{Table: str("table"), ID: str("identifier"), Field: str("field"), ExpectedHash: str("expected_hash"), Ensure: p["ensure"] == true, Correction: str("correction")}
			if p["content"] != nil {
				content, ok := p["content"].(string)
				if !ok {
					return fmt.Errorf("Document content must be text")
				}
				o.Content = &content
			}
			result, e = s.Document(o)
		default:
			return fmt.Errorf("Unknown bridge operation")
		}
	}
	if e != nil {
		return e
	}
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(app.Object{"ok": true, "result": result})
}
