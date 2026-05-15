# task 46 SEC focused audit

- Date: 2026-05-15
- Verdict: PASS
- Risk: Medium

## Scope

- requirement truth:
  - `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md`
- QA factual fail:
  - `docs/plans/2026-05-15-task-43-qa-merge-workbench-factual-check.md`
- SA solution:
  - `docs/superpowers/specs/2026-05-15-task-44-sa-merge-workbench-first-render-positioning-bug.md`
- reviewed DEV change set:
  - `frontend/src/components/terminal/external-edit/MergeWorkbench.tsx`
  - `frontend/src/__tests__/MergeWorkbench.test.tsx`
  - `frontend/src/__tests__/FileManagerPanel.test.tsx`

## Verdict

Previous round's only blocker is closed.

The production-side fix remains within the SA minimum boundary, the first-frame positioning gap is now covered by focused tests that pass both singly and in combination, and the test-only `mountedRef` repair successfully removes the mock-side remount loop that previously made the evidence unreliable.

I did not find a remaining SEC blocker in the reviewed scope.

## Focused Evidence

### 1. Production fix still stays inside the SA minimum boundary

- `frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:34-90`
- only merge-workbench-local state/effects were added:
  - `editorMountVersion`
  - `revealFrameRef`
  - mount-triggered rerun dependencies for decorations/reveal
- no edits landed in:
  - shared `CodeEditor.tsx`
  - Wails IPC bindings
  - external-edit store
  - shared dialog shell

This remains aligned with the SA requirement to keep the bugfix out of shared layers.

### 2. First-frame positioning bug is directly addressed in production code

- `frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:57-90`
- decorations effect now reruns after editor mount via `editorMountVersion`
- reveal effect now reruns after editor mount and uses `requestAnimationFrame` before calling:
  - `revealLineInCenter(...)`
  - `setPosition(...)`
- `handleEditorMount()` increments the rerun signal:
  - `frontend/src/components/terminal/external-edit/MergeWorkbench.tsx:103-109`

This exactly targets the QA/SA timing gap where initial effects could miss because Monaco refs were still null during first render.

### 3. The previous SEC blocker in `FileManagerPanel.test.tsx` is closed

- `frontend/src/__tests__/FileManagerPanel.test.tsx:77-100`
- the `CodeEditor` mock now uses:
  - `mountedRef`
  - stable `editorRef`
  - stable `monacoRef`
- the deferred `onMount(...)` path is still preserved, but it now executes only once per mock instance:
  - `if (!onMount || mountedRef.current) return;`
  - `mountedRef.current = true`

This closes the prior mock-side loop:

1. rerender no longer re-fires `onMount` for the same mock instance
2. `setEditorMountVersion(...)` no longer cascades into repeated fake remounts
3. focused vitest evidence is now reproducible

### 4. Focused merge-workbench verification now passes

Local verification:

- `cd frontend && pnpm test -- --run src/__tests__/MergeWorkbench.test.tsx`
- PASS

Relevant assertions:

- `frontend/src/__tests__/MergeWorkbench.test.tsx:82-126`
- verifies mount-after-open scenario
- verifies all three panes receive:
  - `createDecorationsCollection(...)`
  - `revealLineInCenter(2)`
  - `setPosition({ lineNumber: 2, column: 1 })`

This is the closest direct automated proof that the first-open reveal bug is patched.

### 5. Focused `FileManagerPanel` regression verification now passes

Local verification:

- `cd frontend && pnpm test -- --run src/__tests__/FileManagerPanel.test.tsx`
- PASS
- result:
  - `1 passed (1)` file
  - `24 passed (24)` tests

This closes the prior evidence-integrity gap from the SEC FAIL round.

### 6. Combined focused verification also passes

Local verification:

- `cd frontend && pnpm test -- --run src/__tests__/MergeWorkbench.test.tsx src/__tests__/FileManagerPanel.test.tsx`
- PASS
- result:
  - `2 passed (2)` files
  - `25 passed (25)` tests

This matters because the prior blocker only surfaced when the files were run together. That interaction is now clean.

### 7. Remaining build failure is still outside this task's change surface

Local verification:

- `cd frontend && pnpm build`
- FAIL

Observed errors:

1. `src/components/ai/AIChatInput.tsx`: missing `@tiptap/extension-hard-break`
2. `src/components/layout/TopBar.tsx`: missing `@tabler/icons-react`

These errors are unchanged from the previous SEC round, remain outside the 3-file reviewed patch set, and do not indicate a new regression from the merge-workbench fix.

## Residual Risk

- Risk remains Medium rather than Low because the full frontend build is still blocked by repository-level dependency gaps, so this gate relies on focused test evidence rather than a clean whole-frontend build.
- Within the reviewed task 46 scope, that residual risk is acceptable:
  - the production fix is bounded
  - the targeted first-open scenario is covered
  - the earlier mock-side evidence blocker is closed

## QA Gate Note

- SEC allows QA task 47 to resume on the current reviewed patch set.
