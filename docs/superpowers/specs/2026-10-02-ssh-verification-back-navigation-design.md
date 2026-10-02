# SSH Verification Back Navigation

## Goal

Allow a user who entered an incorrect SSH Git repository URL during onboarding
to return to the repository step, edit the URL, and retry verification without
restarting the application.

## Scope

- Show an **Edit repository URL** action after SSH verification fails.
- Keep the verification error visible until the user chooses to edit.
- Move the backend onboarding state back to the repository step when the action
  is selected.
- Prefill the repository form with the previously submitted URL.
- Submit the edited URL through the existing repository validation and SSH
  verification flow.

HTTPS device-flow behavior and post-onboarding repository settings are unchanged.

## Design

The onboarding service remains the source of truth for wizard progress. A new
`ReturnToRepository` operation will cancel any active authentication flow, clear
transient authentication fields and messages, and set the step to `Repository`.
It will retain the repository URL so the frontend can prefill the edit form.

The onboarding UI will show **Edit repository URL** only when an SSH verification
attempt has failed. Clicking it calls `ReturnToRepository`, updates the rendered
state from the returned backend state, copies the retained URL into the input,
and clears the previous inline error. Submitting the form continues to use the
existing `SetRepository` operation, so URL parsing and protocol selection remain
centralized.

## Error Handling

- If returning to the repository step fails, the user stays on verification and
  sees the operation error.
- Invalid edited URLs continue to be rejected by `SetRepository`.
- A second SSH failure again shows the edit action and preserves the new error.

## Validation

- Backend tests verify the state transition, retained URL, cleared transient
  fields, and cancellation of an active authentication flow.
- Frontend tests verify that the edit action appears after SSH failure, returns
  to a prefilled repository form, and allows resubmission.
- Repository checks and the full source-bound verification command must pass.

## Acceptance Criteria

1. Entering an inaccessible or incorrect SSH repository URL produces the normal
   verification error and an **Edit repository URL** action.
2. Selecting the action returns to step 1 with the previous URL prefilled.
3. The user can change the URL, continue, and run SSH verification again.
4. Refreshing or backend state events do not return the UI to the failed
   verification step after the explicit back action.
5. Successful SSH and HTTPS onboarding behavior remains unchanged.
