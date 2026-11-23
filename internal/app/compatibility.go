package app

import (
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/assets"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"rumsdlc/internal/store"
	"sort"
	"strings"
)

func managedName(name string, recovery bool) {
	require(name != "" && !filepath.IsAbs(name) && filepath.ToSlash(filepath.Clean(name)) == name, "Invalid managed resource path")
	for _, part := range strings.Split(name, "/") {
		require(part != "..", "Invalid managed resource path")
	}
	allowed := oneOf(name, "AGENTS.md rum.py .rum-context.json") || strings.HasPrefix(name, ".agents/skills/rum-project/") && len(strings.Split(name, "/")) > 3
	if recovery {
		allowed = allowed || oneOf(name, ".rum-agent-files.json meta/project-config.json")
	}
	require(allowed, "Resource is outside managed project resources: "+name)
}
func agentPath(root, name string) string {
	path, e := store.Safe(root, filepath.FromSlash(name))
	must(e)
	return path
}
func projectContract() Object {
	raw, e := assets.Files.ReadFile("resources/project-contract.json")
	must(e)
	value, e := jsonvalue.Decode(raw)
	must(e)
	return object(value, "Project contract")
}
func (o *operation) checkAgent(record Object) (result Object, err error) {
	defer guard(&err)
	root := str(record["workspace"])
	require(!fsutil.Exists(filepath.Join(root, ".rum-agent-update.json")), "Interrupted project update; rerun agent-install")
	contextPath := agentPath(root, ".rum-context.json")
	require(fsutil.IsFile(contextPath), "Missing project context; run explicit project-update")
	ctx, e := fsutil.ReadJSON(contextPath)
	must(e)
	contract := projectContract()
	require(ctx["format"] == "rum-agent-context/1" && equal(ctx["project_id"], record["id"]), "Project context identity/format is incompatible")
	native := ctx["runtime_kind"] == "go-native"
	text(ctx["version"], "Missing project resource version")
	if !native {
		supported := false
		for _, version := range array(contract["versions"], "versions") {
			supported = supported || equal(version, ctx["version"])
		}
		require(supported, "Legacy project resources require an explicit project-update before code work")
	}
	for _, key := range []string{"workspace", "meta", "device_home", "core_scripts", "runtime"} {
		require(filepath.IsAbs(str(ctx[key])), "Invalid project context path: "+key)
	}
	for key, expected := range map[string]string{"workspace": root, "meta": str(record["path"]), "device_home": o.device.Home} {
		actual, e := filepath.EvalSymlinks(str(ctx[key]))
		must(e)
		wanted, e := filepath.EvalSymlinks(expected)
		must(e)
		require(actual == wanted, "Project context path differs from registered "+key+"; run explicit project-update")
	}
	coreFiles := object(contract["core"], "core")
	if native {
		coreFiles = Object{}
		entries, e := assets.Files.ReadDir("native-core")
		must(e)
		for _, entry := range entries {
			if !entry.IsDir() {
				raw, e := assets.Files.ReadFile("native-core/" + entry.Name())
				must(e)
				coreFiles[entry.Name()] = jsonvalue.HashBytes(raw)
			}
		}
	}
	for name, expected := range coreFiles {
		path := filepath.Join(str(ctx["core_scripts"]), name)
		info, e := os.Lstat(path)
		require(e == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0, "Project core resource is missing/unsafe: "+path)
		raw, e := os.ReadFile(path)
		must(e)
		digest := jsonvalue.HashBytes(raw)
		// The 0.9.0 native model uses long Topic IDs. It can still operate on
		// the same Store, while refreshed tools support short references too.
		legacyNativeModel := native && name == "model.py" && digest == "e1f68ba6581898bb18247f63a27345d5aee7c6da193a5721d5ab5bc4640a7ca2"
		require(digest == expected || legacyNativeModel, "Unrecognized project core resource; run project-update: "+path)
	}
	require(fsutil.IsFile(str(ctx["runtime"])), "Project launcher runtime is missing; run explicit project-update")
	inventoryPath := agentPath(root, ".rum-agent-files.json")
	require(fsutil.IsFile(inventoryPath), "Missing project resource inventory; run explicit project-update")
	inventory, e := fsutil.ReadJSON(inventoryPath)
	must(e)
	for name, digest := range inventory {
		managedName(name, false)
		require(regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(str(digest)), "Invalid project inventory digest: "+name)
	}
	for _, v := range array(contract["required"], "required") {
		require(has(inventory, str(v)), "Project inventory is incomplete; run explicit project-update")
	}
	customized := []string{}
	// Release-owned Python resources were syntax-checked when packaged. Their
	// exact hashes permit native health/sync to work without a Python interpreter.
	// Only customized project scripts need their own Python environment to check syntax.
	knownPython := map[string]bool{}
	for _, data := range desiredResources(root, str(record["id"]), o.device.Home, str(record["workspace_name"])) {
		knownPython[jsonvalue.HashBytes(data)] = true
	}
	for _, name := range keys(inventory) {
		if strings.HasSuffix(name, ".py") && strings.HasPrefix(name, ".agents/skills/rum-project/scripts/") {
			if data, err := assets.Files.ReadFile("resources/project-skill/scripts/" + strings.TrimPrefix(name, ".agents/skills/rum-project/scripts/")); err == nil {
				knownPython[jsonvalue.HashBytes(data)] = true
			}
		}
	}
	for _, name := range keys(inventory) {
		path := agentPath(root, name)
		require(fsutil.IsFile(path), "Project resource is missing: "+name)
		raw, e := os.ReadFile(path)
		must(e)
		if strings.HasSuffix(name, ".py") && !knownPython[jsonvalue.HashBytes(raw)] {
			// Python is a dependency of these project-level resources, never the native
			// application. Compile the resource without executing it or writing pycache.
			_, e := o.device.Runner.Run(o.ctx, []string{"python3", "-B", "-c", "import pathlib,sys; p=pathlib.Path(sys.argv[1]); compile(p.read_bytes(),str(p),'exec')", path}, "", "", nil, 0, true)
			must(e)
		}
		if jsonvalue.HashBytes(raw) != inventory[name] {
			customized = append(customized, name)
		}
	}
	sort.Strings(customized)
	customValues := []any{}
	for _, name := range customized {
		customValues = append(customValues, name)
	}
	gate := ".agents/skills/rum-project/scripts/agent_bootstrap.py"
	raw, e := os.ReadFile(agentPath(root, gate))
	must(e)
	expected, e := assets.Files.ReadFile("resources/project-skill/scripts/agent_bootstrap.py")
	must(e)
	ready := string(raw) == string(expected)
	warnings := []any{}
	if ctx["version"] != Version {
		warnings = append(warnings, "Project resources were generated by "+str(ctx["version"])+"; use project-update to refresh them if needed.")
	}
	if !ready {
		warnings = append(warnings, "Older or customized project bootstrap: update/review it before relying on the current runtime gate contract.")
	}
	return Object{"compatible": true, "rules_version": ctx["version"], "runtime_version": Version, "update_available": ctx["version"] != Version, "customized_files": customValues, "runtime_gate_ready": ready, "warnings": warnings, "note": "Compatibility and presence only; customized guidance requires actual agent review."}, nil
}
