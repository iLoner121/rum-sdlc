# RumSDLC

RumSDLC 为 agent 驱动的开发提供工程管理、可追溯的 SDLC 工作流、并发写保护和 Git 元数据同步。
从 **0.9.0** 起，系统本体使用 Go 开发，发布为可直接运行的单个程序。

- **系统本体**：运行机器不需要 Python、Go；仓库操作需要用户自行安装 Git。
- **安装 skill**：仅下载、校验并调用本体的安装程序，不包含另一套安装或业务逻辑。
- **工程资源**：创建、接管、导入工程时，由本体投放到工程目录；项目级 agent/skill 和脚本保留 Python 及原有工具依赖。显式更新、修复工程时也由本体管理这些资源。

安装或升级本体不会改写已有工程、自动登录、启用同步或启动后台服务。
本体内置项目资源；使用时按需写入工程目录，离线手册和显式调用的工具可能写入设备缓存。

## 把这个仓库交给 agent

把下面这段话和仓库链接发给能运行本地命令的 agent 即可：

> 请按照 https://github.com/iLoner121/rum-sdlc 的 README 和 installer-skill/rum-sdlc-install/SKILL.md，安装最新正式版 RumSDLC。保留已有工程与设备配置，检查 Git 是否可用，并告诉我安装版本、命令路径和验证结果。

[安装 skill 的完整入口](installer-skill/rum-sdlc-install/SKILL.md)负责选择平台、核对发布包 SHA-256、调用原生安装程序并验证结果。
README 是分享和说明入口；支持 skill 自动发现的工具可以将 `installer-skill/rum-sdlc-install/` 放入自己的 skill 目录。
也可以直接让 agent 读取这个文件完成安装，无需先在电脑上安装该 skill。

## 手动下载和运行

