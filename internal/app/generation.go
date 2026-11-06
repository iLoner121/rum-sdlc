package app

import (
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"rumsdlc/internal/assets"
	"rumsdlc/internal/fsutil"
	"rumsdlc/internal/jsonvalue"
	"runtime"
	"sort"
	"strings"
)

func jsonBytes(value any) []byte {
	raw, e := json.MarshalIndent(value, "", "  ")
	must(e)
	return append(raw, '\n')
}
func resourceBytes(path string) any {
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	must(e)
	return hex.EncodeToString(raw)
}
func writeResource(path string, value any) {
	if value == nil {
		e := os.Remove(path)
		if !os.IsNotExist(e) {
			must(e)
		}
		return
	}
	raw, e := hex.DecodeString(str(value))
	must(e)
	must(fsutil.WriteBytes(path, raw))
}
func desiredResources(root, meta, id, home, name string) map[string][]byte {
	loadConfig(meta, true)
	result := map[string][]byte{}
	prefix := ".agents/skills/rum-project/"
	must(fs.WalkDir(assets.Files, "resources/project-skill", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			raw, e := assets.Files.ReadFile(path)
			if e != nil {
				return e
			}
			result[prefix+strings.TrimPrefix(path, "resources/project-skill/")] = raw
		}
		return nil
	}))
	must(fs.WalkDir(assets.Files, "native-core", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			raw, e := assets.Files.ReadFile(path)
			if e != nil {
				return e
			}
			result[prefix+"native-core/"+strings.TrimPrefix(path, "native-core/")] = raw
		}
		return nil
	}))
	executable, e := os.Executable()
	must(e)
	// Installed projects follow the stable native launcher across offline upgrades.
	// Portable deployments keep the explicit executable path instead.
	active, err := fsutil.ReadJSON(filepath.Join(home, "active.json"))
	if err == nil && active["runtime_kind"] == "go-native" {
		name := "launch"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		stable := filepath.Join(home, name)
		if fsutil.IsFile(stable) {
			executable = stable
		}
	}
	context := Object{"format": "rum-agent-context/1", "project_id": id, "workspace": root, "meta": filepath.Join(root, "meta"), "code_binding": "Read rum.py context for actual device paths", "device_home": home, "core_scripts": filepath.Join(root, filepath.FromSlash(prefix), "native-core"), "runtime": executable, "runtime_kind": "go-native", "version": Version}
	result[".rum-context.json"] = jsonBytes(context)
	entry := "# RumSDLC Project\n\n工程：" + name + "；Project：" + id + "\n\n" +
		"- 从本工程根目录启动会话。code/ 是默认代码目录；实际关联以 context 为准，meta/ 是独立管理数据目录。\n" +
		"- 已安装全局入口为 rum；本目录 rum.py 是自动补全 Project 的便捷入口。\n" +
		"- 接管时运行 python3 rum.py context 与 python3 rum.py doctor，读取返回的实际入口。\n" +
		"- 项目操作采用 [.agents/skills/rum-project/SKILL.md](.agents/skills/rum-project/SKILL.md)。\n" +
		"- 先确定本会话处理的 Topic；不依赖项目共享的当前 Topic，不自动领取下一项。\n" +
		"- 系统写入先预检再提交；不得直接修改 meta/state.json。Git 同步不代表业务验收。\n" +
		"- 代码关联以 context 为准；本体不检查本地代码；远端事实仅按显式请求查询，不自动改变流程状态。\n" +
		"- 代码获取、现场核验及 Git 操作由 agent 按项目 skill 执行；检查不完整必须按 git-fallback.md 补证，不能直接推送；代码目录的开发规则继续适用。\n" +
		"- 新建 Topic 时按 skill 的 context 模板准备 TASK_CONTEXT；其他文档按需建立。\n"
	result["AGENTS.md"] = []byte(entry)
	result["rum.py"] = []byte(`#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
root=Path(__file__).resolve().parent
c=json.loads((root/'.rum-context.json').read_text())
args=sys.argv[1:]
def supplied(name):
 return any(arg.startswith('--') and arg != '--' and name.startswith(arg.split('=',1)[0]) for arg in args[1:])
if args and args[0] in {'platform','enable','disable','sync','resolve','bind-code','conflict-retry','agent-install','project-update','project-identity'} and not supplied('--project'):
 args += ['--project',c['project_id']]
if args and args[0] in {'context','doctor','observe-code'} and not supplied('--path') and not supplied('--project'):
 args += ['--path',str(root)]
if not Path(c['runtime']).is_file():
 sys.exit('RumSDLC runtime missing; reinstall RumSDLC and run agent-install for this Project')
os.execv(c['runtime'],[c['runtime'],'--home',c['device_home'],*args])
`)
	result[prefix+"scripts/workflow.py"] = []byte(`import os,sys
from pathlib import Path
root=Path(__file__).resolve().parents[4]
os.execv(sys.executable,[sys.executable,str(root/'rum.py'),'tool','workflow','--',*sys.argv[1:]])
`)
	return result
}
func recoverAgent(root string) {
	journal := agentPath(root, ".rum-agent-update.json")
	if !fsutil.Exists(journal) {
		return
	}
	updates, e := fsutil.ReadJSON(journal)
	must(e)
	for name, v := range updates {
		managedName(name, true)
		item := object(v, "project recovery item")
		exact(item, "before after", "Invalid project recovery item")
		for _, v := range item {
			require(v == nil || regexp.MustCompile(`^(?:[0-9a-f]{2})*$`).MatchString(str(v)), "Invalid project recovery content")
		}
	}
	for name, v := range updates {
		item := object(v, "recovery item")
		content := resourceBytes(agentPath(root, name))
		require(equal(content, item["before"]) || equal(content, item["after"]), "Interrupted generation has external edits; retained recovery journal: "+journal)
	}
	for _, name := range keys(updates) {
		writeResource(agentPath(root, name), object(updates[name], "recovery item")["before"])
	}
	must(os.Remove(journal))
}
func (o *operation) prepareAgent(record Object, physical string, updateConfig bool) Object {
	return contract(o.ctx, "workspace.prepare_agent_update", []any{record, physical, updateConfig}, func() Object { return o.prepareAgentNative(record, physical, updateConfig) })
}
func (o *operation) prepareAgentNative(record Object, physical string, updateConfig bool) Object {
	root := str(record["workspace"])
	actual := physical
	if actual == "" {
		actual = root
	}
	configPath := agentPath(actual, "meta/project-config.json")
	raw, e := os.ReadFile(configPath)
	must(e)
	value, e := jsonvalue.Decode(raw)
	must(e)
	cfg := validateConfig(object(value, "configuration"))
	expected := contract(o.ctx, "workspace.generated_files", []any{root, filepath.Join(actual, "meta"), record["id"], o.device.Home, record["workspace_name"]}, func() map[string][]byte {
		return desiredResources(root, filepath.Join(actual, "meta"), str(record["id"]), o.device.Home, str(record["workspace_name"]))
	})
	old := Object{}
	inventoryPath := agentPath(actual, ".rum-agent-files.json")
	inventoryBefore := resourceBytes(inventoryPath)
	if inventoryBefore != nil {
		data, e := hex.DecodeString(str(inventoryBefore))
		must(e)
		value, e := jsonvalue.Decode(data)
		must(e)
		old = object(value, "project inventory")
		for name, digest := range old {
			managedName(name, false)
			require(regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(str(digest)), "Invalid project inventory digest: "+name)
		}
	}
	before := Object{"meta/project-config.json": hex.EncodeToString(raw), ".rum-agent-files.json": inventoryBefore}
	names := Object{}
	for name := range old {
		names[name] = true
	}
	for name := range expected {
		names[name] = true
	}
	for _, name := range keys(names) {
		data := resourceBytes(agentPath(actual, name))
		before[name] = data
		if data != nil {
			bytes, e := hex.DecodeString(str(data))
			must(e)
			desired, exists := expected[name]
			require((has(old, name) && jsonvalue.HashBytes(bytes) == old[name]) || (exists && string(bytes) == string(desired)), "Generated file was customized; back up local edits and restore the managed file before regenerating: "+name)
		}
	}
	output := Object{}
	inventory := Object{}
	for name, data := range expected {
		output[name] = hex.EncodeToString(data)
		inventory[name] = jsonvalue.HashBytes(data)
	}
	for name := range old {
		if _, exists := expected[name]; !exists {
			output[name] = nil
		}
	}
	output[".rum-agent-files.json"] = hex.EncodeToString(jsonBytes(inventory))
	contract(o.ctx, "workspace.agent_output", []any{record, physical}, func() any { return nil })
	if updateConfig {
		data := contract(o.ctx, "workspace.update_configuration", []any{record, clone(cfg)}, func() []byte {
			if !equal(record["identity_version"], 4) && record["remote"] != nil {
				remote := object(record["remote"], "remote")
				b := object(object(cfg["repositories"], "repositories")["meta"], "meta")
				b["url"] = "https://github.com/" + str(remote["full_name"]) + ".git"
				b["id"] = remote["id"]
				b["branch"] = remote["branch"]
			}
			cfg["rules_version"] = Version
			return jsonBytes(validateConfig(cfg))
		})
		output["meta/project-config.json"] = hex.EncodeToString(data)
	}
	updates := Object{}
	for name, after := range output {
		if !equal(before[name], after) {
			updates[name] = Object{"before": before[name], "after": after}
		}
	}
	return Object{"before": before, "updates": updates, "files": len(inventory)}
}
func (o *operation) installAgent(record Object, physical string, updateConfig bool, prepared Object) Object {
	return contract(o.ctx, "workspace.install_agent", []any{record, physical, updateConfig, prepared}, func() Object { return o.installAgentNative(record, physical, updateConfig, prepared) })
}
func (o *operation) installAgentNative(record Object, physical string, updateConfig bool, prepared Object) Object {
	root := str(record["workspace"])
	actual := physical
	if actual == "" {
		actual = root
	}
	if prepared == nil {
		recoverAgent(actual)
		prepared = o.prepareAgent(record, physical, updateConfig)
	} else {
		require(!fsutil.Exists(filepath.Join(actual, ".rum-agent-update.json")), "Project update needs recovery")
	}
	for name, before := range object(prepared["before"], "before") {
		require(equal(resourceBytes(agentPath(actual, name)), before), "Project resources changed after preparation; preview again: "+name)
	}
	updates := object(prepared["updates"], "updates")
	if len(updates) > 0 {
		journal := agentPath(actual, ".rum-agent-update.json")
		must(fsutil.WriteJSON(journal, updates))
		for _, name := range keys(updates) {
			item := object(updates[name], "update")
			path := agentPath(actual, name)
			require(equal(resourceBytes(path), item["before"]), "Generated file changed during regeneration; recovery retained")
			contract(o.ctx, "workspace._write_resource", []any{path, item["after"]}, func() any { writeResource(path, item["after"]); return nil })
		}
		for name, v := range updates {
			require(equal(resourceBytes(agentPath(actual, name)), object(v, "update")["after"]), "Generated file changed before completion; recovery retained")
		}
		must(os.Remove(journal))
	}
	return Object{"workspace": root, "files": prepared["files"], "rules_version": Version}
}
func (o *operation) updatePlan(record, prepared Object) Object {
	require(!fsutil.Exists(filepath.Join(str(record["workspace"]), ".rum-agent-update.json")), "Interrupted project update; run agent-install to recover first")
	names := keys(object(prepared["updates"], "updates"))
	sort.Strings(names)
	files := []any{}
	for _, name := range names {
		files = append(files, name)
	}
	return Object{"project_id": record["id"], "revision": fingerprint(Object{"record": record, "prepared": prepared}), "files": files, "rules_version": Version, "dry_run": true}
}
func (o *operation) updateAgent(p Object, preview bool) Object {
	record := o.local(str(p["project_id"]))
	require(truth(record["workspace"]), "Attach a workspace before updating project resources")
	if record["remote"] != nil {
		checkMetaBinding(str(record["path"]), object(record["remote"], "remote"))
	}
	return withStoreLock(str(record["path"]), func() Object {
		var result Object
		if preview {
			prepared := o.prepareAgent(record, "", true)
			plan := o.updatePlan(record, prepared)
			if truth(p["dry_run"]) {
				return plan
			}
			require(equal(p["expect_revision"], plan["revision"]), "Stale/missing project update preview; rerun project-update --dry-run")
			result = o.installAgent(record, "", true, prepared)
			result["changed"] = truth(plan["files"])
			result["dry_run"] = false
		} else {
			result = o.installAgent(record, "", true, nil)
		}
		delete(record, "github_account")
		record["identity_version"] = 4
		return result
	})
}
