# 系统接口与调用

系统能力由 RumSDLC runtime 提供；项目 skill 的 scripts/ 仅转发调用，不复制实现。Python 3.9+，系统层不需要 Git。

## 写操作

统一入口：

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py command --store <工程>/meta --request <请求JSON文件> --dry-run
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py command --store <工程>/meta --request <同一请求JSON文件> --expect-revision <返回令牌>
```

请求格式：

```json
{
  "action": "project.init",
  "params": {"id": "P-example", "name": "Example"},
  "date": "2026-10-26",
  "reason": "用户决定采用该项目工作流",
  "evidence": "现有项目说明或决定位置",
  "decision": "已有授权的引用"
}
```

日期由 agent 按用户时区提供。action/params/date/reason/evidence/decision 六个键固定；无适用证据/决定时后两项可为 null，但需要它们的操作会拒绝空值。未知参数拒绝，避免写错字段被静默忽略。接口只检查说明存在与结构，不认证引用内容。一般读取或内部活动不额外索取用户批准；已有授权用已有引用。

Python API：将 scripts 目录加入 Python 模块搜索路径后，service.request(action, params, date, reason, evidence=None, decision=None) 构造请求；Store(root).execute(request, dry_run=True) 预检，然后 execute(request, expect=token) 应用。命令成功不提交或推送文件。以下参数名均指 params 内字段。

| action | 参数及含义 |
|---|---|
| project.init | id（P- 开头小写段）、name；空目录初始化，生成业务 Pool/mainline，不需要代码仓库 |
| project.close | summary；先结束全部 Topic、ITER、M 和临时/维护 Line，需要用户决定与证据 |
| extension.set | table、id、namespace、value；整体替换一个外部命名空间的 JSON 对象；需 reason/evidence/decision，适用时允许关闭后事实纠错；不改变核心实体字段或关系 |
| entity.rename | table、id、name；closed 另需 correction，稳定 ID/路径/关系不变 |
| closure.correct | table、id、changes；仅纠正已关闭实体的既有事实字段，日期/原因/证据/已有决定写统一请求；不改身份、归属、outcome、关闭日期或重开生命周期 |
| milestone.create | name；建立 planned 阶段，分配稳定编号 |
| milestone.activate | id；前序 M 已结束且无其他 active；需要阶段选择决定 |
| milestone.reorder | ids：全部且仅 planned M 的精确顺序，不改已开始阶段 |
| milestone.close | id、outcome=completed/cancelled/superseded、summary，替代另需 successor；未完成成员先转出；用户决定/证据必需 |
| workset.create | name，可选 type=iteration；draft 可为空，Pool 不重复创建 |
| workset.propose | id、topics：提议 Topic ID 数组；不改变当前 Pool 归属，候选未形成 Topic 时先写 draft 正文 |
| workset.activate | id；非空提议、成员仍在 Pool，冻结范围并原子移动；需要范围决定/证据 |
| workset.change | id、action=add/remove/reorder、topics；active 范围调整，remove 另需 destination；需要影响证据与决定，复核日期不自动重置 |
| workset.review | id；真实复核的决定/证据，更新实际日期 |
| workset.close | id、outcome=completed/replanned/cancelled、summary、results，可选 goal_acceptance；整轮完成按经批准调整后的目标是否达成判断，未完成当前成员回 Pool |
| line.create | id（稳定 slug）、name、type=development/maintenance，可选 extensions（外部配置对象）；不创建或要求 Git 分支 |
| line.state | id、status=active/paused/integrating；暂停需 resume_when；检查业务成员准入，不要求 observed_tip。可选 observations 为不解释内容的外部观察对象，仅记录在请求历史。统一请求 evidence 可为 null，保留新版约定 |
| line.close | id、outcome、summary、verification；integrated 需 delivery_record；取消/替代需 retention 或 discard_decision 二选一；替代需 successor；retired 需 retention/last_release；可选 extensions 保存外部证据 |
| topic.create | name、type=feat/fix/spike/refactor/docs/chore、slug；可选 milestone_id/line_id；无 M 必须 unassigned_reason，默认 Line 为主线，默认当前集合为 Pool；默认系统分配 ID，外部已预留身份时可 id + id_reason，必须同日期、格式正确且未用过 |
| topic.integration.prepare | id、context；当前 Line 上更新已有整合评估，保持原来源与当前归属；暂停执行、失效旧验证及接受；不由系统解释代码锚点 |
| topic.dependencies | id、dependencies：唯一 Topic ID 数组，不接受缺失引用或环 |
| topic.state | id、status；review 需 verification，ready 需 acceptance；更换 Line 后 review 另需与选定 context 一致的 integration_proof，含 verified_on/report/verification；不能只填报告清门禁 |
| topic.close | id、outcome、summary、verification；交付需 delivery_record；取消/替代需 retention（保留记录或明确丢弃决定），替代需 successor；可选 extensions 保存外部证据 |
| relation.move | kind=milestone/workset/line、topic_id、to；M 可为 null，当前集合只能 Pool/active ITER，从 ITER 移出另需 destination，与 change remove 共用约束；Line 迁移另需 integration_context（见下文），保存并暂停执行现场；需要关系调整决定与证据 |
| task.create | topic_id、name、kind；建立 planned 活动，不自动执行 |
| task.state | id（T-ID/tNNN）、status；done 需 result（可记录失败），deferred 需 destination；异常转换的原因写请求 reason |
| task.correct | id、result；仅纠正 done 活动的实际结果，统一请求需 reason/evidence/decision；Topic 或 Project 已关闭也可留痕纠错，不重开或改变活动状态 |
| task.kind | id、kind；为 planned/blocked 活动或导入的未知旧活动确定实际类型，不改写执行中已知活动含义 |

ITER results 每项：topic、disposition=completed/cancelled/superseded/carry-forward/backlog、note、evidence；覆盖实际承诺范围。completed 对应已经 delivered/research-complete 关闭的 Topic；未完成项不能冒充 completed。整轮 outcome=completed 不要求每项 disposition 都 completed：取消/替代/移出如实记录，goal_acceptance 写调整后目标的实际接受；未单独提供时保留统一请求 decision 的引用，agent 必须核对它确实覆盖目标达成。不能靠改名伪造逐项结果。

## 查询与检查

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py check --store <目录>
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view overview --date YYYY-MM-DD
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view resolve --id <T-ID>
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view members --kind milestone --id M0
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view orphans
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view show --table topics --id <T-ID>
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view task-summary --id <T-ID>
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py query --store <目录> --view history --id <T-ID> --after-revision 0
```

