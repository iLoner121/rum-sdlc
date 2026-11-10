# 规划与关系维护

Iteration、Milestone 的说明统一使用 document 的 description 字段，定位 DESCRIPTION.md；每个实体只维护这一份基础说明，按实际需要记录目标、范围选择及调整理由、关键决定、复核和结束纪要，不要求预建空章节。TASK_CONTEXT 与 Task 仅用于 Topic，规划工作不在 Iteration 或 Milestone 下建立执行文档；具体需求、设计与验收引用相关 Topic，成员和进度通过系统查询。已有规划层 TASK_CONTEXT 在整理时将有效纪要并入 DESCRIPTION.md，保留重要决定并修复引用，不保留两份正文。

Project 是数据边界。M 管有顺序的阶段目标，工作集合管当前选择，Line 管代码交付路线；三者独立，不嵌套、不强制一一对应。

新需求先写 Pool 的候选正文，记录动机、未知与优先理由，不立即创建空 Topic。ITER 可先建空 draft 并说明目标、候选和待拆分事项；选定内容足够明确后创建或复用 Topic，propose 提出成员，用户确认范围后 activate 冻结初始范围。认可计划不自动授权整批实施、具体代码提交或推送。

全项目一个 Pool、最多一个 draft/active ITER、最多一个 active M。M 默认 planned，有明确阶段顺序；启用前前序结束。Topic 通常指定 M，确无适用阶段时显式记无归属原因，在 orphans 和 overview 中可见；不建立永不结束的兜底 M。

近期规划核对需求/验收、重复冲突、依赖未知、拆分与 spike、架构及 Line 三条件。下一项足够可开始即可，不提前冻结所有实现活动。新里程碑建立/复核受影响架构。调查结果可反馈规划，不因早期决定硬套开发线。

Topic–M、Topic–工作集合、Topic–Line 只由统一接口修改关系表；从任意视角发起操作结果一致，不要求切换代码分支。关系操作使用已有授权与影响证据，不能因为涉及自己就任意改变范围。修改正式承诺或架构路线仍由用户决定；agent 在已有决定内落实，不新增逐命令询问。

运行中范围默认稳定；新想法入 Pool。用户要求的紧急 fix 可插入并说明推迟/暂停项与恢复点；其他新增等待或由用户重新选择。目标仍成立则 change 调整范围，目标失效先 replanned 收尾再开新轮。关系移动不换 Topic ID，不复制需求、不抹去原 ITER 的参与历史。从 ITER 移出时，两种入口都必须明确 destination：后续去向/恢复条件可直接引用已有决定，不新增逐命令询问。

Topic 完成不自动领取下一项、不删除当轮历史成员。真实完成/失效/全阻塞/用户要求时复核本轮；由用户决定等待、调整或结束。距 last_reviewed_on 满 14 个自然日，在下次项目会话复核，不补空迭代、不强制双周交付、不创建后台提醒。普通读取、单次活动和范围变更不伪装成复核。

ITER 结束要覆盖全部实际选入项，已验收未整合不记 completed；进入开发线明确真实交付范围。未完成项回 Pool 并记 carry-forward/backlog，后者不自动选入未来轮。整轮 completed 按经批准调整后的目标达成判断，取消/替代/移出成员仍逐项如实记结果；不能要求它们冒充 completed。goal_acceptance 或统一决定引用须覆盖实际目标接受。关闭结果冻结，不随 Topic 后续变化更新。M 结束前未完成成员明确转入其他阶段或无归属；阶段完成仍需要整体结果验收，不能由 Topic 全关闭推导。

接口和记录格式见 api.md，实体与当前/历史视图见 model.md。定制正文可编辑，名称、排序、状态和关系必须经接口。


复核本轮进展时用 iteration-progress 的 scope；需要当前待办用 current，需要历次选入/移出用 history。不要因 closed Topic 已离开当前队列，就把它从本轮完成进展漏掉。阶段安排用 milestones 的 position 顺序，不按 M 编号字符串猜先后。


Topic 正式开题前按 [项目执行入口](../SKILL.md) 使用项目共享执行手册，不复制 Topic 级入口，按需准备文档；topic.create 不自动完成这些说明层步骤。正式名称是 Milestone/Iteration，M/ITER 仅为简称及编号形式，见 [terminology.md](terminology.md)。