[最新正式版](https://github.com/iLoner121/rum-sdlc/releases/latest) · [0.9.0 发布页](https://github.com/iLoner121/rum-sdlc/releases/tag/v0.9.0)

| 平台 | 0.9.0 下载 |
| --- | --- |
| macOS Apple Silicon | [darwin-arm64.tar.gz](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-darwin-arm64.tar.gz) |
| macOS Intel | [darwin-amd64.tar.gz](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-darwin-amd64.tar.gz) |
| Linux x86-64 | [linux-amd64.tar.gz](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-linux-amd64.tar.gz) |
| Linux ARM64 | [linux-arm64.tar.gz](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-linux-arm64.tar.gz) |
| Windows x86-64 | [windows-amd64.zip](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-windows-amd64.zip) |
| Windows ARM64 | [windows-arm64.zip](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/rumsdlc-0.9.0-windows-arm64.zip) |

下载同一发布下的 [checksums.json](https://github.com/iLoner121/rum-sdlc/releases/download/v0.9.0/checksums.json)，
用系统的 SHA-256 工具核对压缩包与 `files` 中同名文件的值，一致后解压。
macOS 可用 `shasum -a 256 文件名`，Linux 可用 `sha256sum 文件名`，Windows PowerShell 可用 `Get-FileHash 文件名 -Algorithm SHA256`。

解压后只有 `rum`（Windows 为 `rum.exe`），放到长期保留的位置即可使用：

```sh
./rum --version
./rum --help
git --version
```

Windows PowerShell 使用 `./rum.exe`。不需要配置 PATH 或启动服务。
直接使用模式下，工程会记录本体的位置，因此创建工程后不要随意移动或删除这个程序。

### 可选：安装固定入口

从解压目录运行下载的原始程序：

```sh
./rum install
```

Windows PowerShell 使用 `./rum.exe install`。默认路径如下，`~` 表示当前用户主目录：

| 用途 | 默认位置 |
| --- | --- |
| 版本目录 | `~/.local/share/rumsdlc/0.9.0/` |
| 固定命令 | `~/.local/bin/rum`，Windows 为 `rum.exe` |
| 设备配置与稳定转发入口 | `~/.rumsdlc/` |

安装会返回实际命令路径及 `path_ready`。即使命令目录不在 PATH 中，也能用返回的完整路径运行，
无需修改环境变量。需要自定义位置时使用命令前的 `--home` 和安装参数：

```sh
./rum --home /your/data/rumsdlc install --destination /your/apps/rumsdlc/0.9.0 --bin-dir /your/bin
```

升级时从下载的新程序运行安装，使用新的版本目录。已有工程保持原版本；
仅在需要更新某个工程时，执行 `project-update --dry-run`，再用返回的令牌显式应用。
安装器遇到运行中的命令或服务会拒绝切换；旧版升级、停止服务和事务恢复见[使用手册](internal/assets/docs/usage.md)。

## 创建或接管工程

以下示例使用完整命令路径；名称、日期和目录按实际需要填写：

```sh
~/.local/bin/rum workspace-init --parent /your/projects --name demo --date 2026-11-06
~/.local/bin/rum list
```

已有工程使用 `workspace-adopt --path DIR --date DATE`；远端管理工程使用
`workspace-import --repository OWNER/RumSDLC_ID --parent DIR`。
创建、接管或导入后，从该工程根目录启动 agent，让它读取本地 `AGENTS.md` 和
`.agents/skills/rum-project/SKILL.md`。工程中的 Python 脚本需要原有 Python 及相关工具环境；
这与 RumSDLC 本体的原生运行依赖分开。

本地使用不要求 GitHub 登录。`rum explain` 可取得内置离线手册，`rum COMMAND --help` 查看具体参数。
同步、代码关联和升级流程见[使用手册](internal/assets/docs/usage.md)。

## Topic 编号

Topic 的用户编号统一为 `T1`、`T2`，项目内按创建历史连续分配，不补零、不复用。查询与创建输出增加 `short_id`；命令、文档和代码关联可用短编号，原长 ID 和文档路径继续有效。新开发分支默认建议 `feat/T42-export-report`，显式名称与已有映射优先。两设备离线创建产生分叉时，先显式恢复冲突、确认分配顺序；详见[使用手册](internal/assets/docs/usage.md#topic-编号与开发分支)。

## 代码结构

| 目录 | 职责 |
| --- | --- |
| `internal/core` | 27 项业务操作、校验、历史重放、查询、Markdown 和旧快照转换；不做文件、进程或网络 I/O |
| `internal/store` | 状态与文档持久化、预览令牌、并发写保护、兼容的目录锁与恢复 |
| `internal/app` | 设备登记、工程生命周期、资源投放、代码关联、Git 同步与冲突、身份和远端观察 |
| `internal/service` | 可选本地服务；与单次 CLI 共用 Device.Call，不复制业务逻辑 |
| `internal/cli`、`internal/catalog` | 命令解析、输入路径转换、输出和可查询的命令目录 |
| `internal/installation`、`internal/migration` | 独立的安装事务与旧快照落盘流程 |
| `internal/process`、`internal/fsutil` | 有界可取消进程、环境隔离、跨平台文件锁、安装门禁与路径检查 |
| `internal/jsonvalue` | 严格 JSON、原版兼容的令牌计算与数字语义 |
| `internal/assets` | 嵌入的原项目资源、少量 Go 调用桥接脚本和离线手册 |
| `cmd/rum` | 完整系统入口；cmd/workflow、cmd/migrate 可独立构建 |

没有引入第三方 Go 模块、插件框架或通用仓储抽象。应用层可由后续 GUI 直接调用。
版本化 JSON 数据保留扩展字段，由边界校验保护；业务核心与具体入口分离。
项目桥接脚本仅传递请求、锁和结果，不包含另一套 Python 状态机。

## 构建与验证

构建机器需要 Go 1.23 或更新版本；运行机器不需要 Go。

```sh
go test -race ./...
go vet ./...
go build -trimpath -o rum ./cmd/rum
go run ./cmd/build
```

跨平台构建使用 CGO_ENABLED=0，不要求目标机器安装语言运行环境或额外共享库。
GitHub Actions 的 macOS/Linux/Windows 原生 Go 竞态测试、静态检查、构建和启动检查，
以及六种交叉构建均已通过，见[跨平台测试记录](https://github.com/iLoner121/rum-sdlc/actions/runs/42195535922)。

0.9.0 的完整原生回归 281/281 通过，无跳过；竞态、静态、两条旧版升级路径和
无运行时环境的发布程序测试通过。历史核心差分 195/195、迁移专项 10/10 的代码与已验收版本一致。
准确范围、测试适配方式与日志见 [验收说明](ACCEPTANCE.md)。

平台证据须区分：本机完整执行环境为 macOS arm64；macOS amd64 产物通过已有
Rosetta 环境下的安装、直接使用和项目脚本验证。Linux/Windows 已在 CI 中执行原生 Go 测试，
这不等于原有 281 项 Python 驱动回归已在这两个系统完整运行。项目 Python 脚本的原有
POSIX 依赖保留，符合本次仅重构系统层的范围。

## 管理仓库名称

`rum repository-plan --project ID` 提供可编辑的名称建议；优先使用关联代码仓库名，再使用项目显示名。
`--name 自定义后缀` 可预览编辑后的名称，创建时用 `rum enable --project ID --repository-name 自定义后缀`。
管理仓库保留 `RumSDLC_` 前缀，名称占用会提示选择，原 `--repository owner/name` 继续选择既有仓库。
发现结果分别展示仓库名、项目显示名和身份文件状态；旧长名可继续使用。详见[使用手册](internal/assets/docs/usage.md#管理仓库命名与发现)。

管理仓库的自动提交消息统一为 `时间戳 | user="姓名 <邮箱>" | device="设备名"`。
时间采用带时区的 RFC3339 格式，与 Git 提交日期一致；不再附带检查点、合并或项目编号等业务描述。
代码仓库的提交规范独立，不受此格式影响。
