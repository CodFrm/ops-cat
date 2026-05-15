# Task 43 PO PR Description Polish Draft

- Date: 2026-05-15
- Conversation: `01KQ6Q3BRYNKTFSMG4746A4P9M`
- Branch Baseline: `feat/remote-file-external-editing-clean`
- Scope: 基于 owner 与 reviewer 当前往返问题，整理 external edit 这条 PR 的正式描述文案；补齐三栏 merge、SSH 会话 rebind、TOCTOU、大文件上限、manifest schema 版本等设计动机与边界说明；仅整理文案，不改实现
- Status: done
- Fix Mode: `staged-repair`
- Evidence Basis: `docs/plans/2026-05-15-task-43-qa-merge-workbench-factual-check.md`
- Addendum Conversation: `01KQ6DK2XK6ZKY7Q7JN8QFXSQ0`

说明：
本稿用于替换或重写当前 external edit PR 正文，目标是让 reviewer 能直接看懂“为什么这样设计、边界在哪里、哪些问题已经兜底、哪些是明确不在本次范围内”。
本稿只承载 PR copy，不作为本轮 task 43 追加需求的 canonical requirement truth。
本轮三栏 merge 当前冲突高亮 / 单冲突定位的正式需求边界，统一以 `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md` 为准。

## 建议直接用于 PR 的正文

## Summary

本 PR 为远程文件 external edit 补齐一条完整的桌面端工作链路：用户可以从文件管理器直接打开远程文件到本地工作副本，使用外部编辑器修改，并在保存时通过统一的冲突处理链路回写远程。

本次改动不只是“打开文件并覆盖回去”，而是把以下几个容易出错的场景一起收口：

- 运行中保存时，远端文件已经被改动
- 运行中保存时，远端文件已经被删除
- 应用重启后，本地仍有未完成处理的副本
- 外部编辑生命周期长于单个 SSH 会话
- 用户需要在本地修改、远端新版本、共同基线之间做可见的冲突处理

交付结果包括：

- 文件管理器中的 external edit 打开入口
- 统一的 pending / conflict / error 弹窗收口
- 类 IDEA 的三栏 merge 工作台
- merge 工作台的目标行为是首开即可看到当前冲突点；单冲突场景默认定位唯一冲突并显示 `1 / 1`
- 基于资产与文档身份的 SSH 会话 rebind / 恢复
- 本地工作副本与 manifest 持久化恢复
- 面向 full-file external edit 的大文件上限保护

## Design Motivation

### 1. 为什么新增三栏 merge 工作台

在 #54 的既有共识里，冲突处理主要只有“重新读取远程”和“覆盖远程”两类动作。这个口径足够简单，但在真实冲突场景里并不够用：

- 用户改过本地副本
- 远端文件也被他人改过
- 这时如果只给“重读”或“覆盖”，用户很难判断到底差异在哪里，更难手动整理冲突内容

因此本 PR 新增三栏 merge 工作台，把“共同基线 / 本地修改 / 远端最新”同时呈现出来。这样做的目的不是增加功能花样，而是把原本需要用户盲选的冲突处理，变成一个可见、可判断、可手动收口的流程。

这套 merge 工作台同时覆盖两类来源：

- 运行中的保存冲突
- 应用重启后恢复出来的待处理冲突

也就是说，用户无论是在运行中遇到冲突，还是重启后继续处理，看到的都是同一套决策模型，而不是两套分裂的处理方式。

这套 merge 工作台的既定行为不只是“能打开三栏”，还包括首开即可感知当前冲突位置：

- 首帧直接看到当前冲突点
- 若只有 1 处冲突，则默认定位到唯一冲突块并显示 `1 / 1`
- 当前冲突块应以更强的 line / gutter emphasis 可见，而不是等用户手动点 `上一处` / `下一处` 后才知道焦点在哪里

### 2. 为什么把 external edit 从“绑定 SSH 会话”提升到“绑定资产 + 文档身份”

第一版 external edit 如果严格绑死在某个 SSH 会话上，会遇到一个现实问题：外部编辑器的生命周期往往比单个 SSH 会话更长，甚至可能比应用进程本身更长。

典型场景包括：

- SSH 会话被用户主动关闭
- SSH 会话因为网络原因中断
- 应用被关闭或异常退出
- 用户重启应用后，希望继续处理之前未完成的本地副本

如果 external edit 仍然只认原始 SSH session，那么这些场景下，本地副本虽然还在，但回写远程、继续冲突处理、恢复显示都会失效。

所以本 PR 把恢复与回写的身份收口到“资产 + canonical remote path / document identity”上。这样同一资产下重新建立会话后，系统可以尝试重新绑定 transport，让未完成的 external edit 继续可处理。

这里不是无条件乱绑，会保留两个边界：

- 只有候选会话唯一时才允许自动 rebind
- 仍会结合 `RealPath` / canonical path 做校验，避免把副本错误地绑到别的远端文件

如果这些条件不满足，系统不会静默猜测，而是要求用户在同一资产中重新打开目标文件，避免误写错目标。

## Save / Conflict Semantics

