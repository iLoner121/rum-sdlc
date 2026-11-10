---
name: rum-project
description: 在接入 RumSDLC 的工程内规划 Milestone、Iteration、Line、Topic 和 Task，执行需求、设计、开发、验收及代码 Git 流程。适用于项目内工作，不负责安装 RumSDLC。
---

# Project 执行手册

本体只负责流程、文档、设备登记、显式代码关联和自身 meta 同步；不检查本地代码，不读取适配器索引。代码获取、Git 初始化、现场核验、分支、提交、合并、推送与交付由本 skill/agent 负责。

接管先读 context/doctor，再由本 skill 核验实际代码。doctor 正常不证明代码就绪；远端观察也不授权代码动作。Git 实施前由 agent 确认实际 Topic 分支，创建/绑定后显式登记共享关联。命名是 agent 的可覆盖 rules：新分支默认建议 <type>/<Topic短编号>-<slug>，如 feat/T42-cache-timeout；提交消息默认带真实 Topic 标识，首次/整合提交或规划归属变化时补充已存在的 M/ITER。用户或项目约定可替换、精简或取消这些管理标识，无须改系统；名称和消息不决定 Topic 归属，实体缩写不替代 feat/fix/docs 等 Git 类型。普通目录只使用默认 Line，真实代码仍需验证和交付，不虚构 Git 或 no_code。

Git 代码操作前运行项目脚本 repo_check.py inspect，并按操作读取 Git 准则。检查不完整、失败、脚本不支持或仓库结构特殊时，必须读取 [git-fallback.md](references/git-fallback.md)，补齐与本次操作有关的具体证据；关键事实未知就暂停该代码动作。不得凭脚本退出 0 或 RumSDLC 状态推送。

本体自解释与工具入口见 [entry.md](references/entry.md)。

先运行工程根目录的 rum.py context，核对 Project 与当前会话要处理的 Topic。没有指定 Topic 时先查询，不选择一个全局当前 Topic。进入某 Topic 后读取其 TASK_CONTEXT，再按当前活动读取所需准则。

身份、名称、生命周期和当前 Milestone/Iteration/Pool/Line 归属以系统为准，不在本文件维护实时台账。保留宿主与代码工作区有效约束。系统调用见 [api.md](references/api.md)，名称对应见 [terminology.md](references/terminology.md)。

## 接管与上下文

核对真实 Store、Project、Topic 和代码现场（如有），运行 check/overview/resolve，读取项目说明、已确认约束和当前 Task。系统调用仍需本机兼容运行时；不因路径缺失而重建项目、复制脚本进 Topic 或直接编辑 JSON。

TASK_CONTEXT 记录目标、各正文入口、当前有效结论、活动指针和恢复点，按 [context.md](assets/context.md) 的章节维护。开题时未知部分可留空/待确认；开始对应活动前要具备其输入。空文件不证明需求明确、方案通过或验收完成。

## 文档逐步准备

| 文档 | 何时准备 | 内容 |
|---|---|---|
| TASK_CONTEXT.md | 开题先有章节，交接时更新 | 最小上下文与正文入口；没有独立文档时承载简明需求/方案/验证摘要 |
| PRD.md | 独立需求评审/交付或外部行为协调需要；已有需求优先引用 | 目标、范围、行为、验收条件；见 [requirements.md](references/requirements.md)、[prd.md](assets/prd.md) |
| DESIGN.md | D1—D6 任一命中；已有 DESIGN 继续维护 | 技术方案、实施路径、偏差；见 [design.md](references/design.md) |
| acceptance.md | 用户要求、分批/人工验收、跨活动汇总等需要 | 判据、实际检查、结果、接受与交付证据；见 [verification.md](references/verification.md)、[acceptance.md](assets/acceptance.md) |
| tasks/tNNN.md | 对应活动开始前 | 一件活动的输入、过程、产物、结论与交接；见 [task.md](assets/task.md) |

不一次生成全部空文件。对应需求、方案、验收判据必须在相应工作前明确，但不强制各有独立文件。独立正文建立后，上下文保留摘要和链接，不双份维护。

## 活动与实施

按 [tasks.md](references/tasks.md) 选择 investigate/design/implement/verify/process 等实际活动。仍做同一件事则恢复，否则 task.create、写目标/输入/产物/停止条件、再转 active；需要另一件事时先交接再建后续活动。不强制类型顺序，不按命令、会话或提交碎拆，记账不递归建 Task。

正式实施前按 design.md 核对 D1—D6、A1—A4 和技术准入。仅要求调查/方案不推断实现授权；已有决定不重复询问，授权内普通活动连续推进。Task done 表示活动结束，结果可以失败/未知，不替代设计批准或 Topic 验收。

## 验证、接受与结束

按 verification.md 选择真实检查，记录输入/基线/环境、结果和证据；未执行、必需失败不写通过。当前范围自检满足后 topic.state → review；用户接受实际结果且人工验收完成后 ready。需调查、设计或改实现先回 active。

交付按 [closure.md](references/closure.md)：实际成果交付并复核目标，先结束最后收尾 Task，再 topic.close。取消/替代须已有用户决定和成果处置，不伪造 done。完成后复核本轮并推荐，不自动领取下一 Topic。规划与 Line 见 [planning.md](references/planning.md)、[lines.md](references/lines.md)。

记录错误用 task.correct/closure.correct，不重开关闭生命周期；真实新问题另建后续工作。受管写入先预检再带令牌，已有授权沿用；字符串/脚本成功不认证授权、接受或证据真实。

## Git 项目按触发点读取

以下 Git 规则适用于 Git 项目。非 Git 本地代码按实际内容、测试和交付证据推进，不调用要求 --repo 的工具；没有 Git 不等于没有代码。纯业务/真实无代码成果不虚构分支或 SHA。

- 代码现场创建/恢复、绑定、换 Line、证据：读 [git-integration.md](references/git-integration.md)。普通串行可复用，需要并行或隔离脏现场才增工作树，Task 不另建分支。
- 具体 commit/merge/amend/rebase/push/清理：先读 [git-safety.md](references/git-safety.md)、[git-conventions.md](references/git-conventions.md)。展示准确结果与消息，取得/沿用本次明确批准，preflight/postcheck 不代替批准，不绕过 hook/签名。
- 代码交付先整合并在目标验证，适配器覆盖完整时再 closure-plan → integration-apply，同时携带平台令牌与现场令牌；所有 integration-apply（含 Line 配置/状态、迁移、验证、摘要纠错）均使用双令牌；这是关闭记录预检，不是线上回归或新增审批。否则按 git-fallback.md 补证并正常系统预检/写入；真实无代码成果用 no_code。目标变化先暂停、刷新同 Line context，再做受影响验证及接受。

保留代码与恢复依据，不自动 stash/reset/force/prune/删除。通用关闭摘要纠错不要求原分支仍存在；实际代码交付、保留或发布事实变化仍重验。
