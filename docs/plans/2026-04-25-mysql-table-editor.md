# MySQL Table Editor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 MySQL 表数据页升级为完整的数据编辑工作台，补齐右键菜单、工具栏、导入导出、刷新停止和安全提交能力。

**Architecture:** 第一阶段尽量复用现有 `TableDataTab` 的分页、筛选、排序、编辑、SQL 预览和提交流程；`QueryResultTable` 只负责表格交互和菜单事件上抛，实际 SQL 构造、执行、导入导出由 `TableDataTab` 编排。危险写操作统一进入确认或 pending edit 流程，避免静默写库。

**Tech Stack:** React 19, TypeScript, Zustand, Wails IPC `ExecuteSQL`, Vitest, React Testing Library, `@opskat/ui`, `lucide-react`, i18next.

## PR Strategy

按小 PR 合并，不把所有能力塞进一个巨型 PR。每个 PR 必须能独立 review、独立测试、独立回滚。

| PR | Scope | Tasks | Depends On | Recommended Branch | Commit Prefix |
| --- | --- | --- | --- | --- | --- |
| PR 1 | 单元格右键基础编辑动作 | Task 1 | none | `codex/mysql-table-cell-context-actions` | `✨` |
| PR 2 | 右键筛选/排序 | Task 2 | PR 1 | `codex/mysql-table-context-filter-sort` | `✨` |
| PR 3 | 删除记录确认 | Task 3 | PR 1 | `codex/mysql-table-delete-record` | `✨` |
| PR 4 | 编辑器工具栏和底部状态栏 | Task 4 | PR 1, PR 3 | `codex/mysql-table-editor-toolbar` | `🎨` |
| PR 5 | 导出 CSV/TSV/SQL | Task 5 | PR 1 | `codex/mysql-table-export` | `✨` |
| PR 6 | 导入 CSV/TSV 预览 | Task 6 | PR 5 | `codex/mysql-table-import-preview` | `✨` |
| PR 7 | 停止加载 | Task 7 | PR 4 | `codex/mysql-table-stop-loading` | `✨` |
| PR 8 | Copy As 和 UUID | Task 8 | PR 1, PR 5 | `codex/mysql-table-copy-as` | `✨` |
| PR 9 | 表格显示设置 | Task 9 | PR 4 | `codex/mysql-table-display-settings` | `🎨` |

Recommended first delivery batch: PR 1, PR 2, PR 3, PR 4. This batch completes the edit workflow before import/export work starts.

## PR Rules

- Keep each PR focused on one task group. Do not combine import/export, toolbar, deletion, and display settings in one PR.
- Do not edit generated Wails files in `frontend/wailsjs/**`.
- Do not introduce a second table component. Extend `QueryResultTable` and `TableDataTab`.
- Do not execute database writes directly from the grid component. Grid actions must be callbacks; `TableDataTab` owns SQL generation and Wails calls.
- Do not silently execute destructive actions. Delete/import/write actions require pending edit, SQL preview, or confirmation.
- Do not add new package dependencies unless the PR explains why existing platform APIs are insufficient.
- Every new user-facing string must be added to both `zh-CN` and `en` locale files.
- Every PR must include frontend tests for the behavior it changes.

## Review Checklist

Use this checklist in every PR description:

- [ ] Scope is limited to the planned PR.
- [ ] No generated files were edited by hand.
- [ ] Existing table editing behavior still works.
- [ ] New strings are localized in Chinese and English.
- [ ] Dangerous operations have confirmation or preview.
- [ ] NULL, empty string, Chinese text, quotes, and numbers are handled deliberately.
- [ ] Tests were added or updated.
- [ ] Verification commands and results are listed in the PR.

## PR Description Template

```markdown
## Summary
-

## Scope
-

## Out of Scope
-

## Screenshots / Recording
-

## Test Plan
- [ ] `cd frontend && pnpm test -- <specific-test-file>`
- [ ] `cd frontend && pnpm lint`
- [ ] Manual: open a MySQL table and verify ...

## Risk
-

## Rollback
- Revert this PR. No migration or data format change is included.
```

## Milestone 1: 编辑闭环 P0

### Task 1: 右键菜单基础动作

**目标:** 单元格右键菜单支持复制、复制字段名、粘贴、设为空字符串、设为 NULL、刷新入口。

