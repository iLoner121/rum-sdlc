# 项目内 Topic 短编号与身份兼容

- 状态：accepted；实现已完成，待代码提交与整合。
- 日期：2026-11-10。
- 决定依据：用户确认采用 T1，并要求补完文档和实施（Topic T-20261109-4ea134ab / T4）。

## 背景与选项

日常界面、命令和开发分支需要简短编号；系统已有长 ID 贯穿历史、正文路径和 Git 显式关联。比较 T1 与 T-1 后选择 T1，以便输入、口头交流及分支阅读。

## 决定与适用范围

对外统一 T1、T2，不补零、不复用。核心从受管追加历史首次创建 Topic 的顺序派生 short_id，保留 canonical id，不重写身份或历史。现有 Topic 自动获得投影，无需转换数据；所有 Topic 引用入口可解析短编号，数据存储继续使用长 ID。新分支建议 type/Tn-slug；显式名称、已有映射和历史提交保留。

## 实现证据

核心为 internal/core/numbering.go；存储、应用及项目 Git 适配器解析短编号并保留长身份。回归为 internal/core/numbering_test.go、internal/store/numbering_test.go、internal/app/numbering_test.go 与 tests/topic_numbering.py。2026-11-10 全量 Go 竞态、静态、新工程入口和 Git 适配器验证通过；macOS 实际运行、Linux/Windows amd64 交叉构建通过。详细验收证据保存在独立工作流 Topic T4 的 acceptance.md；尚未安装/发布。

## 并发与代价

同 Store 使用锁和预检令牌保护分配；两个离线设备只共享共同前缀，未共享后缀编号属于各自本地分配，可能冲突。系统不自动合并分配分叉，须保留双方 canonical ID、共同历史和候选，并按明确顺序通过 API 重放后缀后确认编号；共同前缀编号保持。不能宣称离线无协调连续全局分号。

选择投影方案可兼容全部既有状态格式和路径，代价是依赖完整追加历史，以及离线分叉恢复需要显式业务顺序。后续如压缩历史，必须先设计可保留编号的迁移契约。
