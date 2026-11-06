# 代码 Git 与工作流的集成

本层处理代码现场、分支策略和真实 Git 证据，不能决定系统数据的特殊写权限。本页脚本流程适用于覆盖完整的仓库；永久不受支持的结构按 [git-fallback.md](git-fallback.md) 补齐明确证据，再使用正常系统预检/写入路径，不机械重复拒绝的脚本。所有工作流记录在唯一独立 Store，代码工作树不复制 workflow/、Task 或全局规划。

git_adapter.py 的 current/bind/unbind/list/check/add/adopt/remove/delivery/line-delivery/retain-check/retarget-plan/retarget-verify/integration-apply/line-retention/line-configure/line-state-plan/closure-plan 均需 --repo。current 可从已有本地绑定读取工作流路径，否则显式 --store。本地绑定保存在设备 agent-state/git 目录，按 Git 公共目录和工作树身份分开索引。--state-home 可显式指定仓库外目录；生成的项目脚本默认使用工程的设备目录。本体不读取这些文件，不跨客户端同步。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py current --repo <代码树> --store <工程>/meta
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py bind --repo <代码树> --store <工程>/meta --topic <T-ID> --dry-run
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py bind --repo <代码树> --store <工程>/meta --topic <T-ID> --expect-revision <返回令牌>
```

未绑定时明确返回规划/整合模式和 binding_required，不按 Topic ID 的分支命名猜测。agent 根据实际任务核对后显式 bind；不重命名现有分支。新建 Topic 分支优先沿用代码项目约定；省略 --branch 时仅在确认首次代码且没有已记录分支时建议 <type>/<Topic-ID>-<slug>，如 feat/T-20261105-aabbccdd-cache-timeout。feat/fix/docs/refactor/chore 保留常规含义，spike 的默认前缀为 chore。显式 --branch 允许用户自定义、带其他 Topic 编号或完全不带编号；只检查合法 Git 名称，不以名称推导归属。已登记分支优先恢复；建议名称已存在时拒绝自动接管，先核对后显式选择。开始会话从系统重读当前 Topic/关系，不依赖旧本地记录。

同一代码树/Git 现场保持一个写入者；不同 Topic 真并行、保留脏现场或紧急插入才增加工作树，普通串行复用。暂停保留代码目录与恢复说明；不愿提交保留脏树，不自动 stash。工作流暂停/切换不保存代码，也不切分支。跨设备代码写入仍需明确交接，绑定不是跨设备独占锁。

add/adopt/remove/bind/unbind 先 dry-run，再使用返回令牌执行同一请求。add 的 --store/--path 必填，选择 --topic、--line 或明确 --branch。新的 Topic 分支同时提供 --first-code 和 --code-history-check，--branch 用于覆盖默认建议；这些文字必须对应实际核对，不是放行开关。

### 创建与恢复的唯一规则

| 主体 | 创建新分支 | 恢复已有分支 |
|---|---|---|
| Topic | 默认从关联 Line 的当前分支 tip 开始；显式 --base-ref 只允许该 tip 或已声明依赖 Topic 的当前实际 tip | 保留该分支原 HEAD，禁止 --base-ref；不因 Topic 状态重建代码 |
| Line | 使用 extensions.git 登记的固定分支；默认从配置的 base_commit 开始，也可显式 --base-ref 指定实际起点 | 保留现有 HEAD，禁止 --base-ref |
| 无 Topic 规划树 | 必须 --branch 和 --base-ref，允许明确指定实际起点；不适用 Topic 依赖限制 | 保留现有 HEAD，禁止 --base-ref |

Topic 业务活动和代码活动独立。创建任何新的 Topic 代码分支前，均需同时 --first-code --code-history-check <实际核对记录>。核对本地索引、工作树、已有分支和远端/其他客户端交接；参数是已有事实说明，不是伪造无历史的开关。接入记录或历史 Git context 已有代码时，即使 planned 或带 first-code 也拒绝重建，应恢复原代码后 add/adopt；存在原分支时不使用 first-code。

适配器在设备 agent-state/git 下的仓库索引留存观察到的 Topic 实际引用和首次观察提交，移除工作树、解绑不删除此记录。它只帮助代码定位与防误重建，不是平台关系或跨设备事实源；新客户端缺少索引时必须核对共享代码和交接，不能宣称自动证明“从未有代码”。保留该接入记录，不手改清空以绕过保护。

实际 Topic 引用统一按：显式指定 → 本仓库已登记／存活绑定的唯一引用 → 无记录时由 agent 显式指定实际引用。多个候选必须显式选择，已记录却消失的分支不能退回默认名猜测。下游 add 可重复 --dependency-ref <T-ID>=refs/heads/<实际分支>；只接受声明过的依赖，预检展示实际 ref 和 tip，执行重读，变化使令牌失效。移除上游工作树后仍可通过本仓库索引定位；跨客户端使用明确映射或重新绑定。delivery/retarget 使用同一解析器，可 --source-ref。

宿主已有工作树用 adopt --path；必须精确匹配同仓库已登记根目录，拒绝普通子目录和 locked/prunable。创建不复制凭据、不安装环境、不改变业务状态。部分成功时保留目录／分支，用 adopt 修复绑定，禁止重置重试。

创建前检查路径、基线、已检出分支；创建后核对 HEAD/分支；只有 Topic 工作树写 Topic 本地绑定，无 Topic 不虚构 Topic。实际运行前核对依赖环境、端口、数据库、缓存和进程隔离。同一可变资源不能由两个写入者同时使用；无法隔离时串行并记录。

remove 参数 --path --expect-head <完整SHA> --resources-check <真实核对>，可选 --retain-ref。不能移除调用/主工作树或 locked/prunable 树；拒绝修改/未跟踪/忽略文件，必要忽略内容先保存到目录外。HEAD 必须由真实本地分支/tag 包含，detached 必须显式 retain-ref；移除保留分支与独立工作流。进程/数据资源由 agent 核实，参数不认证事实。宿主管理目录沿用宿主生命周期接口。

Git 操作与本地绑定不是跨系统事务；add 创建成功而绑定失败时输出 changed=null，保留目录与分支，检查后 adopt。不能盲目重跑 add 或删除部分成果。预检不是冻结外部 Git 的锁，执行期间仍要求单写入者。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py delivery --repo <代码树> --store <目录> --topic <T-ID> --commit <整合SHA>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py line-delivery --repo <代码树> --store <目录> --line <ID> --commit <整合SHA> --expect-tip <线当前SHA>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py retain-check --repo <代码树> --ref refs/heads/keep --contains <待保留SHA>
```

