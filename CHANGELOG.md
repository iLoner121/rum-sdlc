# 版本记录

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
