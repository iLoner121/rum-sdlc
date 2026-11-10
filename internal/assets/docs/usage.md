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

## Topic 编号与开发分支

用户引用 Topic 时统一使用 `T1`、`T2`，不补零。编号在同一项目的追加创建历史中稳定；关闭、调整里程碑/迭代/开发线不改变编号，超过 `T999` 后继续使用 `T1000`。原有 Topic 也按创建历史获得编号。

查询 JSON 增加 `short_id`，保留机器关联用的长 `id`；创建预检/执行保留 `result` 并增加 `short_id`。例如 `rum tool workflow -- query --store meta --view show --table topics --id T1`，以及 `rum tool workflow -- document --store meta --table topics --id T1 --field context`。结构化操作参数支持短编号；Task 可写作 `T1/t001`。既有长 ID、正文路径与历史引用继续可用，不接受 `T-1` 或 `T01`。

新分支建议为 `feat/T42-export-report`；类型保留 `feat/fix/docs/refactor/chore`，spike 使用 chore。显式合法名称和已有分支关联优先，名称不决定 Topic 归属。

短编号的作用域为 Project 的共同历史。共享 Store 上预检后有其他写入，需刷新预检再执行。不同设备离线新建时可能出现相同临时编号；长 ID 仍区分身份，同步会停止自动合并并保留冲突候选，错误列出冲突编号及双方长 ID。先决定分配顺序，从共同历史通过正常预检/API 重放创建（用原 `id` 和 `id_reason`），再以长 ID 重放后续操作，恢复文档/关联并检查；仅在候选正确后显式接受。共同历史编号保持，尚未共享的编号按恢复顺序确认；冲突期间用完整长 ID 沟通，不手改 state.json。

## 管理仓库命名与发现

创建管理仓库时保留 `RumSDLC_` 前缀，后缀默认取已登记的代码仓库名，没有代码来源时取 Project 显示名。
使用 `rum repository-plan --project ID` 取得可编辑建议；例如代码仓库 `source-app` 对应 `RumSDLC_source-app`。
使用 `--name edited-name` 预览自定义结果，`rum enable --project ID --repository-name edited-name` 采用最终输入创建。
显式输入可以是后缀，也可以含完整前缀；输入必须是合法 ASCII 仓库名，总长不超过 100 字符。
自动建议会规整不支持的字符，结果为空时提示建议 `RumSDLC_project`，可继续编辑。
原 `enable --repository owner/name` 用于明确选择既有仓库，与 `--repository-name` 互斥。
同一 Project 已存在的有效仓库按身份恢复，旧长名称不自动改名；名称占用时选择其他名称，不自动覆盖。

`rum discover` 只读取可访问的 `RumSDLC_` 前缀候选，逐一核验根目录 `.rumsdlc-project.json`。
`manifest_status` 分为 `valid`、`missing`、`invalid`、`unavailable`；403、限流和网络失败不会伪装成文件不存在。
`name` 是仓库名，`display_name` 是最佳可用展示名，`project_name` 是可获得的项目名，`name_source` 说明名称来自身份文件、本地项目、旧项目配置还是仓库名。
缺少/无效身份文件的候选保留诊断，不自动注册为项目；无代码项目同样可以被发现。
身份文件写入 `name` 展示项目名，现有三字段文件可读并在下一次同步时自动补充该字段；没有该字段时也可从现有 `project-config.json` 读取。显示名不用于判断 Project 身份。

保留前缀的远端改名可以在显式 `sync` 或 `enable` 时恢复：必须同时核对数字仓库 ID、Project ID 和 genesis。
核验成功才更新本地配置、设备绑定及单一的 origin fetch/push URL；不同仓库、身份文件不匹配或 origin 被另行修改时拒绝操作。
远端旧名称已被其他仓库使用时，仍按原数字 ID 查找，不能按同名接管。整个流程不改变项目 ID、代码仓库关联或业务历史。
该能力恢复已经发生的远端改名，不主动请求 GitHub 为仓库改名。

## 管理仓库提交消息

所有管理仓库自动提交（检查点、自动合并、接受冲突结果、仓库改名配置刷新）统一为单行：

```text
2026-11-11T00:32:10+08:00 | user="iLoner121 <80140230+iLoner121@users.noreply.github.com>" | device="EdvinMac-mini"
```

时间采用 RFC3339、精确到秒并保留时区，与该 commit 的 author/committer 日期一致。
`user` 使用管理仓库实际提交者的 Git 姓名与邮箱，Git 自带的身份字段仍正常保存。
`device` 默认使用本机 hostname，名称不可用时回退本机登记的 device_id；字段按引号字符串转义，保持单行。
这些信息在消息中重复呈现是为了统一浏览，不用于推导 Project 身份或同步权限。
无变化不创建新提交，既有历史消息不重写；产品代码仓库仍遵守自己的提交规范。