delivery 使用上述实际引用解析规则，可 --source-ref 显式指定实际源；源不包含在整合历史中时，只有已按 closure.md 获批的 squash 使用 --squash-evidence 记录具体证据。该参数不是自动证明。Line 整体整合不提供 squash 绕过，核对实际 source tip、整合 SHA 与主线祖先关系。retain-check 必须真实 branch/tag，不能用裸 SHA 作为唯一保留依据。

实际 commit/push/rebase/清理仍按 git-safety.md 的授权、准确结果 review 和证据办理；本版不自动创建产品提交、推送或同步工作流库。


## 为业务 Line 接入 Git

先使用系统 project.init / line.create 创建业务实体，再配置 Git。命令生成 request/platform_preview；核对后将 request 对象保存为 JSON，integration-apply --request <文件> --expect-revision <platform_preview.revision> --expect-code-revision <code_revision> 应用。没有代码仓库的项目无需此步骤。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py line-configure --repo <代码树> --store <目录> --line mainline --branch main --date YYYY-MM-DD --reason <原因> --evidence <实际仓库信息> --decision <已有决定>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py line-configure --repo <代码树> --store <目录> --line engine --branch line/engine --base-ref main --date YYYY-MM-DD --reason <原因> --evidence <实际起点> --decision <已有决定>
```

Line 分支名不重复；已配置分支身份保持稳定，不能靠重新配置跳过代码迁移。非主线需实际 base-ref，配置保存解析出的完整提交；平台仅保存扩展对象。

## Git 代码现场能力

add/list/check 的观察、策略与效果全部属于 Git 接入层；git_workspaces 提供可复用的预检与健康结果，核心系统不依赖它。list --repo <代码树> 可选 --store <目录>，返回可用性、绑定、包含忽略文件的 dirty 状态和 issues；check 使用相同检查，有错误退出 1。提供 Store 时同时校验平台快照和绑定 Project/Topic；dirty 作为需保留的现场状态报告，不自动清理，也不把正常开发中的脏树一律视为结构错误。只读结果不是跨设备独占锁。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py add --repo <代码树> --store <目录> --branch planning --base-ref main --path <新目录> --dry-run
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py add --repo <代码树> --store <目录> --line <Line-ID> --path <新目录> --dry-run
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py check --repo <代码树> --store <目录>
```

