# 主线、开发线与维护线

Line 具有稳定身份、名称、类型、业务目标、状态及结束事实；外部开发环境是可选接入。Project 初始化生成默认 mainline；它在项目存续期间保持 active，没有 integrating/关闭临时线的流程。

development 只有同时满足三条件才建立：多个有局部验收边界的 Topic 共同完成技术过渡；必须共享暂不能安全进入主线的基线且障碍具体；已经评估小步兼容、隔离入口与功能开关并说明不适用理由。仅提到大版本、重构、里程碑或工作量大不足以建线。Line 与 M/ITER 独立。

maintenance 仅在已发布版本需要独立维护、主线已前进不适合直接发布旧版本时建立，从明确发布基线出发。单有版本号不足以建线；支持范围、传播到主线/其他版本的去向和最后发布点明确。不能默认整条维护线进入主线。

通过 line.create 登记业务 Line。Git 项目另用 line-configure 保存 extensions.git 的实际分支与起点，再创建/检查代码环境；开展代码活动前两者均核对。描述文档写隔离理由（三条件逐项证据）、目标/支持范围、基线、退出条件、验证方法、范围变化与恢复入口。成员由关系查询，不手写第二份列表/状态台账。

开发线 active → integrating → closed；active/integrating 可 paused；恢复 active 或重新满足全部准入后 integrating。维护线不使用 integrating。Git Line 用 line-state-plan / integration-apply 核对实际 tip 后记录业务状态；纯业务项目直接 line.state。状态请求的 evidence 允许 null（新版明确规则），原因和事实核对仍需真实；暂停必须明确恢复条件；恢复核对现有代码 tip，不因状态改变就自动恢复成员。

进入 integrating 前技术退出条件已有实现证据，有成员且全部 ready/closed，无 planned/active/blocked Task，待办仅整体核验、整合和记录。整合阶段仅已有 review/ready 成员可执行 verify/process 与关闭，不得新建成员或调查、设计、修改代码。发现需要修复、冲突处理、新成员或方案改变先退回 active。

成员可记录 paused/blocked 而不破坏结构；它们不得在 integrating 执行或关闭。固定恢复顺序：记录原因/失效证据 → Line 回 active → 核对成员条件并恢复 Topic active → 重做受影响设计、验证与验收 → 重新满足整合准入。父 Line 恢复不自动解阻成员。

Topic 交付开发线后可关闭，仍报告开发线交付；整条线后来进入主线不会改写 Topic 的原交付事实。全部成员已关闭后，整体核验记录在 Line 描述；需要代码修复则回 active 并建立新 Topic，不复活旧成员。

Line 关闭前全部成员关闭或转出。开发线 integrated 需要整合后验证、当前 source tip 确实包含于整合提交且提交在主线历史中；实际 Git 证据用 git_adapter.py line-delivery 核对。取消/替代须用户决定和真实保留 ref 或明确丢弃决定；替代有开放后继。维护线 retired 保存真实保留 ref 与最后发布 ref/提交，需要支持终止决定。

Line 状态与关系接口没有代码分支身份门禁，实际 Git 证据由外层工具和 agent 核对。关闭不删除分支/目录，清理仍需授权和代码可达性/现场检查。


改变 Topic 的 Line 时，关系由系统层统一修改；Git 实际准备、源历史保留、脏代码检查、新目标 baseline/HEAD 祖先核对由 git-integration.md 的迁移流程提供事实。Line 成员准入阻塞用 line-readiness 查询，不凭手工成员列表判断。Line 关闭参数按 outcome 严格互斥：非 integrated 不接收 delivery_record，非 superseded 不接收 successor，非 retired 不接收 last_release；代码提交和保留引用的互斥由 Git 接入层另行检查。


保留代码的取消/替代/维护结束，先用 Git 实现的 line-retention --repo <代码树> --store <目录> --line <ID> --retained-ref refs/heads/<保留分支> 核对真实源 tip 仍被保留。维护线另加 --last-release-ref refs/tags/<实际发布>；工具解析实际发布 SHA 并核对其在维护线历史中，将返回值记入 line.close 的 extensions.git.last_release_commit，业务 last_release 引用对应发布记录，不把无关主线发布当作最后维护发布。明确丢弃仍走已有用户决定，不用空的保留声明代替。

系统关闭使用通用交付／保留／发布记录，Git 项目按 git-integration.md 的 closure-plan/integration-apply 检查实际代码；没有 Git 的项目使用自身真实交付证据，不虚构分支或提交。

普通目录项目按项目级 skill 仅使用默认 mainline，不虚构 Git 主分支。核心不通过检测 .git 限制 Line 方法；agent 根据本体的 Line 列表核对是否存在额外 Line，并按本规则安排工作；本体不从代码现场生成提醒。尚未获取 Git 代码的设备不等于普通目录模式。