**Files:**
- Modify: `frontend/src/components/query/QueryResultTable.tsx`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: `frontend/src/__tests__/QueryResultTable.test.tsx`

**Steps:**
1. Write or extend tests in `frontend/src/__tests__/QueryResultTable.test.tsx` for each new menu action.
2. Run `cd frontend && pnpm test -- QueryResultTable.test.tsx`; expected: failing tests for missing menu/actions.
3. 为 `QueryResultTable` 增加明确 callbacks：`onSetCellValue`, `onPasteCell`, `onCopyFieldName`, `onRefresh`.
4. 扩展当前 cell context menu，展示：
   - Set to Empty String
   - Set to NULL
   - Copy
   - Copy Field Name
   - Paste
   - Refresh
5. 粘贴从 `navigator.clipboard.readText()` 读取，写入 pending edit，不直接提交。
6. `Set to NULL` 写入 `null`；`Set to Empty String` 写入 `""`。
7. `Copy Field Name` 复制当前列名。
8. 增加中英文 i18n 文案。
9. Run `cd frontend && pnpm test -- QueryResultTable.test.tsx`; expected: pass.
10. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 右键任意可编辑单元格能看到基础菜单。
- 操作 NULL、空字符串、粘贴后，下方 pending edits 条出现。
- 复制值和复制字段名写入剪贴板。

**Commit:** `✨ add mysql table cell context actions`

### Task 2: 筛选、排序、清除条件右键动作

**目标:** 右键菜单可以按当前值筛选、按当前列排序、清除所有筛选排序。

**Files:**
- Modify: `frontend/src/components/query/QueryResultTable.tsx`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: `frontend/src/__tests__/QueryResultTable.test.tsx`

**Steps:**
1. Write failing tests for filter-by-value, sort-by-column, and clear-filter-sort actions.
2. Run the focused tests and confirm they fail for missing callbacks/actions.
3. `QueryResultTable` 在右键状态中保留 `rowIdx`, `col`, `value`。
4. 增加 callbacks：`onFilterByCellValue`, `onSortByColumn`, `onClearFilterSort`。
5. `TableDataTab` 实现按值筛选：
   - `NULL` -> ``col IS NULL``
   - 字符串/数字 -> ``col = quotedValue``
   - 新条件写入 `whereInput` 和 `whereClause`，并回到第一页。
6. 排序复用现有 `handleSortChange`。
7. 清除筛选排序时清空 `whereInput`, `whereClause`, `orderByInput`, `orderByClause`, `sortColumn`, `sortDir`。
8. Run focused tests; expected: pass.
9. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 右键某个值选择筛选后，表格刷新为对应 WHERE 条件。
- 右键列排序会刷新第一页并显示排序方向。
- 清除后恢复无 WHERE/ORDER 状态。

**Commit:** `✨ add mysql table context filter and sort`

### Task 3: 删除记录确认

**目标:** 支持右键或工具栏删除当前选中记录，执行前展示 SQL 确认。

