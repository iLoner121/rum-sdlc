# 版本记录

## [1.3.0](https://github.com/iLoner121/rum-sdlc/compare/v1.2.0...v1.3.0) (2026-11-28)


### Features

* **workflow:** Add Topic priority levels ([21df5ad](https://github.com/iLoner121/rum-sdlc/commit/21df5ad1145e602521eaa5c4790182f6c6b9259e))
* **workspace:** Show parallel worktree recovery and archive context ([d3cb7b4](https://github.com/iLoner121/rum-sdlc/commit/d3cb7b4ca17a473904074dd501d5e8fcea24f928))

## [1.2.0](https://github.com/iLoner121/rum-sdlc/compare/v1.1.1...v1.2.0) (2026-11-27)


### Features

* **workspace:** Add device AGENTS.md templates ([83d2417](https://github.com/iLoner121/rum-sdlc/commit/83d2417484ce81729802d6eca652fae7ab9ba229))

## [1.1.1](https://github.com/iLoner121/rum-sdlc/compare/v1.1.0...v1.1.1) (2026-11-23)


### Bug Fixes

* **workspace:** Decouple project compatibility from releases ([061e862](https://github.com/iLoner121/rum-sdlc/commit/061e8625b0b8ff02765be80568fb92ba45ee5020))

## [1.1.0](https://github.com/iLoner121/rum-sdlc/compare/v1.0.0...v1.1.0) (2026-11-18)


### Features

* **ui:** Add browser shell and project overview [T7] ([8841ad7](https://github.com/iLoner121/rum-sdlc/commit/8841ad793f9458c6ee30cee21c8f609004739fdc))
* **ui:** Browse filtered and historical Topics [T9] ([1e7cf01](https://github.com/iLoner121/rum-sdlc/commit/1e7cf01b0c7ff4f53fd49a0bca605059c607009f))
* **ui:** Browse iteration scope and frozen results [T10] ([6a2bdad](https://github.com/iLoner121/rum-sdlc/commit/6a2bdadfb358c617e5305198a02df4c6a399f3c0))
* **ui:** Browse milestone goals and scope [T8] ([5a33353](https://github.com/iLoner121/rum-sdlc/commit/5a333534deb5c9197eb072e7a34779895013a2f9))
* **ui:** Browse the ordered candidate pool [T11] ([5e54c2f](https://github.com/iLoner121/rum-sdlc/commit/5e54c2f12e1a44c029f05e888f02b64ddffe0f13))
* **ui:** Discover cloud projects without persistence [T13] ([f8f0851](https://github.com/iLoner121/rum-sdlc/commit/f8f08518286be6bb7a7e7c54a7cab2928c93b5f6))
* **ui:** Read Topic documents and activity evidence [T12] ([28f3dbf](https://github.com/iLoner121/rum-sdlc/commit/28f3dbf69ffc8f5e30b6c3bb412f818088257a9c))
* **ui:** Serve read-only local project views [T6] ([415be12](https://github.com/iLoner121/rum-sdlc/commit/415be1246a29263a5a63e618695840be07dcf2c1))


### Bug Fixes

* **ui:** Anchor task status in the summary header [T12] ([8d8ffe7](https://github.com/iLoner121/rum-sdlc/commit/8d8ffe74fc2ed2c7ebaa52067635d3fdd73b7922))
* **ui:** Keep mobile scrim out of desktop grid [T7] ([92e1570](https://github.com/iLoner121/rum-sdlc/commit/92e1570460b3ffffcb44061bbf95da7c4b546b2d))
* **ui:** Preserve cached Topics when refresh fails [T9] ([d8ffd4a](https://github.com/iLoner121/rum-sdlc/commit/d8ffd4ac0027d58905b5ba5b076633809ed404e0))
* **ui:** Reject rooted document paths on every platform [T6] ([a19b6dc](https://github.com/iLoner121/rum-sdlc/commit/a19b6dcc66383dcc143ab89a8b5c658208daffaa))
* **ui:** Reorder navigation and unify vivid status colors [T7] ([1ba1b2e](https://github.com/iLoner121/rum-sdlc/commit/1ba1b2ea50806c1d9023182db488cf891c875b36))
* **update:** Recheck due time from a consistent snapshot [T18] ([647d3b7](https://github.com/iLoner121/rum-sdlc/commit/647d3b70bc614367ece0219830ba49ba51fbaec8))
* **update:** Reject stale scheduled checks after disabling [T18] ([da4e109](https://github.com/iLoner121/rum-sdlc/commit/da4e1097a081a3afb12c6060d76d2dca92625e73))

## [1.0.0](https://github.com/iLoner121/rum-sdlc/compare/v0.9.0...v1.0.0) (2026-11-11)


### ⚠ BREAKING CHANGES

* **app:** 身份文件默认写入 name 字段，原严格匹配三字段的客户端需升级；新建仓库默认采用可读名称，已有仓库按身份继续恢复。

### Features

* **app:** Add repository names and identity discovery [T3] ([5d3c8ca](https://github.com/iLoner121/rum-sdlc/commit/5d3c8cabb1501624101135bbd328fa29b7a46b9c))
* **core:** Add stable Topic short numbers [T2] ([ef52ccb](https://github.com/iLoner121/rum-sdlc/commit/ef52ccb2e388cb0ebb8949c17cfd20a2414bcc2d))
* **sync:** Standardize metadata commit messages [T4] ([b8cf930](https://github.com/iLoner121/rum-sdlc/commit/b8cf93011ef81241f5fa4c25c2f7908cffed0430))
* **update:** Add release PRs and native self-updates [T18] ([ce95d89](https://github.com/iLoner121/rum-sdlc/commit/ce95d89417ff66e7ac55a19092c672ff4671e8a1))


### Bug Fixes

* **update:** Recover abandoned update preparations [T18] ([3d642c4](https://github.com/iLoner121/rum-sdlc/commit/3d642c4eeb18ed34ddf40ffc737392fc9a3f7dac))
* **update:** Serialize short update state writes [T18] ([1fa16fa](https://github.com/iLoner121/rum-sdlc/commit/1fa16fa940d363ece57377f78af48e15f5332062))

## 0.9.0 — 2026-11-06

从本版本开始，RumSDLC 系统层基于 Go 开发和发布。此前的 0.8.x Python 发布保留为历史版本，
原有标记为 0.8.5-go 的重构产物属于过渡构建；正式 Go 版本从 0.9.0 开始。

- 程序版本、安装清单、服务信息、新建及显式更新后的项目规则统一为 0.9.0。
- 单一发布版本定义驱动程序、构建目录和包名，后续版本不覆盖旧发布目录。
- 系统直接运行，无需 Python/Go 环境；仓库操作由用户已有 Git 支持。
- 正确识别 Windows 的服务连接拒绝错误，未启动后台服务时 CLI 可直接执行本地操作。
- 项目级 agent/skill 保留 Python 与原有依赖。
- README 提供仓库分享、原生包下载与安装入口；独立薄安装 skill 调用本体安装器。
- 保持工作流核心 4.1.2 的业务状态、数据格式与令牌语义，不做无关协议升级。
- 保留 0.8.x 及更早受支持工程的兼容读取；安装仅切换程序，项目规则按原流程显式更新。

升级时将 0.9.0 安装到新的版本目录。旧工程经 doctor 检查后，使用
project-update --dry-run 获取预览，再用返回的 revision 显式应用。
原 Python 发布目录和旧版 Go 构建均保留。