## 更换 Topic 整合目标

先按已有授权核对范围、依赖与契约影响；执行中的 Topic/Task 先暂停或阻塞。保存原 Topic HEAD，保留脏现场；实际 merge/rebase 仍按 git-safety 的具体授权办理，迁移工具不替你改代码。准备后 HEAD 必须包含新目标真实 tip，原 HEAD 必须仍可达，或由真实 branch/tag 保留。不能只给一个裸 SHA 当永久保留。

1. 在实际 Topic 代码树运行 retarget-plan，提供目标、日期、理由、证据、已有决定、依赖/契约评估；目标与当前 Line 相同且已有上下文时，生成独立的 topic.integration.prepare，不伪造关系移动；已有代码另提供 --previous-head <原完整SHA>，必要时 --retained-ref refs/tags/keep。实际尚未建立代码的 Topic 可先迁移，工具保留明确的 null 源 HEAD，并登记真实目标 baseline。
2. 检查返回的 request 和 platform_preview。将 request 对象本身保存为 JSON（不能把整个返回包当请求），使用 integration-apply --request <文件> --expect-revision <platform_preview.revision> --expect-code-revision <code_revision> 应用；会重新观察代码并走相同 Store.execute 接口，目标 tip/HEAD/请求或平台变化均拒绝旧预检。适配器没有特殊系统写权限。
3. 恢复 Topic active，按新目标复核受影响设计、契约和测试。需要实施代码且尚未建分支时，按 add --first-code --code-history-check <实际核对> 首次建立代码分支；planned Topic 也必须提供同样的首次代码核对参数。纯调查/报告交付不创建空分支：retarget-verify 增加 --no-code <实际代码历史核对记录>，proof.verification.git 保存 no_code，原/准备 HEAD 必须仍为 null。完成代码活动时，正常生成含 verified_head 的 proof。两种路径的 --report 都引用实际复核，不能空填形式说明。
4. 仍用 integration-apply 应用该 review 请求；执行前再次核对目标 tip，以及代码路径的 HEAD/干净现场或无代码路径的本地记录。生成和应用 review 时，目标 tip 都必须等于所选 baseline。目标已变化则暂停活动/Topic，对同一 Line 重新 retarget-plan 并应用，更新评估后恢复 active、复核受影响结论，再生成 review；不重新领取 Topic、不伪造 Line 迁移。未受影响的既有验证可按 verification.md 复用。后续 ready 仍需要实际用户接受，不继承旧目标上的接受。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py retarget-plan --repo <Topic代码树> --store <目录> --topic <T-ID> --to-line <Line-ID> --date YYYY-MM-DD --reason <迁移原因> --evidence <影响证据> --decision <已有决定> --dependencies <依赖评估> --contracts <契约评估> --previous-head <原SHA>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py integration-apply --repo <Topic代码树> --store <目录> --request <request对象JSON> --expect-revision <平台预检令牌> --expect-code-revision <现场预检令牌>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py retarget-verify --repo <Topic代码树> --store <目录> --topic <T-ID> --date YYYY-MM-DD --reason <复核原因> --evidence <实际结果> --decision <已有授权> --report <新基线验证报告>
```

实际源分支无本适配器记录时，显式 --source-ref refs/heads/<实际分支>，生成和应用使用一致的参数；已有本地绑定也可定位。工具检查真实代码现场，不要求你在某条分支上才有修改全局规划的权限。预检不冻结外部 Git，仍保持代码现场单写入者；Git 与工作流没有跨系统事务，应用失败不重复改代码。

line-retention --repo <代码树> --store <目录> --line <ID> --retained-ref refs/heads/<分支> 可选 --last-release-ref refs/tags/<发布>，生成真实 Line 代码保留/维护最后发布证据；维护结束必须提供最后发布。它只检查代码对象事实，不批准关闭。


## 已批准历史改写后的恢复

获准 amend/rebase 后，若原/准备 HEAD 不再是当前代码的祖先，先确认它们由真实 branch/tag 保留。暂停当前执行的 Topic/Task，对同一 --to-line 再运行 retarget-plan，提供新的 --retained-ref 及重新核对的依赖/契约说明；该命令生成 topic.integration.prepare，应用仍使用 integration-apply。它保持 Line 归属和原始来源锚点，记录新准备 HEAD，不放宽 baseline/保留检查。

原 previous_head 已记录时可省略 --previous-head，工具复用该锚点；显式提供时必须一致。原迁移两个 HEAD 都为 null、现在已开始代码时，刷新必须明确提供实际原 HEAD；仍无代码时继续保持 null。执行前核对实际代码或无代码记录，Line 为 active；更新后旧 review/ready 依据失效，恢复 active 完成受影响验证，再 retarget-verify 和重新接受。

多轮历史改写可能分别使用不同保留 ref。工具从本 Topic 当前及历史 context 动态收集候选引用，并逐项核对真实 Git 可达性；不会因为历史里记过一个名字就视为代码仍被保留。刷新也检查旧 prepared_head 的保留，不能仅保住最早提交就丢掉后续准备成果。Topic 尚开放时，未被当前代码历史包含且恢复仍需要的旧 HEAD 必须有真实引用；清理不能只看当前 context 的单个 ref。关闭后按既有清理授权和具体留史要求处理。

## Git 项目的业务关闭

系统关闭使用通用 delivery_record/retention/last_release；按本 Topic 的实际成果选择代码或无代码检查，不因为 Project 有 Git 就虚构代码交付。代码 Topic 的 params.extensions.git 为 integration_commit/retained_ref，delivery_record 等于检查过的 integration_commit；Line 为 integration_commit/expect_tip/retained_ref/last_release_ref/last_release_commit，不适用值显式 null。保留与最后发布使用相应记录引用。squash 的具体批准证据可放在 extensions.git-review.squash_evidence。

纯调查或文档成果、此前从未建立 Topic 代码现场时，delivered/research-complete 可使用 extensions.git={integration_commit:null,retained_ref:null,no_code:<实际核对记录>}，delivery_record 引用实际报告/成果，research-complete 仍仅限 spike。核对本地索引、已有分支、工作树及远端/其他客户端交接；工具拒绝已有/曾记录代码或代码验证事实的无代码声明，并在 integration-apply 再查本地记录。跨客户端的实际无代码事实由 agent 核实，字符串不是证明。曾有原型或已丢失代码时按真实代码/保留流程处理，不能改填 no_code 绕过。

准备完整 topic.close/line.close 请求或涉及代码事实的 closure.correct 后，运行 closure-plan --repo <代码树> --store <目录> --request <请求JSON>，代码路径可 --source-ref 指定实际源。检查返回 request/platform_preview/git_observations/code_snapshot/code_revision，再用 integration-apply 同时传 --expect-revision <platform_preview.revision> 和 --expect-code-revision <code_revision> 应用；入口按成果重新核对代码整合/保留/最后发布或无代码事实。Git 配置、换 Line 和关闭均使用相同系统 CAS，不拥有特殊权限。

仅纠正 summary/evidence/verification 等通用事实、不改变 delivery_record/retention/discard_decision/last_release 或 extensions.git/git-review 时，直接使用系统 closure.correct；经 closure-plan/integration-apply 还需携带平台/现场双令牌，但不核验原代码引用、不要求原分支仍存在。新增其他扩展命名空间也不触发 Git 核验。代码事实实际变化仍走完整 Git 检查，原引用清理后需提供真实源引用/恢复依据；不能用普通摘要纠错变更代码交付事实。

直接调用 workflow.py 只校验业务事实，不证明提交存在或已合并；Git 项目的交付和代码事实变更按上述适用路径检查，通用事实纠错不额外要求 Git。非 Git 项目可以使用交付文档、发布平台或其他真实证据。

## Git Line 暂停、恢复与整合状态

业务 line.state 不要求 Git。Git Line 在状态改变前用 line-state-plan 核对实际分支 tip，暂停仍需恢复条件；可选 evidence/decision 沿用既有授权，不新增审批。返回 request/platform_preview/code_revision 后用 integration-apply 携带平台和现场令牌应用，执行前再次核对 tip。观察只保存在请求历史 observations.git，不增加核心业务字段或分支身份权限。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py line-state-plan --repo <代码树> --store <目录> --line <Line-ID> --status active --expect-tip <实际完整SHA> --date YYYY-MM-DD --reason <实际恢复原因>
```

