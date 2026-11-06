# 自解释与项目入口

从工程根目录运行 python3 rum.py context 和 python3 rum.py doctor。它们只读取 RumSDLC 自身管理数据与规则安装状况，不读取本地代码现场或适配器私有文件。查询接口用 explain；具体参数用 explain --action topic.create。工具帮助用 tool workflow、tool git_adapter、tool commit_check、tool repo_check，后接 -- --help。

workspace-init 只建立管理数据和空 code 目录；workspace-import 只恢复 meta 和本机规则。代码路径以 context 的 code_paths 为准，可能缺失或为空；agent 按已有授权自行获取、初始化、恢复或绑定代码。普通目录项目也须准备真实内容，不因 meta 导入成功就宣称代码已经取得。workspace-adopt 是显式整目录搬迁操作，继续保留结构安全检查、回滚和恢复；它不是后台观察。

bind-code 只登记显式信息，不发现 origin、当前分支或项目种类。登记代码路径时由 agent 指定 --kind git/directory；Git 分支共享关联用 --topic T-ID --branch 实际分支，不能省略 branch 期待自动探测。规划可无分支，开始实施时由 agent 核对并绑定。项目级 git_adapter 的本地 bind 与共享关联用途不同，均不能替代真实交付证据。

普通目录只使用默认 mainline；保留实际内容基线、测试和交付证据，不调用要求 Git 的适配器，不填假 SHA/no_code。Git 初次接入需确认实际代码来源、作者、目标 Line 分支和已有开发历史；先创建真实初始提交，再按 git-integration.md 配置 mainline，不能制造空 Topic 历史来满足接口。

远端观察是独立显式操作：先通过 bind-code --topic T-ID --review-url HTTPS链接 --target-branch 目标分支登记，再运行 observe-code --topic T-ID --remote。0.7.1 自动支持 github.com PR；其他服务由 agent 使用相应 CLI/API 核验并在 meta 记录，观察返回 unknown 不代表未合并。默认 observe-code 不访问网络，也不检查本地。PR 已合并仅表示历史事件，不证明当前目标未撤销、测试通过、用户接受或授权推送。查询采用调用进程环境；daemon 模式采用服务启动环境，代码托管认证不继承 meta 的选定账号。

仓库/分支/PR 关联保存到共享配置；本机路径和本地来源只在设备登记。适配器的工作树绑定与历史索引在设备 agent-state/git，RumSDLC 不解析。代码树及 .git 不新增 RumSDLC 文件。升级不自动迁移、删除仓库内旧索引，按 git-integration.md 的显式迁移步骤处理。

项目入口、skill 和脚本在代码仓库之外的工程管理目录；外部绑定仓库不注入文件。必须从工程根目录启动 agent 并实际确认规则已加载；进入外部代码目录时读取其有效开发约束。程序安装不更新已有工程。doctor 检查已安装规则的兼容性与完整性；自定义内容会列出，仍须 agent 核对语义。需要更新时，用新全局入口 project-update --project 项目ID --dry-run 预览，再以 --expect-revision 执行；自定义文件须先备份并合并差异，不强制覆盖。中断的工程生成可用 agent-install 显式恢复；若写入进程被强制终止并留下 Store 锁，先通过 tool workflow 的 lock-status 核对写入者，确认退出后按 lock-recover 的归属令牌要求显式回收，再恢复生成。

meta 同步与代码操作完全分开。meta 账号由 setup 管理；代码作者、SSH、代理、凭据按仓库实际环境处理。无论本地检查或远端查询结果如何，都不得自动推进生命周期、认证交付或执行推送。脚本覆盖不足时按 git-fallback.md 继续核实，保留已有授权与现场。


0.8.1 工程脚本由本工程上下文确定设备 home，终端中的其他设备环境变量不改变归属；需要独立适配器状态目录时显式 --state-home。转换恢复完成前不继续运行代码工具。doctor 的 compatible 只说明契约兼容；runtime_gate_ready=false 或 warnings 非空时，先按提示更新/复核再依赖新版升级互斥。缺少必需脚本、核心依赖或脚本语法损坏会明确不健康；自定义规则语义仍由 agent 复核。


0.8.2 便捷入口保留显式工程/路径的等号及合法缩写，不覆盖为默认目标。更新/重建工程资源期间保持单写入者；若检测到准备后变化，保留编辑并刷新预览；若已开始写入则按恢复记录处理，不删除记录绕过。实际复核安装版本和工程版本，避免用保留的旧脚本验收新版行为。


0.8.3 的 CLI 路径以发起命令的工作目录为准，即使后台服务从另一目录启动也保持一致；--path、--parent、每个 --root 均适用。项目便捷入口显式传入的相对路径同样相对于调用目录。~ 由调用进程展开，服务不重新解释；符号链接仍明确拒绝。


0.8.5 区分逻辑 Project 与物理 Store：核心令牌保护状态/请求，Git 适配器的 code_revision 保护操作现场和适用代码事实。所有 integration-apply 都携带两个令牌，摘要纠错也固定现场但不要求原代码分支存在。current/check/list 不允许用另一份同 Project Store 冒充已有绑定；current 自动发现 Store 后仍检查设备归属和转换状态。显式更新旧工程脚本并重新预览。
