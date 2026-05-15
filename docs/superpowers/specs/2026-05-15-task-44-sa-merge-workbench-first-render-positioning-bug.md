# Task 44 SA 方案：merge workbench 首开首帧定位缺陷

- Date: 2026-05-15
- Task: 44
- Role: SA
- Status: done
- Fix Mode: `minimal-fix`
- Current Gate: `FAIL`
- Risk: `High`
- Backend Reuse Classification: `shared-reuse`
- Requirement Truth:
  - `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md`
- QA Factual Fail:
  - `docs/plans/2026-05-15-task-43-qa-merge-workbench-factual-check.md`
- PR Copy Reference:
  - `docs/plans/2026-05-15-task-43-po-pr-description-polish.md`

## $maestro-analyze

### 1. 问题归类

本轮不是需求扩写，也不是单纯高亮不明显，而是已确认的首开首帧定位缺陷：

1. 三栏 merge workbench 首次打开时，当前冲突块不保证立即可见。
2. 用户手动触发一次 `上一处冲突` / `下一处冲突` 或等价后续交互后，当前冲突块才出现。
3. `1 / 1` 计数、disabled 导航、current-conflict 样式本身已经存在，因此问题更符合 Monaco mount 完成晚于首轮 reveal / decoration effect 的时序缺口。

### 2. 根因判断

根因判断收口为前端首帧 render / positioning timing bug，具体锚点如下：

1. [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx) 在 `useEffect([mergeResult])` 中首开就执行 `setActiveBlockIndex(0)` 与 `setNavigationToken(...)`。
2. 同文件中 decorations effect 依赖 `[activeBlockIndex, conflictBlocks]`，reveal effect 依赖 `[activeBlockIndex, conflictBlocks, navigationToken]`。
3. 两个 effect 在 `editorRefs.current[pane].editor/monaco` 仍为空时都会直接 `return`。
4. `handleEditorMount` 当前只把 `editor` / `monaco` 写入 ref，不会触发任何 state 更新，因此 mount 完成后不会自动补跑 decorations / reveal。
5. [CodeEditor.tsx](/H:/github/opskat/opskat/frontend/src/components/CodeEditor.tsx) 的 `onMount` 是 Monaco 创建完成后的异步回调，因此完全可能出现：
   - 先执行 merge 初始化 state
   - 首轮 effect 因 editor 为空而跳过
   - 后续 mount 只写 ref，不重跑 effect
   - 直到人工导航再次改动 `navigationToken` / `activeBlockIndex` 才补出当前冲突块

因此，本轮不应把问题误判为：

1. 冲突识别缺失
2. 导航逻辑缺失
3. current-conflict 样式缺失
4. 需要重做 merge 状态机

### 3. 最小修复边界

最小修复边界只落在 [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx)：

1. 为 merge workbench 增加一个轻量 mount-ready / mount-version 状态，用来表达“至少有 editor 完成 mount，且应补跑首开同步”。
2. decorations effect 与 reveal effect 显式依赖这个 mount-ready / mount-version 状态，确保 mount 完成后必有一次补跑。
3. reveal 逻辑在首次补跑时增加一层轻量延后一帧处理，例如 `requestAnimationFrame`，确保 Monaco 首次 layout 已稳定后再 `revealLineInCenter` / `setPosition`。

本轮不建议扩到：

1. [CodeEditor.tsx](/H:/github/opskat/opskat/frontend/src/components/CodeEditor.tsx) 通用层。原因是 bug 只发生在 merge workbench 首帧定位语义，不是所有 CodeEditor 的通用 mount contract 都需要带“首帧可见性补跑”语义。
2. [FileManagerPanel.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/FileManagerPanel.tsx) 或 store。原因是冲突计数、入口、打开动作都已正确，问题发生在 merge workbench 内部。
3. `merge-decorations.ts` / `globals.css`。原因是当前 current-conflict line/gutter class 与样式已经存在，不是 decoration 规则缺失。

### 4. 实现策略

#### 4.1 mount 完成后的 effect 重跑

建议在 merge workbench 内新增一个本地状态，例如：

1. `editorMountVersion`
2. 或 `mountedPaneCount`

要求：

1. 每次 `handleEditorMount` 被调用时递增或推进该状态。
2. decorations effect 与 reveal effect 把它纳入依赖。
3. effect 内仅在 `conflictBlocks.length > 0` 且目标 pane editor 已准备好时执行。

收口语义：

1. 首轮 `mergeResult` 初始化仍保留 `setActiveBlockIndex(0)` 与 `setNavigationToken(...)`。
2. 若首轮 effect 因 editor 未 mount 被跳过，mount 状态推进后必须再跑一次。
3. 这次补跑不能依赖用户手点导航。

#### 4.2 active block reveal 策略