## Git Topic 关闭请求示例

以下为交付请求结构，使用实际核对的 SHA 替换占位符；先 closure-plan，再 integration-apply，不直接把示例当作证据。取消／替代时 delivery_record 和 integration_commit 为 null，retention 写保留说明或已有明确丢弃决定；retained_ref 有保留代码时填真实 branch/tag，没有代码或已批准丢弃时为 null。

```json
{
  "action": "topic.close",
  "params": {
    "id": "<T-ID>", "outcome": "delivered",
    "summary": "实际交付摘要", "verification": "实际目标验证报告",
    "delivery_record": "<整合完整SHA>",
    "extensions": {"git": {"integration_commit": "<整合完整SHA>", "retained_ref": null}}
  },
  "date": "YYYY-MM-DD", "reason": "完成已接受的交付",
  "evidence": "实际结果证据位置", "decision": "既有验收决定位置"
}
```

## 本体观察与共享分支记录

本页工具由 agent 显式调用。代码检查失败只影响对应执行动作；本体不读取本地索引、不检查本地分支或脏状态。命名的默认顺序为常规 type、真实 Topic ID、简短描述；这是 agent 初稿 rules，用户的合法自定义名完全覆盖它。M/ITER 默认放在提交/交付说明的适当位置，不要求拼入分支名。创建/绑定实际代码后，agent 必须通过 bind-code --topic T-ID --branch 实际分支将名称写入共享 Topic 关联，交接时从 context 查询；不能仅保留私有索引。关联是显式记录，不要求名称里的编号和 Topic ID 相同，不解析名称自动迁移关系。真实业务接受、交付与代码现场检查继续按规则执行。

