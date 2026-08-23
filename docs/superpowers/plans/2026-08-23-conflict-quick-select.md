# Conflict Quick Select Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fast conflict batch-selection actions (use local for all, use remote for all, clear all) in the desktop conflict panel.

**Architecture:** Keep all logic in the existing frontend conflict UI state (`ConflictPanel`) and reuse the existing batch apply payload generation. No backend/API/model changes are needed; the feature is a pure UI state transformation. Validation remains governed by existing `complete` checks, so merged-content requirements and apply gating remain intact.

**Tech Stack:** React + TypeScript, Vitest + Testing Library, existing Wails desktop frontend

---

## File Structure

- **Modify:** `frontend/src/resources/ConflictPanel.tsx`
  - Add quick-action handlers and action buttons.
- **Modify:** `frontend/src/resources/ConflictPanel.test.tsx`
  - Add focused tests for local-all, remote-all, clear-all, and busy-state disable behavior.
- **No backend changes**
  - Existing `applyBatch` contract remains unchanged.

### Task 1: Add failing tests for quick actions

**Files:**
- Modify: `frontend/src/resources/ConflictPanel.test.tsx`
- Test: `frontend/src/resources/ConflictPanel.test.tsx`

- [ ] **Step 1: Add failing test for “Use local for all”**

```tsx
it('applies local choice to all conflicts in one click', async () => {
  const applyBatch = vi.fn().mockResolvedValue(undefined)
  const user = userEvent.setup()
  render(
    <ConflictPanel
      conflicts={conflicts}
      resolution={null}
      busy={false}
      applyBatch={applyBatch}
      retryBatch={vi.fn().mockResolvedValue(undefined)}
    />,
  )

  await user.click(screen.getByRole('button', { name: 'Use local for all' }))
  const applyButton = screen.getByRole('button', { name: 'Apply all and synchronize' })
  expect(applyButton).toBeEnabled()
  await user.click(applyButton)

  expect(applyBatch).toHaveBeenCalledWith([
    { id: 'conflict-1', revision: 'rev-1', choice: 'local', content: '' },
    { id: 'conflict-2', revision: 'rev-2', choice: 'local', content: '' },
  ])
})
```

- [ ] **Step 2: Add failing test for “Use remote for all”**

```tsx
it('applies remote choice to all conflicts in one click', async () => {
  const applyBatch = vi.fn().mockResolvedValue(undefined)
  const user = userEvent.setup()
  render(
    <ConflictPanel
      conflicts={conflicts}
      resolution={null}
      busy={false}
      applyBatch={applyBatch}
      retryBatch={vi.fn().mockResolvedValue(undefined)}
    />,
  )

  await user.click(screen.getByRole('button', { name: 'Use remote for all' }))
  await user.click(screen.getByRole('button', { name: 'Apply all and synchronize' }))

  expect(applyBatch).toHaveBeenCalledWith([
    { id: 'conflict-1', revision: 'rev-1', choice: 'remote', content: '' },
    { id: 'conflict-2', revision: 'rev-2', choice: 'remote', content: '' },
  ])
})
```

- [ ] **Step 3: Add failing test for “Clear all”**

```tsx
it('clears all batch selections and disables apply', async () => {
  const user = userEvent.setup()
  render(
    <ConflictPanel
      conflicts={conflicts}
      resolution={null}
      busy={false}
      applyBatch={vi.fn().mockResolvedValue(undefined)}
      retryBatch={vi.fn().mockResolvedValue(undefined)}
    />,
  )

  const applyButton = screen.getByRole('button', { name: 'Apply all and synchronize' })
  await user.click(screen.getByRole('button', { name: 'Use local for all' }))
  expect(applyButton).toBeEnabled()

  await user.click(screen.getByRole('button', { name: 'Clear all' }))
  expect(applyButton).toBeDisabled()
})
```

- [ ] **Step 4: Add failing test for busy-state disable**

```tsx
it('disables quick-action buttons while busy', () => {
  render(
    <ConflictPanel
      conflicts={conflicts}
      resolution={null}
      busy
      applyBatch={vi.fn().mockResolvedValue(undefined)}
      retryBatch={vi.fn().mockResolvedValue(undefined)}
    />,
  )

  expect(screen.getByRole('button', { name: 'Use local for all' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Use remote for all' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Clear all' })).toBeDisabled()
})
```

- [ ] **Step 5: Run test file and confirm failure**

Run:

```bash
npm --prefix frontend test -- ConflictPanel.test.tsx
```

Expected:
- FAIL with missing quick-action buttons and/or unmet payload expectations.

- [ ] **Step 6: Commit failing tests**

```bash
git add frontend/src/resources/ConflictPanel.test.tsx
git commit -m "test: add failing conflict quick-select coverage"
```

### Task 2: Implement quick actions in ConflictPanel

**Files:**
- Modify: `frontend/src/resources/ConflictPanel.tsx`
- Test: `frontend/src/resources/ConflictPanel.test.tsx`

- [ ] **Step 1: Add `setAllChoice` and `clearAllChoices` helpers**

```tsx
const setAllChoice = (choice: Exclude<SelectionChoice, 'merged'>) => {
  setSelections(() => Object.fromEntries(
    conflicts.map((conflict) => [conflict.id, { choice, content: '' }]),
  ))
}

const clearAllChoices = () => {
  setSelections({})
}
```

- [ ] **Step 2: Render top-level quick-action row**

```tsx
<div className="inline-actions">
  <button
    className="secondary"
    disabled={busy || conflicts.length === 0}
    onClick={() => setAllChoice('local')}
  >
    Use local for all
  </button>
  <button
    className="secondary"
    disabled={busy || conflicts.length === 0}
    onClick={() => setAllChoice('remote')}
  >
    Use remote for all
  </button>
  <button
    className="text-button"
    disabled={busy || conflicts.length === 0}
    onClick={clearAllChoices}
  >
    Clear all
  </button>
</div>
```

- [ ] **Step 3: Verify no behavior regression in completion logic**

Keep existing logic unchanged:

```tsx
const complete = conflicts.length > 0 && applySelections.every((selection) => {
  if (selection.choice === '') return false
  if (selection.choice === 'merged') return selection.content.trim().length > 0
  return true
})
```

- [ ] **Step 4: Run targeted tests**

Run:

```bash
npm --prefix frontend test -- ConflictPanel.test.tsx
```

Expected:
- PASS for existing tests and the new quick-action tests.

- [ ] **Step 5: Commit implementation**

```bash
git add frontend/src/resources/ConflictPanel.tsx frontend/src/resources/ConflictPanel.test.tsx
git commit -m "feat: add conflict quick-select actions"
```

### Task 3: Regression verification

**Files:**
- Modify: none expected
- Test: existing frontend suite

- [ ] **Step 1: Run full frontend tests**

Run:

```bash
npm --prefix frontend test
```

Expected:
- PASS with no regressions in app/settings/install/conflict panels.

- [ ] **Step 2: Run frontend build**

Run:

```bash
npm --prefix frontend run build
```

Expected:
- TypeScript compile and Vite build succeed.

- [ ] **Step 3: Commit verification checkpoint (optional)**

```bash
git commit --allow-empty -m "chore: verify frontend regression checks for conflict quick-select"
```

## Self-Review

1. **Spec coverage:**  
   - local/remote/clear all quick actions: Task 2  
   - keep existing apply contract: Task 2 Step 3  
   - test coverage for all new actions + busy state: Task 1  
2. **Placeholder scan:** No TBD/TODO or unresolved placeholders.
3. **Type consistency:** Uses existing `SelectionChoice`, `ConflictSelections`, and existing `applyBatch` payload shape.