reveal 策略建议分两段：

1. state 层仍以 `activeBlockIndex` 为唯一当前冲突来源，不新增第二套“首帧焦点”状态。
2. 首次 mount 补跑时，通过 `requestAnimationFrame` 包裹 `revealLineInCenter` 与 `setPosition`，避免 Monaco 尚未 layout 完成时定位失效。

最小要求：

1. 对当前 active block 的三栏都执行同步 reveal。
2. 单冲突 `1 / 1` 场景即使导航按钮 disabled，也必须完成 reveal。
3. multi-conflict 场景保持现有导航逻辑，不额外重写导航算法。

#### 4.3 decoration 同步策略

当前 decoration 构建逻辑可继续复用 [merge-decorations.ts](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/merge-decorations.ts)：

1. `buildMergePaneDecorations(...)`
2. `external-edit-merge-line-current`
3. `external-edit-merge-gutter-current`

本轮只要求 mount 后补跑，保证首帧当前冲突块 decorations 真正落到 editor 实例上。

### 5. 受影响面

#### 必改

1. [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx)

#### focused 测试

1. [FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx)

#### 预计无需改动

1. [CodeEditor.tsx](/H:/github/opskat/opskat/frontend/src/components/CodeEditor.tsx)
2. [merge-decorations.ts](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/merge-decorations.ts)
3. [globals.css](/H:/github/opskat/opskat/frontend/src/styles/globals.css)
4. backend Go service / Wails IPC / store

### 6. 风险点

1. 若只做 mount 后 `ref` 写入、不引入 state 级补跑信号，则首帧 bug 仍可能保留。
2. 若把补跑做成过宽的通用 `CodeEditor` contract，容易误伤 compare/editor 其他使用方，超出本轮最小 bugfix 边界。
3. 若 reveal 不等待首帧 layout 稳定，某些 Monaco 时序下仍可能出现 `setPosition` 已调用但 viewport 未落到冲突块的假修复。

## $maestro-plan

### 1. DEV 最小实现顺序

1. 在 `MergeWorkbench.tsx` 增加 mount 完成后的本地补跑信号，仅影响 merge workbench。
2. 让 decorations effect 与 reveal effect 依赖该补跑信号。
3. 在 reveal 中加入一帧延后执行，保证 mount 后的首帧定位稳定。
4. 补前端 focused 测试，覆盖“先打开 workbench，再 mount editor”时序。
5. 跑 focused 测试，确认单冲突 `1 / 1` 语义与现有导航状态不回退。

### 2. 需要补的前端测试

在 [FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx) 补至少两类断言：

1. 首开 merge workbench 后，无需人工导航，`CodeEditor` mock 的三栏 editor 都会收到：
   - `createDecorationsCollection(...)`
   - `revealLineInCenter(...)`
   - `setPosition(...)`
2. 单冲突 `1 / 1` 场景下，即便上一处/下一处按钮 disabled，上述调用也在 mount 后自动发生。

建议测试手法：

1. 复用现有 `CodeEditor` mock 暴露的 spy。
2. 显式验证 mount 回调触发后，自动调用次数大于 0。
3. 不把测试写成“点击上一处/下一处后才有 reveal”，否则会掩盖本轮 bug。

### 3. 验收点

#### PASS

1. 用户首次打开 merge workbench，无需任何额外交互，即可看到当前冲突块。
2. 单冲突场景首开即显示 `1 / 1`，且当前冲突块已经可见。
3. 当前冲突块在三栏都有 current-conflict line/gutter 强化，不依赖手动导航后才出现。
4. 导航按钮原有 disabled / enabled 语义不回退。

#### FAIL

1. 首次打开仍需手点 `上一处冲突` / `下一处冲突` 才看到当前冲突块。
2. 只有 count 正确但 reveal / current highlight 仍未自动落地。
3. 修复后引入 compare workbench、pending dialog 或 merge 保存链路的无关回归。

### 4. focused 回归面

1. single-conflict `1 / 1` 计数与 disabled 导航不变。
2. multi-conflict 场景上一处/下一处导航仍按既有方向工作。
3. merge 保存、取消关闭、dirty confirm 不受影响。
4. compare 双栏与外部普通 CodeEditor 使用场景不因本轮修复被连带修改。

## 结论

task 44 作为 focused SA bugfix 方案已收口完成。当前 authoritative gate 仍为 `QA FAIL / Risk: High`，原因不是状态识别缺失，而是首帧 mount/effect 时序导致 reveal 与 current-conflict decorations 未必在首开可见。推荐下游角色：`DEV` 按最小前端边界修复 `MergeWorkbench.tsx` 首帧 mount 时序，并由 `QA` 按 requirement truth 对“首开无需人工导航即可看到当前冲突块”做 focused 复核。
