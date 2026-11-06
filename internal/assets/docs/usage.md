# RumSDLC 0.9.0

系统本体直接运行原生可执行文件，不需要 Python、Go 或额外安装步骤。
涉及仓库的操作需要用户自行安装 Git。工程中的 agent/skill 继续使用 Python，
并保留原有的 Python、Git、POSIX 及其他工具依赖。Windows 原生系统程序与
项目脚本的运行环境是两个独立边界。

从 0.9.0 起系统层使用 Go 迭代。程序与新项目规则统一为 0.9.0，核心数据协议仍为
4.1.2。旧项目保留原有规则版本，通过显式 project-update 更新。

## 直接使用

将对应系统与架构的 `rum`（Windows 为 `rum.exe`）放在长期保留的位置。
macOS/Linux 使用 `./rum`，Windows PowerShell 使用 `./rum.exe`。
无需配置 PATH；下面示例省略 `./`。离线使用不需要 GitHub 登录。

```text
rum --version
rum --help
rum --home /data/rumsdlc workspace-init --parent /work/projects --name demo --date 2026-11-06
rum --home /data/rumsdlc list
rum --home /data/rumsdlc context --project PROJECT_ID
rum --home /data/rumsdlc doctor --project PROJECT_ID
```

`--home` 是命令前的全局选项，支持 `--home=PATH`、`--ho PATH`；也可使用
`RUMSDLC_HOME`，默认设备目录是用户主目录下的 `.rumsdlc`。多设备或测试时应
分别指定目录。服务套接字的完整路径须小于 100 字节。相对路径和 `~` 由调用
命令的进程解释，启动了后台服务也不会改变路径含义。

`workspace-init` 创建 `code/`、`meta/`、项目说明和 skill；
`workspace-adopt --path DIR --date DATE` 接管现有本地目录并保留内容；
`workspace-import --repository OWNER/RumSDLC_ID --parent DIR` 导入远端管理数据。
仅管理 Store 时使用 `create --path DIR --name NAME --date DATE`、`register --path DIR`
或 `import --repository OWNER/RumSDLC_ID --path DIR`。

每条命令的参数通过 `rum COMMAND --help` 查询；`rum explain` 返回命令、数据模型、
命名规则和本手册的位置；`rum explain --action ACTION` 返回某项业务操作的参数。

## 可选的固定入口与升级

直接执行下载的程序即可使用。需要固定全局命令和可切换版本的入口时再运行：

```text
rum --home /data/rumsdlc install --destination /apps/rumsdlc/0.9.0 --bin-dir /commands
```

升级使用新下载的原始可执行文件以及新的版本目录，不能使用已经安装的转发入口
覆盖该入口自身。先停止运行中的 RumSDLC 命令与项目脚本；后台服务用
`rum daemon stop` 显式关闭。安装器发现运行中会拒绝升级，不强制停止进程。
从 0.8.0 之前的版本首次升级，需要确认旧入口均已关闭后增加 `--confirm-legacy-idle`。

安装不登录、不启用同步、不启动服务，也不读取或改写工程数据和 registry。
入口变更采用事务记录；普通失败回退，进程中断使用下载的程序执行：

```text
rum --home /data/rumsdlc install --recover-only
```

恢复保留外部编辑和无法确认归属的锁。旧版全工程安装事务必须显式恢复；
不要删除记录来绕过冲突。工程资源更新是单独操作：

```text
rum project-update --project PROJECT_ID --dry-run
rum project-update --project PROJECT_ID --expect-revision PREVIEW_TOKEN
```

需要接续中断的项目资源更新时使用 `agent-install --project PROJECT_ID`。
更新会保护自定义内容；`doctor` 检查兼容性与完整性，不自动修复。
未显式更新的旧工程保留原资源和原有环境依赖。

## 元数据同步与代码边界

GitHub 同步可使用已配置的 Git 凭据助手或 `GH_TOKEN`/`GITHUB_TOKEN`。
已有 `gh` 登录也可继续使用，但系统不要求安装 `gh`。固定账户时使用
`setup --confirm-login LOGIN`，其凭据必须可从该账户的已有登录或 Git 凭据助手取得；
`setup --global-credentials` 选择全局凭据策略。

```text
rum identity
rum setup --confirm-login alice
rum project-identity --project PROJECT_ID
rum enable --project PROJECT_ID
rum sync --project PROJECT_ID
rum disable --project PROJECT_ID
rum discover
```

管理仓库使用独立 Git 数据与账户策略，代码仓库保留本地 Git 认证和作者设置。
`bind-code` 记录路径、仓库、Topic 分支或 review 关联；系统不会自动巡查代码，
也不会代替 agent 推送或验收。`observe-code --project ID --remote --topic TOPIC_ID`
只读取显式登记的远端 review 事实，不修改业务状态。

冲突保留候选与冲突 ID；按报告完成核对后，使用
`resolve --project ID --conflict-id ID --accept`，或 `conflict-retry` 重新准备。
不同实体的独立变更可自动合并；同一实体冲突须显式处理。

## 流程数据、文档与迁移

`platform --project ID --operation OP --args JSON` 提供工作流接口，
`--args @FILE` 读取调用者目录中的文件，`--args -` 读取标准输入。
写操作先以 `dry_run: true` 获取 revision，再提交 `expect`；文档使用内容哈希保护。
Project、Milestone、Iteration、Line、Topic、Task 的原有约束与历史验证保持有效。

独立 Store 可使用内置原生工具，无需 Python：

```text
rum tool workflow -- command --store DIR --request request.json --dry-run
rum tool workflow -- command --store DIR --request request.json --expect-revision TOKEN
rum tool workflow -- query --store DIR --view overview --format markdown
rum tool workflow -- check --store DIR
rum tool workflow -- lock-status --store DIR
rum tool migrate -- --help
```

`tool migrate` 将支持的旧版快照转换到新空目录，预览令牌同时固定源内容和目标，
保留源文件归档与文档，失败不删除其他写入者的文件。锁恢复需要匹配 owner 和
显式空闲确认，未知或活动写入者的锁不会被回收。

`tool git_adapter`、`tool repo_check`、`tool commit_check` 是原有项目 Python 工具，
仍要求其 Python 环境。项目中的 `rum.py` 和 workflow 入口仅转发到本机 Go 程序。

## 可选后台服务

```text
rum daemon start
rum daemon status
rum daemon stop
```

默认同步周期 60 秒、发现周期 600 秒，可通过 `--interval` 和
`--discovery-interval` 调整；同步周期至少 5 秒，发现周期不得更短。
`daemon run` 前台运行，便于交给操作系统的服务管理器。
未启动服务时，CLI 直接使用相同应用逻辑；不需要提前配置服务。
