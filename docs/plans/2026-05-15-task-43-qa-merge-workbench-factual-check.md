# task 43 QA focused factual verification

- Date: 2026-05-15
- Scope: factual verification only, no final QA gate verdict
- Working branch: `feat/remote-file-external-editing-clean`
- Verified HEAD: `311294d54467bdc21abcbe50a1acc38e534cbf18`
- Authoritative requirement truth:
  - `docs/plans/2026-05-15-task-43-po-merge-workbench-visibility-requirement.md`
- Risk: High

## Factual Result

- Result: FAIL
- Stable reproduction on current branch:
  - deterministic automated reproduction: NOT established
  - factual branch status against requirement truth: FAIL
  - reason: the current implementation does not guarantee first-open reveal/highlight after Monaco mount, so owner-observed failure remains credible and unresolved on current branch
- Classification:
  - current branch contains current-conflict highlight logic
  - current branch contains first-conflict auto-positioning logic
  - but current implementation has a credible **first-frame render / positioning timing gap**
- Best-fit interpretation:
  - this is **not** a “single-conflict `1 / 1` state is unknown” problem
  - this is **not** a “navigation buttons missing” problem
  - this is most likely a **首帧渲染/定位时机问题**
  - owner’s latest observation is consistent with the implementation shape: first open may miss the initial reveal/current-highlight application until a later interaction re-triggers the effect path

## Minimal Repro

1. Prepare an external-edit conflict document that yields exactly one merge conflict block.
2. Open File Manager and enter the unified pending dialog.
3. Click `合并` to open the three-column merge workbench for the first time.
4. On first paint, observe whether the conflict block is already visible and obviously highlighted.
5. If the conflict point is not obvious, trigger a navigation-related refresh interaction:
   - click `上一处冲突` / `下一处冲突` when enabled in multi-conflict cases
   - or otherwise cause a later focus/render refresh in runtime
6. Observe whether the conflict location/highlight becomes visible only after that later interaction.

For the single-conflict case specifically:

1. open a merge workbench that shows `1 / 1`
2. observe that the UI state already knows there is one current conflict
3. the remaining question is whether first-frame reveal/highlight is visually applied before any further interaction
4. note that `1 / 1` plus disabled navigation can still coexist with a first-frame visibility defect, because state recognition and first-open render timing are separate concerns

## Visible Evidence

### 1. Focused test evidence

- Command:
  - `cd frontend && pnpm test -- --run src/__tests__/FileManagerPanel.test.tsx`
- Result:
  - PASS (`1 file / 24 tests`)

Relevant assertions in [FileManagerPanel.test.tsx](H:/github/opskat/opskat/frontend/src/__tests__/FileManagerPanel.test.tsx):

- merge workbench opens:
  - line `588`: `external-edit-merge-workbench`
- single-conflict counter is explicitly `1 / 1`:
  - line `597`
- previous / next buttons are disabled in the single-conflict case:
  - lines `598-599`

This proves the current implementation already recognizes the single-conflict state correctly at the view-model level.
It does **not** prove that the first-frame Monaco render has already applied reveal/highlight after editor mount completes.

### 2. Current-conflict highlight exists in code

Relevant implementation in [merge-decorations.ts](H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/merge-decorations.ts):

- active block line class:
  - line `20`: `external-edit-merge-line-current`
- active block gutter class:
  - line `33`: `external-edit-merge-gutter-current`
- decorations are applied through Monaco:
  - lines `45-72`

Relevant styling in [globals.css](H:/github/opskat/opskat/frontend/src/styles/globals.css):

- current line emphasis:
  - line `255`: `.external-edit-merge-line-current`
  - lines `256-260`: stronger background + dual inset bars + explicit outline
- current gutter emphasis:
  - line `275`: `.external-edit-merge-gutter-current`
  - lines `276-277`: dedicated current gutter color + outline

Conclusion:
- this is **not** an absence-of-highlight implementation bug

### 3. First-conflict auto-positioning logic exists in code

Relevant implementation in [MergeWorkbench.tsx](H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx):

- initial state on merge open:
  - lines `48-53`
  - `setActiveBlockIndex(0)`
  - `setNavigationToken((token) => token + 1)`
- auto-reveal effect:
  - lines `65-76`
  - uses current block computed from `activeBlockIndex`
  - calls:
    - `editor.revealLineInCenter(lineNumber)` line `73`
    - `editor.setPosition({ lineNumber, column: 1 })` line `74`

Conclusion:
- this is **not** an absence-of-navigation-logic implementation bug

### 4. Why owner’s observed failure is still plausible on current code

Relevant implementation in [CodeEditor.tsx](H:/github/opskat/opskat/frontend/src/components/CodeEditor.tsx) and [MergeWorkbench.tsx](H:/github/opskat/opskat/frontend/src/components/terminal/external-edit/MergeWorkbench.tsx):

- `CodeEditor` mount path:
  - line `122`: `onMount?.(editor, monaco)`
  - line `198`: editor mount callback is invoked by Monaco after editor creation
- merge workbench mount handler:
  - `MergeWorkbench.tsx` lines `89-93`
  - only stores `{ editor, monaco }` into refs
  - it does **not** trigger a follow-up re-run of the reveal/highlight effects
- highlight effect dependencies:
  - lines `55-63`: depends on `[activeBlockIndex, conflictBlocks]`
- reveal effect dependencies:
  - lines `65-76`: depends on `[activeBlockIndex, conflictBlocks, navigationToken]`
- both effects early-return per pane when editor is still null:
  - line `58`: `if (!editor || !monaco) return;`
  - line `70`: `if (!editor) return;`

This means:

1. the initial `setActiveBlockIndex(0)` / `setNavigationToken(...)` can happen before all three Monaco editors finish mounting
2. if the effects run while editor refs are still null, the first-frame reveal/highlight work is skipped
3. when editor mount later fills the refs, there is no automatic dependency change that guarantees those effects re-run
4. a later interaction can then re-trigger the relevant path and make the conflict point/highlight become visible

This matches owner’s latest observation far better than the previous “likely expectation mismatch” interpretation.

It also explains why the previous single-conflict `1 / 1` / disabled-navigation conclusion can coexist with the new owner observation:

1. the model layer can already know there is exactly one conflict
2. the navigation buttons can already be disabled correctly
3. but the first-frame reveal/highlight can still miss if Monaco mount completes after the initial effects have already run

## Conclusion

Current factual verification is `FAIL`.

Risk is `High` because this affects the core conflict-resolution entry path, is directly owner-visible on first open, and in single-conflict scenarios may leave the user without an obvious visible conflict anchor on initial render.

The previous “single conflict state is present and highlight logic exists” findings are still true, but they do **not** clear the owner concern. Updated reading of the implementation shows a credible first-frame timing gap:

1. the model state knows `1 / 1`
2. the current-block highlight classes exist
3. the reveal logic exists
4. but neither reveal nor current decoration is guaranteed to re-run after editor mount completes

Therefore the owner-reported symptom is consistent with the current branch:

- first open of the three-column merge workbench may fail to visibly land on the conflict block
- later interaction can make the conflict point become visible

## Suggested PM Framing

- factual status: FAIL
- risk: High
- issue type:
  - first-frame render / positioning timing bug
- not the right framing:
  - not “bug unconfirmed”
  - not “single-conflict state missing”
  - not “no highlight implementation”
- suggested next handling:
  - yes, this should be upgraded to SA / DEV bugfix handling
  - requirement wording should explicitly cover:
    1. on first open of merge workbench, the current conflict block must already be visible without any extra interaction
    2. the current conflict block highlight must already be visually obvious on first paint