external edit 的保存语义不是“本地一保存就直接覆盖远端”，而是先做远端基线校验，再决定走哪条分支：

- 如果远端仍然等于当前基线，则正常回写
- 如果远端已变化，则进入 conflict 流
- 如果远端已不存在，则进入 remote missing / recreate 流

这样做的目标是把“误覆盖他人改动”从默认行为改成显式决策。

## Boundary Notes

### 1. TOCTOU：为什么这里不能做到完全原子

本 PR 已经尽量把风险压低，但这里仍有一个需要明确写出的边界：

保存前的 `Stat + ReadFile` 与最后的写回并不是一个“远端快照级原子事务”。

原因不是实现偷懒，而是通用 SFTP 本身没有稳定、跨实现一致的“按版本比较后写入”或“带条件的原子 compare-and-swap”能力。当前这条链路能稳定做到的是：

- 保存前先读取并比对远端状态，尽量拦住已经发生的外部改动
- 最终写回阶段使用原子替换，避免把远端文件写成半截

但如果远端在“检查完成之后、最终写回之前”的很小窗口里又被改了一次，客户端无法依靠通用 SFTP 协议把这件事彻底消灭。

所以当前设计的真实边界是：

- 能拦住保存前已经发生的远端漂移
- 能保证最终写回本身尽量完整
- 不能承诺 generic SFTP 上的零窗口并发覆盖

这部分剩余风险，当前由用户显式的 `merge / overwrite / reread / recreate` 决策流兜底。

### 2. 大文件上限为什么放在底层

external edit 不是流式编辑，而是“完整读取远程文件 -> 落本地副本 -> 打开外部编辑器 -> 再完整读取本地副本做比较/回写”这套 full-file 链路。

因此，如果不加限制，几十 MB 甚至更大的日志文件会直接进入整文件读取，既容易放大内存占用，也容易让桌面端交互卡顿。

本 PR 已在底层 `sftp_svc.ReadFile()` 增加 full-file 读取上限，当前阈值为 `10MB`，并在 local copy 读取侧保持同样的限制。这样做的目的，是让所有依赖“完整读入内容”的桌面特性共用一条保护线，而不是把限制散落在某个单独页面或按钮上。

这个限制影响的是：

- external edit 打开远程文件
- reread / merge 等需要完整读取内容的链路

这个限制不影响的是：

- 普通 SFTP 目录浏览
- 普通文件上传 / 下载
- 非 full-file 的其他远程操作

### 3. `manifestVersion = 4` 的含义

这里的 `manifestVersion` 不是数据库 schema 版本，而是 external edit 本地 manifest JSON 的格式版本。

开发过程中，这份 manifest 结构经历过多次迭代，因此版本号已经走到 `v4`。但相对 `main` 来说，external edit 这一整条能力尚未正式进入主线，因此：

- `v2 / v3` 属于开发阶段的中间 schema
- 本 PR 合入后，`v4` 视为首个正式面向主线用户的持久化 schema

也因为这一点，当前实现没有补 `v1 ~ v3` 的正式迁移链。这里的前提是：这些早期 schema 不作为主线已发布用户数据来承诺兼容。

如果后续 external edit 在进入主线后再次修改 manifest 结构，则需要按正式迁移问题处理，而不是继续依赖“开发期中间版本不迁移”的口径。

## Test Plan

- Go focused tests 覆盖 external edit 主状态机，包括 open / save / conflict / remote missing / reread / merge / rebind / restart restore
- 前端 focused tests 覆盖文件管理入口、统一 pending dialog、三栏 merge 工作台、error / recovery 投影
- 验证三栏 merge 第一次打开时即可看到当前冲突点，不需要先手动点击导航按钮
- 验证单冲突场景首开直接定位唯一冲突块，计数为 `1 / 1`，`上一处` / `下一处` 为 disabled
- 验证大文件保护在 external edit 链路内生效
- 验证同资产多会话时，rebind 只在候选唯一且路径校验通过时成立
- 验证冲突场景下，运行中冲突与重启恢复冲突走同一套 merge / overwrite / reread 决策面

## Requirement Reference

本轮 task 43 关于“三栏 merge 当前冲突高亮与单冲突定位”的 requirement truth、最小复现、期望行为、FAIL 验收口径与分类结论，统一引用：

- `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md`

本稿中已经保留了需要反映到 PR copy 的摘要性文案，包括：

1. merge 工作台首开即定位当前冲突块
2. 单冲突场景默认定位唯一冲突块并显示 `1 / 1`
3. 当前冲突高亮首帧可见

## Out of Scope

以下内容不在本 PR 范围内：

- generic SFTP 上的远端全链路强原子锁 / compare-and-swap 能力
- standalone 的“记录管理 / 副本管理”产品化入口
- external edit 之外的其他远程文件编辑模型

## Reviewer Notes

如果 reviewer 主要关注“为什么不是只做 reread / overwrite”、“为什么要做 rebind”、“为什么没有完全原子”、“为什么 `manifestVersion` 已经到 4 但没迁移”、“为什么大文件限制要放到底层”，以上各节就是本 PR 的正式设计说明。
