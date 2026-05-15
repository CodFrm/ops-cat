# task 47 QA focused review

- Date: 2026-05-15
- Branch: `feat/remote-file-external-editing-clean`
- Verified HEAD: `311294d54467bdc21abcbe50a1acc38e534cbf18`
- Requirement truth:
  - `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md`
- QA factual fail baseline:
  - `docs/plans/2026-05-15-task-43-qa-merge-workbench-factual-check.md`
- SA solution:
  - `docs/superpowers/specs/2026-05-15-task-44-sa-merge-workbench-first-render-positioning-bug.md`
- SEC review:
  - `docs/superpowers/specs/2026-05-15-task-46-sec-merge-workbench-review.md`

## Verdict

- Result: PASS
- Risk: Medium

## Scope

1. First open of merge workbench shows the current conflict block without manual `上一处冲突 / 下一处冲突`.
2. Single-conflict `1 / 1` counter and disabled navigation semantics do not regress.
3. This round does not introduce a new merge workbench visibility regression in the reviewed surface.
4. Frontend build failure remains limited to pre-existing repository dependency gaps, not this patch surface.

## Execution

### Focused tests

- Command:
  - `cd frontend && pnpm test -- --run src/__tests__/MergeWorkbench.test.tsx src/__tests__/FileManagerPanel.test.tsx`
- Result:
  - PASS
  - `2` files / `25` tests

### Build check

- Command:
  - `cd frontend && pnpm build`
- Result:
  - FAIL
- Observed errors:
  - `src/components/ai/AIChatInput.tsx(4,23)`: missing `@tiptap/extension-hard-break`
  - `src/components/layout/TopBar.tsx(9,8)`: missing `@tabler/icons-react`

## Focused Evidence

### 1. First-open reveal is now re-run after editor mount

[MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:34) adds the merge-local `editorMountVersion` signal and [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:42) tracks a `requestAnimationFrame` handle for reveal cleanup.

[MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:57) re-applies decorations when `editorMountVersion` changes, and [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:67) re-runs reveal/positioning after mount with `requestAnimationFrame`, calling `revealLineInCenter(...)` and `setPosition(...)` for all three panes.

[MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:103) increments `editorMountVersion` inside `handleEditorMount`, which closes the previous first-frame timing gap described in task 43.

### 2. Automated evidence now directly covers the original owner concern

[MergeWorkbench.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/MergeWorkbench.test.tsx:82) verifies the mount-after-open scenario that previously failed factually.

The test asserts that, after Monaco mounts, all three panes receive:

1. `createDecorationsCollection(...)`
2. `revealLineInCenter(2)`
3. `setPosition({ lineNumber: 2, column: 1 })`

This is direct focused evidence that first open no longer depends on a manual previous/next click before the current conflict becomes visible.

### 3. Single-conflict `1 / 1` semantics remain intact

[FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:601) still asserts `external-edit-merge-conflict-count` is `1 / 1`.

[FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:602) and [FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:603) still assert both previous/next buttons are disabled in the single-conflict case.

[FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:604) through [FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:606) continue to verify the three merge editors mount through the normal workbench entry path.

This means the fix did not trade away the earlier validated `1 / 1` / disabled-navigation behavior.

### 4. No new reviewed-surface regression was found

The code review surface remains bounded to:

1. [MergeWorkbench.tsx](/H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:34)
2. [MergeWorkbench.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/MergeWorkbench.test.tsx:1)
3. [FileManagerPanel.test.tsx](/H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx:77)

I did not find evidence in this focused scope of:

1. merge workbench navigation state regression
2. current conflict counter regression
3. fallback to the old dialog flow
4. a new shared `CodeEditor` / store / IPC side effect

### 5. Build failure scope remains unchanged and outside this patch

`pnpm build` still fails, but only on the same repository-level missing dependencies already recorded by SEC:

1. `@tiptap/extension-hard-break`
2. `@tabler/icons-react`

No new build error references `MergeWorkbench.tsx`, `MergeWorkbench.test.tsx`, or `FileManagerPanel.test.tsx`.

## Risks

1. Risk remains `Medium` because full frontend build is still blocked by unrelated dependency gaps, so this QA gate relies on focused test evidence rather than a clean whole-frontend build.
2. Within task 47 scope, the owner-reported first-open visibility gap is now directly covered and no residual blocker was found.

## QA Decision

task 47 is `PASS`.

Focused evidence supports that:

1. first open of the three-column merge workbench now reveals the current conflict without manual navigation
2. single-conflict `1 / 1` and disabled navigation semantics remain correct
3. the reviewed fix stays inside the intended merge-workbench boundary
4. build failure remains an unrelated repository dependency issue, not a regression from this repair