**Files:**
- Modify: `frontend/src/components/query/QueryResultTable.tsx`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Reuse: `frontend/src/components/query/SqlPreviewDialog.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: `frontend/src/__tests__/QueryResultTable.test.tsx`
- Test: create `frontend/src/__tests__/TableDataTab.delete.test.tsx` if practical

**Steps:**
1. Write failing tests for generated DELETE SQL with primary key and without primary key.
2. Write a failing interaction test for right-click `Delete Record...` opening confirmation.
3. `QueryResultTable` 上抛 `onDeleteRow(rowIdx)`，右键菜单增加 `Delete Record...`。
4. `TableDataTab` 新增 `buildDeleteStatement(rowIdx)`。
5. 优先用 `primaryKeys` 构造 WHERE；没有主键时退回全列匹配并显示风险文案。
6. MySQL DELETE 使用：`DELETE FROM db.table WHERE ... LIMIT 1;`
7. 删除前打开确认对话框，展示 SQL。
8. 确认后调用 `ExecuteSQL`，成功后刷新当前页和 count。
9. Run focused tests; expected: pass.
10. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 删除不会直接执行，必须确认。
- 有主键时 WHERE 只用主键。
- 删除成功后当前页刷新，toast 展示影响行数。

**Commit:** `✨ add mysql table row deletion`

### Task 4: 顶部和底部编辑工具栏

**目标:** 把常用编辑动作固定在表格上方/下方，减少只能靠右键或顶部表单操作的问题。

**Files:**
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: create or extend `frontend/src/__tests__/TableDataTab.toolbar.test.tsx`

**Steps:**
1. Write failing tests for toolbar disabled/enabled states.
2. Write failing tests for toolbar submit/discard/refresh/delete handlers.
3. 顶部工具栏保留现有 WHERE/ORDER BY 输入，但补齐图标按钮：
   - 新增
   - 删除
   - 提交修改
   - 放弃修改
   - 刷新
   - 停止
   - 导入
   - 导出
   - SQL 预览
4. 底部状态栏显示：
   - pending edit count
   - 当前待执行 SQL 摘要
   - 分页控制
   - 刷新/停止快捷按钮
5. 有 pending edits 时启用提交、放弃、SQL 预览；无 pending edits 时禁用。
6. 无选中行时禁用删除。
7. 现有分页逻辑迁移到更接近截图的底部区域，避免重复入口。
8. 所有按钮使用 lucide 图标和 tooltip/title。
9. Run focused tests; expected: pass.
10. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 用户无需右键也能完成新增、删除、提交、放弃、刷新。
- 禁用态符合当前选中和编辑状态。
- 分页功能保持原行为。

**Commit:** `🎨 add mysql table editor toolbar`

## Milestone 2: 数据流转 P1

### Task 5: 导出数据

**目标:** 支持将当前页或当前筛选结果导出为 CSV/TSV/INSERT SQL。

**Files:**
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Create: `frontend/src/lib/tableExport.ts`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: create `frontend/src/__tests__/tableExport.test.ts`

**Steps:**
1. Write failing tests in `frontend/src/__tests__/tableExport.test.ts` for CSV escaping, TSV escaping, NULL handling, and INSERT SQL quoting.
2. Run `cd frontend && pnpm test -- tableExport.test.ts`; expected: fail because helpers do not exist.
3. 新增 `tableExport.ts`，实现：
   - `toCsv(columns, rows)`
   - `toTsv(columns, rows)`
   - `toInsertSql(tableName, columns, rows, driver)`
4. CSV 处理逗号、双引号、换行。
5. NULL 导出策略：
   - CSV/TSV 默认空值
   - SQL 使用 `NULL`
6. 导出入口提供格式选择。
7. 第一版导出当前已加载页；后续再加“导出全部筛选结果”。
8. 通过浏览器下载 Blob 文件。
9. Run `cd frontend && pnpm test -- tableExport.test.ts`; expected: pass.
10. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 当前页可下载 CSV、TSV、INSERT SQL。
- 中文、逗号、换行、NULL 导出正确。
- 单元测试覆盖转义规则。

**Commit:** `✨ add mysql table export`

### Task 6: CSV/TSV 导入预览

**目标:** 用户选择 CSV/TSV 文件后，先预览字段映射和导入 SQL，不直接写库。

**Files:**
- Create: `frontend/src/components/query/ImportTableDataDialog.tsx`
- Create: `frontend/src/lib/tableImport.ts`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: create `frontend/src/__tests__/tableImport.test.ts`

**Steps:**
1. Write failing parser tests in `frontend/src/__tests__/tableImport.test.ts` for CSV quotes, embedded commas, embedded newlines, TSV, empty cells, and NULL strategy.
2. Run `cd frontend && pnpm test -- tableImport.test.ts`; expected: fail because helpers do not exist.
3. `tableImport.ts` 实现 CSV/TSV 解析。优先用轻量可靠 parser；如果不引入依赖，必须支持 quoted CSV。
4. Dialog 流程：
   - 选择文件
   - 识别分隔符
   - 预览前 20 行
   - 字段映射到当前表 columns
   - NULL/空字符串策略选择
   - 生成 INSERT SQL 预览
5. 第一版只支持 INSERT，不做 UPDATE/UPSERT。
6. 执行前复用 SQL 确认体验。
7. 执行完成展示成功/失败/影响行数，并刷新表格。
8. Run `cd frontend && pnpm test -- tableImport.test.ts`; expected: pass.
9. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 导入必须经过预览和确认。
- 字段不匹配时有明确提示。
- 导入成功后刷新当前表。

**Commit:** `✨ add mysql csv import preview`

## Milestone 3: 体验增强 P2

### Task 7: 停止当前加载/查询

**目标:** 提供停止按钮，至少能停止 UI 接收当前请求结果，后续再评估后端连接级取消。

**Files:**
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: create or extend `frontend/src/__tests__/TableDataTab.loading.test.tsx`

**Steps:**
1. Write a failing test where an older request resolves after stop and must not update rows/count/error.
2. Run the focused loading test; expected: fail because cancellation token does not exist.
3. 在 `fetchData`, `fetchCount`, 导入执行中维护 request token。
4. 点击停止后标记当前 token cancelled。
5. 异步返回时如果 token 已取消，不更新 rows/count/loading error。
6. UI 上停止按钮仅在 loading/importing/exporting 时启用。
7. toast 提示“已停止等待当前操作结果”。
8. Run focused tests; expected: pass.
9. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 长加载中点击停止，UI 不再被旧请求覆盖。
- 刷新后新请求结果可以正常显示。

**Commit:** `✨ add mysql table stop loading`

### Task 8: Copy As 和 UUID

**目标:** 补齐右键高级复制和快速生成值。

**Files:**
- Modify: `frontend/src/components/query/QueryResultTable.tsx`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Reuse/Modify: `frontend/src/lib/tableExport.ts`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: `frontend/src/__tests__/QueryResultTable.test.tsx`
- Test: `frontend/src/__tests__/tableExport.test.ts`

**Steps:**
1. Write failing tests for UUID action and each Copy As output format.
2. Run focused tests; expected: fail because actions are missing.
3. 右键菜单增加 `Generate UUID`。
4. 使用 `crypto.randomUUID()`，结果写入当前单元格 pending edit。
5. 增加 `Copy As` 子菜单：
   - INSERT Statement
   - UPDATE Statement
   - Tab Separated Values (Data only)
   - Tab Separated Values (Field Name only)
   - Tab Separated Values (Field Name and Data)
6. 单行 `UPDATE Statement` 优先使用主键 WHERE。
7. 复制成功 toast。
8. Run focused tests; expected: pass.
9. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- UUID 生成后可预览并提交。
- Copy As 的 SQL/TSV 内容符合当前行和当前字段。

**Commit:** `✨ add mysql table copy as actions`

### Task 9: 表格设置与列显示

**目标:** 提供列显示/隐藏、行高设置、视图设置入口。

**Files:**
- Modify: `frontend/src/components/query/QueryResultTable.tsx`
- Modify: `frontend/src/components/query/TableDataTab.tsx`
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`
- Test: `frontend/src/__tests__/QueryResultTable.test.tsx`

