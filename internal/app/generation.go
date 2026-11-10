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
		"## 接管入口\n" +
		"\n" +
		"- 从工程根目录启动会话。已安装全局入口为 rum；本目录 rum.py 自动补全 Project。\n" +
		"- 先运行 python3 rum.py context 与 python3 rum.py doctor，读取实际 Store、代码路径、运行时和 skill 入口。doctor 只检查管理系统，不证明代码或业务验收通过。\n" +
		"- 项目操作采用 [.agents/skills/rum-project/SKILL.md](.agents/skills/rum-project/SKILL.md)，按活动读取其引用准则；代码目录自身的有效开发规则继续适用。\n" +
		"- 项目查询、规划和汇报不必选择执行 Topic；开始具体需求活动前明确本会话 Topic，读取其 TASK_CONTEXT.md 和当前 Task。不依赖全局当前 Topic，不自动领取下一项。\n" +
		"\n" +
		"## 目录与文档职责\n" +
		"\n" +
		"code/ 是默认代码目录，实际关联以 context 为准；meta/ 是独立管理数据目录。下表路径均相对于实际 Store，由 document 接口定位，不按显示名称或短编号猜测实体目录。\n" +
		"\n" +
		"| 对象 | 正文入口 | 用途 |\n" +
		"|---|---|---|\n" +
		"| Project | `documents/project/<Project-ID>/DESCRIPTION.md` | 项目目标、范围与已确认约束 |\n" +
		"| Milestone | `documents/milestones/<M-ID>/DESCRIPTION.md` | 阶段目标、交付条件及重要纪要 |\n" +
		"| Iteration / Pool | `documents/worksets/<ITER-ID 或 POOL>/DESCRIPTION.md` | 迭代目标、范围决定、复核和结束纪要；Pool 保存候选想法与优先理由 |\n" +
		"| Line | `documents/lines/<Line-ID>/DESCRIPTION.md` | 交付路线、适用范围与保留/退出说明 |\n" +
		"| Topic | `documents/topics/<Topic-ID>/TASK_CONTEXT.md` | 目标、验收条件、当前有效方案、文档入口与恢复点 |\n" +
		"| Task | `documents/topics/<Topic-ID>/tasks/tNNN.md` | 一件活动的输入、过程、产物、结论与交接 |\n" +
		"\n" +
		"- TASK_CONTEXT.md 和 Task 活动文档仅用于 Topic。Milestone、Iteration 各维护一份 DESCRIPTION.md，不套用执行上下文或另建任务、设计、验收文档。\n" +
		"- Topic 开题按 skill 的 context 模板准备上下文；PRD.md、DESIGN.md、acceptance.md 按需求、设计和验收准则逐步建立，不批量生成空文件。独立正文建立后，上下文只留摘要和链接。\n" +
		"- 产品当前架构、接口契约及长期技术决策随代码维护，位置与准则见 skill 的 references/design.md；本次拟议方案和执行过程保留在 Topic/Task。\n" +
		"- 正文记录目标、理由和证据；实体身份、状态、归属、成员、排序和历史以系统查询为准，不维护第二份可编辑台账。重要决定保留来源和日期，具体内容只维护一处并引用。\n" +
		"\n" +
		"## 数据保护与执行边界\n" +
		"\n" +
		"- 实体及关系写入先预检，再用返回令牌执行；不得直接修改 meta/state.json。正文通过 document 接口定位和检查，按需编辑并修复引用；文件存在不证明相应活动已完成。\n" +
		"- 通用文档接口允许选择字段；创建 Milestone、Iteration 或 Topic 不会自动完成所需正文，agent 应按上述职责准备文档。\n" +
		"- Task done、Git 同步、代码合并与业务验收分别核实；不得互相替代或据此自动推进生命周期。\n" +
		"\n" +
		"## 代码与 Git\n" +
		"\n" +
		"- 代码获取、现场核验及 Git 操作由 agent 按项目 skill 执行。本体不检查本地代码；远端事实仅按用户明确请求查询，不自动改变流程状态。\n" +
		"- Git 操作前运行 repo_check.py inspect；检查不完整时按 skill 的 references/git-fallback.md 补证，关键事实未知时停止对应动作。\n" +
		"- 提交前核对实际分支、差异、有效 author/committer、消息、验证和目标，按 Git 准则落实用户授权，不绕过 hook 或签名。保留未提交现场，不自动 stash/reset/force/prune 或删除成果。\n"
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
