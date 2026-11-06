# 0.9.0 验收说明

基线为本地不可变的 rum-sdlc-device-v0.8.5，核心协议为 4.1.2。
系统运行依赖边界：不需要 Python/Go；仓库操作需要 Git。
项目 Python 脚本保留原依赖。没有修改原版源代码或磁盘上的原测试文件。

## 结果与范围

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 原版基线 | 281 项通过 | evidence/baseline-281.log |
| 0.9.0 原生系统入口回归 | 281/281 通过，无跳过，531.420 秒；当前版本期望为 0.9.0，业务断言与历史夹具保留 | evidence/v0.9.0-regression.log |
| 历史核心 | 195 项通过；2,180 次操作、4,292 次校验、40 次查询差分一致 | evidence/rumsdlc-native-final-core-195.log |
| 历史快照迁移 | 10 项通过，18 次转换的状态、报告与文件字节一致 | evidence/rumsdlc-native-final-migration.log |
| 原生 Go 测试 | go test -race -count=1 ./... 通过 | evidence/v0.9.0-race.log |
| 静态检查 | go vet ./... 通过 | evidence/v0.9.0-vet.log |
| 原生核心 CLI | 20 组命令、5 种视图、过期令牌拒绝，结果与持久化快照一致；PATH 为空 | evidence/rumsdlc-native-final-cli.log |
| 发布程序安装 | 安装、全局入口、升级、门禁、旧事务恢复与原生服务重启通过；PATH 为空 | evidence/v0.9.0-install.log |
| 发布程序直接使用 | 接管现有仓库、真实 Git 配置、健康检查、工作流与离线手册通过；PATH 仅含 Git | evidence/v0.9.0-portable.log |
| 原项目脚本协作 | 未修改的 git_adapter 调用 Go 工作流、双令牌提交与过期拒绝通过 | evidence/v0.9.0-project-adapter.log |
| 0.8.5 → 0.9.0 | Python 与旧 Go 两条实际升级路径通过；旧发布与工程保持原字节，显式更新后项目规则为 0.9.0 | evidence/v0.9.0-upgrade.log |
| macOS amd64 | 0.9.0 经已有 Rosetta 验证安装、直接使用与项目脚本 | evidence/v0.9.0-intel.log |
| 分发构建 | macOS/Linux/Windows × amd64/arm64，六种均成功 | evidence/v0.9.0-build.log、build/native/0.9.0/checksums.json |
| 资源保护与打包 | 68 个基线文件、32 个项目资源哈希一致，6 份压缩包内容与二进制一致 | evidence/final-preservation.json |

历史核心、迁移专项与核心 CLI 记录沿用上一次验收：本次相关 4.1.2 源文件没有改变，
哈希核对见 evidence/unchanged-core.json；0.9.0 的完整 Go 测试已重新执行。

Go 测试还覆盖文件/注册表并发写、文档 CAS、符号链接、历史篡改、工作区接管恢复、
真实双设备 Git 同步、冲突保留、用户编辑保护、服务取消和子孙进程终止。
原版中依赖 Python gh 实现细节的凭据与 review 校验单元测试保留为参考；
对应 Go 版本使用真实子进程和 HTTP 请求边界独立验证，见 internal/app/github_test.go。
Go 版 CLI 帮助和错误呈现采用原生入口格式；Markdown 内对象使用稳定键排序。
业务状态、JSON 查询内容、持久化快照与并发令牌按原协议验证，不要求帮助文本逐字相同。

## 跨语言测试如何适配

tests/installed_original.py 运行原来的 281 个用例，保留业务断言，作以下映射：

- install.py 与安装目录的 rum.py 改为原生 rum install 和 rum；历史版本仍作为升级夹具运行原 Python 程序。
- Device.call 通过测试专用 JSON 传输执行 Go 应用；FakeHub 保留为远端测试数据，真实 Git 操作由 Go 执行。
- 原 Python monkeypatch 的故障点映射到 Go I/O 边界，可继续、报错或直接杀死原生进程；生产构建不包含这些可控制的故障入口。
- 原 Python 核心文件完整性检查改为实际原生程序、MANIFEST 和项目桥接依赖检查。并发编辑用例准备旧版托管 context，保证稳定入口设计下仍会触发实际写入。
- 本地服务与跨目录导入使用原生服务。慢网络取消测试保留原并发、时限、退出与数据断言，将 Python 睡眠替换为原生服务启动的真实可取消子进程。
- 参数预处理测试保留原参数集合与断言，捕获原生 CLI 发给服务的请求。
- 版本升级仅将原测试中的当前版本期望从 0.8.5 调整为 0.9.0，并扩展 Python 测试夹具读取器的版本白名单；不改写原测试文件或历史版本。
- 合并验收同时启用核心差分传输；历史核心中的两次旧版本初始化仍用 Python 构造夹具，后续操作和验证进入 Go。

项目 Python 脚本、测试数据构造器、远端模拟和历史升级夹具使用 Python，不代表
发布程序依赖 Python。迁移测试中的原 Python 落盘故障测试作为参考保留；Go 的
真实落盘、源快照变更和仅清理自身文件的失败恢复由 internal/migration 测试覆盖。

## 平台边界

0.9.0 完整回归在 macOS arm64 执行。六种发布程序均为关闭 CGO 的原生编译产物。
0.9.0 的 macOS amd64 安装、直接使用与项目脚本协作已在已有 Rosetta 环境通过；
这不是 Intel 实机的完整回归。
Linux/Windows 无本机执行证据，不能据此声称这些系统上的完整测试已经通过。
.github/workflows/native.yml 提供目标系统 CI，尚未提交或触发远端运行。

原版只面向 POSIX 的项目 Python 脚本保留其依赖，不承诺在原生 Windows Python
中直接执行。这不影响独立 Go 系统的跨平台编译边界。

## 复现

```sh
go build -tags contracttest -o app-contract ./cmd/app-contract
go build -tags contracttest -o rum-under-test ./cmd/rum
go build -o core-contract ./cmd/core-contract
go build -o migration-contract ./cmd/migration-contract
go build -o workflow ./cmd/workflow
```

为 tests/installed_original.py 指定 RUM_BASELINE、RUM_APP_CONTRACT、
RUM_NATIVE_BINARY 和 RUM_CORE_BINARY 的绝对路径，并设置
RUM_PATTERN=test_*.py、PYTHONDONTWRITEBYTECODE=1。原始测试依赖同级历史发布夹具，
目录布局须保留。仅测试指定模块时可改变 RUM_PATTERN。

历史核心测试为 tests/core_differential.py，额外指定 RUM_TESTS；
迁移差分为 tests/migration_differential.py，指定 RUM_CORE_TESTS 和
RUM_MIGRATION_CONTRACT。全部 Python 驱动仅用于开发验证，包含在源码包中，不进入原生程序压缩包。

evidence 中保留本表引用的最终验收记录；历史基线及旧发布夹具需另外提供。
