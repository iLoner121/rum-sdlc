---
name: rum-sdlc-install
description: 从官方发布包或用户指定来源安装、更新 RumSDLC 原生本体；不负责工程资源升级、账号配置或开发流程。
---

# 安装 RumSDLC

RumSDLC 从 0.9.0 起发布 Go 原生程序，系统不依赖 Python/Go；仓库操作需要用户自行安装 Git。
本 skill 只取得发布程序并调用它的安装器。项目 agent/skill 由本体在创建、接管、导入或显式更新工程时投放，保留 Python 及原有环境依赖。

## 来源和平台

- 默认来源：https://github.com/iLoner121/rum-sdlc 。用户指定版本、镜像或本地发布包时沿用其选择。
- 未指定版本时查询 https://api.github.com/repos/iLoner121/rum-sdlc/releases/latest ，选择正式发布；读取该版本的 README 和发布说明。不要把仓库 main 或源码 ZIP 当作已构建程序。
- 平台名为 `darwin`、`linux`、`windows`，架构名为 `amd64`（x86-64）或 `arm64`。按目标系统和 CPU 选包；macOS Rosetta 会影响进程报告的架构，应核对实际硬件。
- 原生包命名为 `rumsdlc-VERSION-OS-ARCH.tar.gz`，Windows 为 `.zip`。下载同一发布的 `checksums.json`，核对其版本和 `files` 中该包的 SHA-256，匹配后解压。校验不匹配、包缺失或平台不受支持时停止并报告，不能跳过校验或擅自安装编译环境。
- 用可用的 API、下载和文件工具完成；公共发布下载无需 GitHub 登录。不要为安装本体要求 Python、Go、gh 或 jq。检查 `git --version`；缺失时告知用户需要自行安装 Git，本体安装仍可完成。

## 调用本体安装器

1. 检查解压的 `rum --version`（Windows 为 `rum.exe`），必须符合所选发布。读取 `install --help`；已有部署沿用用户的设备 home 和命令目录，新安装可采用默认路径。
2. 从刚解压的原始可执行文件调用 `rum [--home HOME] install [--destination VERSION_DIR] [--bin-dir BIN_DIR]`。全局 `--home` 放在 `install` 前；路径传绝对路径。默认版本目录为 `~/.local/share/rumsdlc/VERSION`，命令目录为 `~/.local/bin`，设备目录为 `~/.rumsdlc`。不要从已经安装的转发入口覆盖它自身，不另写迁移或切换脚本。
3. 用安装返回的 `command` 完整路径验证 `--version` 与 `--help`。报告版本、程序目录、设备目录、命令路径、`path_ready` 和 Git 检查结果。命令目录不在 PATH 时给出可直接运行的完整路径；无需为了使用程序修改 shell 配置。

若用户要求直接运行而不安装，将校验后的程序放在长期保留的位置，验证版本与帮助即可；工程会引用这个位置，不能放在随后会清理的临时目录。

## 升级和恢复边界

- 普通安装只激活本体和固定入口，不读取或改写已有工程与登记，不登录、不启用同步、不启动服务、不删除旧版本。项目级 Python 环境保持原样，不因系统安装而安装它。
- 发现运行中的命令、服务或项目脚本时保留现场。已有授权包含停服时可正常执行 `rum daemon stop`，等待退出后重试；不强杀。首次从 0.8.0 之前的版本升级须确认旧入口与脚本均已退出，才能传 `--confirm-legacy-idle`，不可用它绕过错误。
- 安装失败若报告恢复未完成，使用下载程序对同一 home 执行 `install --recover-only`。旧事务恢复可能还原工程或重启原服务，应先读取该版本使用手册并确认符合用户的恢复请求；不删除事务记录、不手改入口、不覆盖外部修改或归属不明的锁。权限、磁盘或冲突持续阻塞时停止并报告具体原因。
- 工程规则更新是独立请求，按该版本手册执行 `project-update --dry-run` 和令牌提交；安装成功不代表工程已经升级或其 Python 环境就绪。