## 旧索引迁移与恢复

0.6.0 及以前可能在 Git 公共目录 personal-workflow-4/topics.json 和各工作树 Git 目录 personal-workflow-4/focus.json 留下记录。新适配器检测到未迁移旧文件时停止相关绑定/历史判断，不读取其内容来隐式恢复；本体完全忽略它们。

逐个现存工作树运行 git_adapter.py migrate-state --repo <实际代码根> --dry-run，核对列出的旧文件，再使用 --expect-revision <令牌> 显式导入设备目录。只迁移这两个已知 JSON 文件，结构错误、符号链接、与新状态冲突时失败并保留双方。重复执行可恢复部分完成，绝不覆盖不同的新状态。迁移后核对 current/list/check 和实际 Store/Topic/分支；搬迁导致旧 Store 路径失效时明确解绑再绑定，不修改原始历史索引掩盖记录。

迁移保留旧文件。确认所有工作树的绑定和仓库索引已保存在仓库外、备份可读后，按用户对这次迁移清理的授权，只删除已核验的这两个旧文件；其他文件、目录、Git 数据不递归清理。身份 include、hook 等历史配置另行辨认和处理，不借迁移覆盖。设备状态丢失不证明从未有代码，必须核对共享关联、远端、现存分支与交接记录再建立新绑定。

## 检查覆盖与替代流程

先执行 repo_check.py inspect --repo <代码根>。本脚本把子模块、内容过滤器、浅/部分克隆、稀疏检出、隐藏索引标志、替换/未完成历史列为不完整，dirty 为 null，禁止解读为干净。Git 适配器对涉及历史、结构变更和交付的操作也要求覆盖完整；bind/list 不认证交付。适配器及 commit_check 都不执行 commit/push。

使用 repo_check.py push-plan --repo <代码根> --remote <已确认远端名> --target refs/heads/<目标> --source <固定源SHA> --expected-remote <刚查得目标SHA> 辅助核对已有远端分支的快进范围。它只返回待审核提交，不是推送许可；新分支和不支持的情况按 [git-fallback.md](git-fallback.md) 逐项补证。

## 关闭预检的两种校验

closure-plan 检查的是准备写入 Topic/Line 的关闭事实，不执行线上回归或 Git 合并。platform_preview.revision 保护流程数据和请求的一致性；code_revision 是适配器现场令牌：保护 Store、调用仓库/工作树、适配器状态文件路径和请求；涉及关闭时还包含实际源引用/SHA、目标引用/SHA、适用保留/发布引用等事实。两者是技术校验值，不是用户批准，也不认证测试真实性。