**Steps:**
1. Write failing tests for hiding/showing columns and row density classes.
2. Run focused tests; expected: fail because settings do not exist.
3. 增加 visible columns 状态。
4. 表格设置菜单可勾选显示/隐藏列。
5. 行高提供 compact/default/comfortable 三档。
6. 设置只作用于当前打开的表 tab；后续再考虑持久化。
7. Run focused tests; expected: pass.
8. Run `cd frontend && pnpm lint`; expected: pass.

**Acceptance:**
- 隐藏列不影响已有 rows 数据和 SQL 构造。
- 行高切换不破坏编辑、右键和分页。

**Commit:** `🎨 add mysql table display settings`

## Verification

Run after each task:

```bash
cd frontend && pnpm test
cd frontend && pnpm lint
```

Run before merging the full feature:

```bash
make test
cd frontend && pnpm test
cd frontend && pnpm lint
cd frontend && pnpm build
```

Manual checks:
- Open a MySQL table data tab.
- Right-click a normal value, NULL value, and empty string.
- Edit multiple cells, preview SQL, submit, then refresh.
- Delete one row with primary key and confirm generated SQL.
- Filter by current cell value, sort by column, clear all filters and sorts.
- Export CSV/TSV/INSERT SQL and inspect output.
- Import CSV via preview and confirm.
- Start a refresh/load and click stop.

## Merge Readiness

Before opening each PR:

```bash
git status --short
cd frontend && pnpm test -- <changed-test-file>
cd frontend && pnpm lint
```

Before marking a PR ready for review:

```bash
cd frontend && pnpm test
cd frontend && pnpm lint
```

Before merging a milestone branch or release batch:

```bash
make test
cd frontend && pnpm test
cd frontend && pnpm lint
cd frontend && pnpm build
```

PRs that touch SQL generation must include sample generated SQL in the PR description. PRs that touch UI layout must include screenshots or a short recording for normal, loading, pending-edit, and error states.
