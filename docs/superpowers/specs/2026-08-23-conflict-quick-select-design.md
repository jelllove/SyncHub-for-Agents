# Conflict Quick Select Design

## Goal

When many conflicts exist, users can quickly set batch choices with one click instead of selecting each item one-by-one.

## Scope

- In-scope:
  - Add three quick actions in the conflict panel:
    - Use local for all
    - Use remote for all
    - Clear all
  - Keep existing "Apply all and synchronize" flow and payload contract.
  - Add targeted unit tests for the new quick actions.
- Out-of-scope:
  - Persisting default conflict strategy in settings.
  - Grouped bulk actions by resource type/path.
  - Any backend API/schema change.

## User Experience

In the "Resolve synchronized conflicts" panel, users get a top-level quick-action row:

1. **Use local for all**
   - Sets every conflict selection to `local`.
2. **Use remote for all**
   - Sets every conflict selection to `remote`.
3. **Clear all**
   - Clears all selections.

Behavior details:

- Existing per-item buttons remain unchanged and continue to work after quick select.
- Existing completion rule remains unchanged:
  - Apply is enabled only when every conflict has a valid selection.
  - `merged` still requires non-empty content.
- Quick-action buttons are disabled when `busy=true`.

## Architecture / Component Design

### Frontend component changes

Modify only:

- `frontend/src/resources/ConflictPanel.tsx`
- `frontend/src/resources/ConflictPanel.test.tsx`

#### `ConflictPanel.tsx`

Add small helper handlers:

- `setAllChoice(choice: 'local' | 'remote')`
  - Builds next `selections` map from `conflicts`.
  - For each conflict id:
    - `choice` set to the selected value.
    - `content` set to `''`.
- `clearAllChoices()`
  - Resets selections to `{}`.

Render quick-action buttons near panel header/body top:

- "Use local for all"
- "Use remote for all"
- "Clear all"

Each button:

- Calls corresponding handler.
- Uses existing button styles (`secondary`/`text-button`) and `busy` disabled state.

No backend call is made by quick actions. Backend call still occurs only on final apply.

## Data Flow

1. User clicks quick action.
2. Local `selections` state updates in one state transition.
3. `applySelections` derived value recomputes from `conflicts + selections`.
4. Existing `complete` rule recalculates and enables/disables Apply button.
5. User clicks Apply → existing `applyBatch(applySelections)` executes unchanged.

## Error Handling

- No new backend or network failure paths introduced.
- Invalid/partial selection behavior is still governed by existing `complete` guard.
- `busy=true` remains the guard for preventing repeated operations.

## Testing Plan

Update `frontend/src/resources/ConflictPanel.test.tsx` with focused cases:

1. **Bulk local**
   - Click "Use local for all" → Apply enabled.
   - Click Apply → `applyBatch` payload all `choice: 'local'`.
2. **Bulk remote**
   - Click "Use remote for all" → Apply enabled.
   - Click Apply → `applyBatch` payload all `choice: 'remote'`.
3. **Clear all**
   - After any bulk action, click "Clear all" → Apply disabled.
4. **Busy state**
   - Render with `busy=true` → three quick-action buttons disabled.

## Rollout and Compatibility

- Fully backward compatible.
- No persistence/schema/version changes.
- No migration required.