entities 视图使用 --table project/milestones/worksets/lines/topics/tasks。members 的 kind 为 milestone/workset/line。默认非 closed，--include-closed 查历史；列表 --offset 0 --limit 30，最多 200。历史 ITER members 显示冻结结果。resolve 输出三种关联、推导目标和快照 revision。状态名称无额外本地副本，默认不加载正文。

Topic 列表/成员/孤儿视图支持 --status（逗号分隔）及 --milestone / --line / --workset 组合筛选，筛选先于分页；指定 closed 状态会包含关闭项。--format markdown 生成人读输出，默认 JSON。--view milestones 按 position 显示阶段顺序；entities --table milestones 默认仍按 ID，可 --order-by position。

--view iteration-progress --id ITER-001 --scope current/scope/history 分别查当前待办、当轮有效范围/进展、曾选入参与历史。活动 ITER 的 members --include-closed 也包含在本轮关闭的 Topic；已移出的后续关闭不冒充本轮完成。关闭 ITER 仅按冻结 results 展示，不用当前状态筛选历史结果。

活动 ITER 的 members（含 --include-closed）和 iteration-progress 优先按当前关系 position 显示待办；本轮已关闭成员随后按关闭时快照 revision/ID 排列，history 再追加已移出成员。当前成员 position 来自唯一关系表；已关闭/移出成员的 position 为本视图派生的后续排序值，不写回关系表。筛选、分页在排定顺序后执行；--order-by position 对包含关闭成员的查询同样有效。关闭 ITER 继续保留 results 原始顺序。

--view line-readiness --id <Line-ID> 返回 integration_blockers（Topic/未结束 Task）、close_blockers 和结构准入诊断；技术退出、实际代码与批准仍由使用规则核对。

check 检查版本、实体、引用、唯一性、依赖/替代环、队列、状态组合和受管历史重放；不联网，不证明授权、证据链接、语义验收或 Git 可达性。