```sh
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py closure-plan --repo <代码树> --store <目录> --request <关闭请求JSON>
python3 -B <工程>/.agents/skills/rum-project/scripts/git_adapter.py integration-apply --repo <代码树> --store <目录> --request <同一关闭请求JSON> --expect-revision <platform_preview.revision> --expect-code-revision <code_revision>
```

目标新增提交或 revert 后，即使源提交仍在目标历史，旧代码令牌也失效；源或保留引用变化、换仓库、改变请求同样拒绝。先核对影响，完成受影响验证和适用的重新接受，必要时回到 active，再更新事实和预检；不能靠刷新令牌掩盖撤销或失败。所有 integration-apply 都需要平台令牌和现场令牌，包括 Line 配置/状态、换目标、迁移验证和摘要纠错。摘要纠错只固定操作目标和请求，不要求原分支存在、不重做交付核验。旧脚本预览不可复用，先更新工程并重新预检。

此校验不冻结外部 Git，也不建立跨系统事务。预检到执行保持单 Git 写入者；代码已整合但记录失败时只重试核对后的记录，不重复合并。生成新令牌本身不证明目标正确、已验收、已发布或允许推送。


## 0.7.3 取消与丢弃的源快照

取消/替代且不保留引用时，关闭预检仍固定实际 source_ref/source_tip；源推进、回退、换引用或删除后，旧 code_revision 不可复用。相同要求适用于改变丢弃事实的 closure.correct。先重新核对本次处置范围与适用决定，再生成新预检；关闭本身不删除代码，也不批准清理。

从未建立 Topic 代码且没有本地索引/历史代码事实时，可以取消并记录明确的 null 源快照；缺少本地记录不能证明其他客户端没有代码，agent 仍须核对共享关联和交接。已记录代码却缺失引用时必须恢复或显式选择实际源，不能退化为无代码。Git Line 丢弃要求已配置的真实分支存在；未建立分支或无法恢复的特殊情况按 git-fallback.md 补齐具体事实，经正常业务接口记录，不伪造脚本成功。


## 0.8.4 操作预览与局部失败

工作树/绑定/索引迁移的 dry-run 返回 context，核对其中的调用仓库、Git 目录、适用目标、Store 和状态文件位置。令牌仅供相同目标使用：不能因为两个克隆的 HEAD 相同而互换，也不能切换 --state-home 或 Store 复用；已有绑定变化后刷新绑定预览。升级后重新预览。校验值依旧不代表 Git 操作批准。

Project ID、Store 和 Topic ID 共同确定现有绑定，切换归属先显式 unbind；不同 Project 的同编号 Topic 不是同一绑定。自定义 --state-home 会在符号链接检查后规范化，必须与全部代码工作树相互分离；新建工作树同样不能放在状态目录中。

写入阶段错误可能发生在部分结果已保存之后。changed=null 表示修改结果未能完整确认，先检查 Store/Git/外部索引，保留实际结果；不要按“未修改”执行删除或重建。migrate-state 保留旧文件，部分完成后可在源/目标未变、已有目标内容一致时幂等重试。工作流状态已改变时重新查询并预检，代码实际已创建/整合时不重复创建/整合。


## 0.8.5 状态令牌与现场令牌

Project 是逻辑工程，Store 是本机数据目录。核心 platform_preview.revision 按完整前状态、请求和后状态计算；相同 Project 快照在不同 Store 中可得到相同核心令牌，这是流程层的内容并发检查，不是物理路径授权。

适配器所有 integration-apply 都要求 --expect-revision 和 --expect-code-revision。后者仍沿用 code_revision 字段，不增加第三类令牌；code_snapshot.context 列出规范化 Store、调用仓库/工作树及实际适配器状态路径，快照同时包含请求及适用 Git 事实。切换 Store、副本克隆、工作树或 state-home 必须重新核对并预览，合法的同一规范路径别名仍等价。纯摘要纠错固定现场，但不重新要求已删除的源分支；直接使用核心 closure.correct 的语义不变。

current/list/check 与 bind 使用相同的本地绑定身份规则。显式 Store 不得与已有绑定的 Store 不同；同 Project 的另一份副本也须显式解绑后再绑定。current 从绑定推导 Store 时仍检查所属设备及未完成工程转换。check 发现错配返回非零退出码；list 可列出诊断结果，但 ok=false 并包含明确 issues，不能把列表命令退出成功当健康。

程序升级不会替换旧工程脚本。0.8.4 工程显式 project-update 后再验收此契约，所有旧预览重新生成。
