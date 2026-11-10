# 实体名称、简称与存储名称

领域说明统一使用下列首字母大写的正式实体名。简称和 ID 前缀不改变实体名称；接口/字段的小写形式是序列化约定，不是另一套实体。

| 正式名称 | 中文 | 简称或 ID 示例 | 当前存储/接口 |
|---|---|---|---|
| Project | 项目 | P-example | project；project.* |
| Milestone | 阶段里程碑 | M；M0、M1 | milestones；milestone.* |
| Iteration | 迭代/轮次 | ITER；ITER-001 | worksets 中 type=iteration；workset.* |
| Pool | 持续候选池 | POOL | worksets 中 type=pool；随 Project 初始化 |
| Line | 业务开发线/维护线 | mainline、engine 等稳定 slug | lines；line.* |
| Topic | 开发主题 | 大写 T；用户编号 T1、T2；内部 ID T-YYYYMMDD-xxxxxxxx | topics；topic.* |
| Task | 一次具体活动 | 小写 t；t001 | tasks；task.*；用户引用 T1/t001；内部 ID 为 Topic-ID/t001 |

Iteration 不叫“ITER 实体”，Milestone 不叫“M 实体”。叙述中可以在含义已明确后用 M/ITER；标题、术语定义和对外介绍优先写完整名称。MS 不是本包的正式缩写或编号前缀，不将既有 M0 改为 MS0。

Workset 是 Pool/Iteration 共用的工作集合存储抽象，不改变 Iteration 的正式名称；workset.* 不表示能重复创建 Pool。Line 没有强制 L 前缀。Task 的 t001 只在所属 Topic 内唯一。

entity.rename 改具体实例显示名称，不改实体类型、ID、路径或归属。短编号按受管创建历史投影，既有内部 ID 和路径保持；关闭与归属迁移不改变短编号。

M、ITER、T、t 的实体映射不改变。它们不替代分支/提交的 feat、fix、docs、refactor、chore 等常规类型；可作为自愿附带的上下文。分支名称和消息中的编号不是实体身份或绑定来源，关联须显式登记。