## 文档信息字段

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py document --store <目录> --table topics --id <T-ID> --field context --ensure
python3 -B <工程>/.agents/skills/rum-project/scripts/workflow.py document --store <目录> --table topics --id <T-ID> --field context
```

Iteration、Milestone 使用 `--field description`（默认字段）维护 DESCRIPTION.md；`context` 和 Task 文档按项目规则仅用于 Topic。通用文档接口允许选择字段，不会在创建迭代或里程碑时自动创建 TASK_CONTEXT。

返回真实路径、当前 hash、是否存在和系统元数据；可直接编辑正文。经接口更新时用 --content-file <UTF8文件> --expect-hash <已读hash>；拒绝过期文档写入。--ensure 幂等创建骨架，不覆盖现有内容。field 为小写 kebab-case，context/design/prd 有固定文件名，其余作为独立 Markdown 字段。Task 的 id 为完整 T-ID/tNNN；closed 文档通过接口纠错需 --correction。项目 rules 字段供已确认的定制说明，不改变固定 JSON schema。

所有 CLI 成功退出 0，参数语法错误 2，结构/业务/IO 错误 1；保留现场并按实际错误恢复。业务写入是原子快照，Git 操作和自由文档不被伪装成同一个跨系统事务。


## 外部集成证据契约

integration_context 固定字段：topic_id、from_line_id、line_id、dependencies、contracts、extensions。前三级 ID 对应业务端点；dependencies/contracts 是本次影响评估引用；extensions 是按命名空间组织的外部证据对象，可以为空。系统不依据 Topic 是否 active 推断代码存在。

integration_proof 为完整 context 加 verified_on、report、verification；verification 同样是命名空间对象，可以为空。context 必须与本次选定评估逐项一致，日期与 review 一致；真实检查与接受由使用规则负责。更换 Line 后不能跳过重验与接受。

Git 的 Line 配置是 extensions.git={branch,base_commit}；Git context 的 extensions.git={baseline,previous_head,prepared_head,retained_ref}；代码 proof 的 verification.git={verified_head}，真实无代码成果使用 {no_code:<实际核对记录>}。这些字段的 Git 格式、存在性与保留语义由 git_evidence/git_adapter 校验，核心系统仅保存。其他接入不必提供它们。

## 外部代码现场

工作树的 target/add_plan/health 在 git_workspaces，是 Git 接入层策略。系统没有创建分支或恢复工作树的能力。Git add/list/check 保留统一预检、路径防重叠、脏文件保护和健康报告，详见已采用 Topic 配置中的 git-integration.md。

## 关闭事实纠错

closure.correct 的 changes 只允许既有 summary/evidence/verification/delivery_record/retention/discard_decision/last_release/extensions/successor/goal_acceptance，且仍满足业务 outcome 的互斥和引用约束。身份、outcome、关闭日期和关联快照不可改；原值和修正值进入历史。外部提交等事实位于 extensions，纠正 Git 事实时使用 Git closure-plan / integration-apply 重新核对并应用；仅改通用摘要/报告引用等、不改代码事实时直接使用本系统接口，不要求源分支存续。系统 check 不证明 Git 有效。

## 相同 Line 的整合恢复

topic.integration.prepare 保持原 from_line_id、当前 line_id。Line 必须 active；Topic/Task 执行先暂停或阻塞，review/ready 退回 paused。旧 context/proof 进入历史，新上下文 required=true，必须重新 review 和接受。相同未验证 context 重复请求不追加空历史。

Git 项目用 retarget-plan / retarget-verify 生成请求，integration-apply 重新核对代码后走同一 Store CAS 接口；代码锚点的不可遗忘规则由接入层执行。没有 Git 的项目按本节通用契约提供真实业务评估与复核。

## Task 结果事实纠错

task.correct 只改变 done Task 的 result，校验非空文本、已有原因/证据/决定；身份、Kind、所属 Topic 和状态不变。当前 result 更新，旧值与新值在历史 before/after 中，按 Task 或 Topic 查询历史均可找到。Topic/Project 已关闭不阻止此操作。未执行或尚在执行的活动不能靠该接口跳到 done；新活动、复测或代码修复仍按各自流程办理。


## 实体创建与说明层准备


project.init 只创建结构化实体。workspace-init/adopt 负责完整工程接入与共享入口生成。topic.create 不自动创建正文；正式开题时 document --ensure 后按 context 模板准备 TASK_CONTEXT；其他文档按需生成。

## 项目内 Topic 短编号

Topic 通过创建历史获得 T1、T2 等连续短编号，不补零，关闭后不复用，超过 T999 继续增长。旧数据直接投影编号，不改写内部长 ID、历史或文档路径。查询中的 short_id 是用户引用编号，id/topic_id 仍为内部身份；Task 使用 T42/t001。命令参数中的 Topic/Task 引用、query --id、document --id、bind-code/observe-code --topic 及 Git 适配器 --topic 支持短编号。旧长 ID 继续有效；不接受 T-1、T01 或模糊缩写。正文与扩展内容不会被替换。创建预检和执行保留 result，并补充 short_id；编号以成功执行后的结果为准。

同一 Store 的过期预检必须刷新；不同设备离线创建可能暂时显示相同短编号，内部长 ID 仍不同。同步拒绝自动合并分配分叉，保留两边和候选。按显式决定从共同历史依次重放分配，使用原长 ID + id_reason 保留身份，再用完整长 ID 重放后续操作/恢复正文与关联，校验通过后显式接受候选；禁止手改 state.json。未共享的短编号不视作跨设备正式分配，冲突期间引用完整长 ID。
