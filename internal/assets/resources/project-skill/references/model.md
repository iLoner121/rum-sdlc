# 系统模型与分层

版本 4.1.2；所有版本标记使用完整发布号。

兼容同一数据模型的 4.1.0/4.1.1 完整快照，已有实体/关系与后续新增行继续沿用该快照版本，历史原样保留；新项目使用 4.1.2。版本兼容不允许混合实体版本或跳过结构/历史校验。4.0.x 的旧字段仍需单独处理。

## 存储

一个工程的 meta 目录对应一个 Project。state.json 是唯一受管快照，逻辑表为 project、milestones、worksets、lines、topics、tasks，以及 relations.milestone / workset / line。一个 JSON 文件容纳这些表，确保一次关系修改、相关状态失效和历史记录共同原子替换，避免跨文件只写一半。以后可替换存储，保持 service.request / Store.execute 的操作契约。

每个实体及关系行带完整版本和 project_id。ID 稳定，名称通过 entity.rename 修改。Topic 中不保存当前 M/ITER/Line 反向字段；父实体中也不再维护当前成员列表。关系查询和成员查询读取同一张表。

Project 初始化生成唯一 Pool 和唯一默认 mainline。worksets.type 为 pool/iteration，同表存储；Pool 的唯一性是业务约束，可以以后放宽。Topic–M 可暂缺；未关闭 Topic 恰好一个当前工作集合、一条 Line。工作集合关系带排序位置。Line 类型为 mainline/development/maintenance；默认主线在项目存续期间保持 active，是否接入代码分支由外层配置决定。

M0、M1 稳定编号，同时有 position 和 planned/active/closed；最多一个 active，激活前更早阶段须结束。只能对尚未开始的阶段重新排序，不用常驻 Goal 代替阶段 M。没有 M 的 Topic 通过 orphans 查询。

## 受管字段与信息文档

版本、ID、名称、类型、顺序、状态、关系、历史与关闭事实是受管字段；不得直接编辑 state.json。接口检查整份快照和历史重放一致性，可发现普通误改，但不是操作系统安全隔离，也不认证授权或证据真实性。

自定义内容在 documents/。Topic 的 context/design/prd 分别定位为 TASK_CONTEXT.md、DESIGN.md、PRD.md；Task 正文为所属 Topic 的 tasks/tNNN.md。文档不包含权威 Status/Kind/Topic 元数据，查询返回实体元数据。背景、PRD、方案、过程和交接可自由写，不复制状态/归属台账。

## 历史

每次受管改变追加 revision、原请求、日期、原因、决定/证据引用和 before/after 补丁。无变化不追加。当前关系只有一份，历史补丁不是另一个当前修改入口。读取时重放与受管快照比较。

ITER 激活冻结 initial_topics；当前工作集合成员只在关系表。关闭结果覆盖激活和运行期间实际选入的所有 Topic，包括已移出者；草案提议不算正式承诺。未完成成员收尾后回 Pool，carry-forward 只表示下一轮候选；后续进入新 ITER 不改旧轮结果。

closed Topic 移除当前队列关系，保留 M/Line 历史归属及 closure 的交付目标、外部证据、验证、结束时关联快照；不能因 Line 后来进入主线而改写原交付范围。closed 不重开；名称、描述和可变关闭事实可带原因纠错；done Task 的 result 使用 task.correct，即使父对象关闭也保留原值和修正依据。受管关闭事实通过 closure.correct，原值/修正值/依据进入历史；身份、outcome、关闭日期和结束时关联快照不由该接口修改。

## 写入与并发

每次操作 dry-run 返回摘要与令牌；正式执行重新读取全部受管快照和请求，拒绝过期令牌。应用期间使用本 meta 目录的 .workflow.lock；state.json 写临时文件、同步并原子替换。读者只看到完整旧版或新版。失败不自动修改业务状态，预检不写文件或锁。

锁异常遗留先核实无运行写入者；用 lock-status 得到 owner token，lock-recover --expect-owner TOKEN --confirmed-idle 显式恢复；同机仍运行的记录进程被拒绝。未完成 owner 文件不能自动裁定。原子写之前的崩溃可能留下未引用 .tmp 文件，核实无写入者后可清理；业务读取不将它们当有效状态。

单主分支不代表跨设备单写者。此锁只解决同机使用同一目录的竞争；不同客户端需先取得最新共享快照，避免同时推进同一 Topic。Git 同步冲突不得手工拼接 revision/history 或选择一份覆盖；先保留双方数据，在统一有效基线上重新预检待执行请求并审查业务差异。本版本不自动实现离线合并、分布式锁或同步服务。

## 三层依赖

系统层是 validation / model / closure_rules / integration / service / store / queries；workflow.py 仅作 CLI 转换。业务 Line 可以没有代码仓库，也没有必需的 branch/base_commit 字段。resolve 返回 Line ID，不推导 Git 分支。业务关闭用 delivery_record、retention、last_release 表达交付、保留和最后发布的证据引用，格式不限定为代码提交。

每个实体有 extensions 对象，按命名空间保存外部接入配置；整合 context/proof 和关闭事实也可附带 extensions/verification。系统只校验 JSON 结构、历史、关联与证据对应，不解释命名空间内容。通过 extension.set 整体更新一个命名空间，仍需预检、原因、证据与已有决定，不得手改 JSON。扩展不能覆盖核心字段或改变业务状态门禁。

Git 接入由 git_adapter / git_workspaces / git_evidence / git_validation / commit_check 实现。Line 的 extensions.git 保存 branch/base_commit，由 line-configure 核对后通过同一个系统接口写入；代码工作树、创建/恢复、基线、提交祖先关系和引用保留策略全部属于接入层。Git 关联不授予特殊业务编辑权。其他开发系统可使用自己的命名空间，不需要模拟 Git。

整合准备与 Line 关系是不同业务动作。同一 Line 的 context 刷新使用 topic.integration.prepare，保持原来源与当前目标，旧验证和接受失效。Git 适配器额外保持原始代码锚点、核对实际保留；从历史动态发现保留引用，不在平台建立重复可编辑的代码引用台账。


## Topic 使用规则配置


项目执行规则由工程父目录的 AGENTS.md 与共享 skill 提供，每个 Topic 仅保存其上下文和任务正文。
