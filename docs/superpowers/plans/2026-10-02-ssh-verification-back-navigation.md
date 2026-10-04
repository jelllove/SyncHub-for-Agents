# SSH Verification Back Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let onboarding users return to the prefilled repository URL form after SSH verification fails and retry with a corrected address.

**Architecture:** Keep wizard progress authoritative in the Go onboarding service by adding a `ReturnToRepository` state transition and exposing it through the Wails service. The React wizard shows the edit action only after an SSH verification error, invokes that transition, and hydrates the repository input from backend state. Existing URL parsing and verification APIs remain unchanged.

**Tech Stack:** Go, Wails v3 bindings, React 19, TypeScript, Vitest, Testing Library

---

## File Structure

- Modify `internal/onboarding/service.go`: own the return-to-repository state
  transition and transient authentication cleanup.
- Modify `internal/onboarding/service_test.go`: verify retained URL, cleared
  transient state, and OAuth cancellation.
- Modify `internal/desktop/wails.go`: expose the transition and emit the updated
  onboarding state.
- Create `frontend/src/onboarding/Onboarding.test.tsx`: exercise the failed SSH
  verification and edit/retry user flow.
- Modify `frontend/src/onboarding/Onboarding.tsx`: render the edit action and
  prefill the repository form from backend state.
- Regenerate `frontend/bindings`: add the generated TypeScript wrapper for the
  new Wails method; do not edit generated files manually.

### Task 1: Add the backend onboarding transition

**Files:**
- Modify: `internal/onboarding/service_test.go`
- Modify: `internal/onboarding/service.go`

- [ ] **Step 1: Write the failing service test**

Add this test to `internal/onboarding/service_test.go`:

```go
func TestReturnToRepositoryRetainsURLAndClearsTransientState(t *testing.T) {
	oauth := &fakeOAuth{}
	service := New(Dependencies{OAuth: oauth})
	if err := service.SetRepository("git@github.com:acme/wrong.git"); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.state.Step = Verification
	service.state.UserCode = "ABCD"
	service.state.VerificationURI = "https://github.com/login/device"
	service.state.Message = "repository access failed"
	service.mu.Unlock()

	state := service.ReturnToRepository()

	if state.Step != Repository ||
		state.RepositoryURL != "git@github.com:acme/wrong.git" ||
		state.UserCode != "" ||
		state.VerificationURI != "" ||
		state.Message != "" {
		t.Fatalf("state = %#v", state)
	}
	if !oauth.cancelled {
		t.Fatal("authentication flow was not cancelled")
	}
}
```

- [ ] **Step 2: Run the test and confirm the missing method failure**

Run:

```powershell
go test ./internal/onboarding -run TestReturnToRepository -count=1
```

Expected: compilation fails because `ReturnToRepository` is undefined.

- [ ] **Step 3: Implement the state transition**

Add this method after `SetRepository` in `internal/onboarding/service.go`:

```go
func (s *Service) ReturnToRepository() State {
	s.Cancel()
	s.mu.Lock()
	s.state.Step = Repository
	s.state.Message = ""
	state := cloneState(s.state)
	s.mu.Unlock()
	return state
}
```

`Cancel` already clears `UserCode`, `VerificationURI`, `loginCtx`, and
`loginCancel`, and cancels the OAuth provider. The new method deliberately keeps
`RepositoryURL` and `AuthMode` so the user can edit the current value.

- [ ] **Step 4: Run the focused backend test**

Run:

```powershell
go test ./internal/onboarding -run 'TestReturnToRepository|TestCancel' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the backend transition**

```powershell
git add internal/onboarding/service.go internal/onboarding/service_test.go
git commit -m "feat: allow returning to onboarding repository step" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 2: Expose the transition through Wails

**Files:**
- Modify: `internal/desktop/wails.go`
- Regenerate: `frontend/bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice.ts`

- [ ] **Step 1: Add the Wails service method**

Add this method after `SetRepository` in `internal/desktop/wails.go`:

```go
func (s *WailsService) ReturnToRepository() onboarding.State {
	state := s.onboarding.ReturnToRepository()
	s.emitOnboarding()
	return state
}
```

- [ ] **Step 2: Regenerate the TypeScript bindings**

Run:

```powershell
wails3 generate bindings -f '-buildvcs=false -gcflags=all="-l"' -clean=true -ts -i
```

Expected: the generated
`frontend/bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice.ts`
exports `ReturnToRepository`.

- [ ] **Step 3: Verify Go compilation**

Run:

```powershell
go test ./internal/desktop ./internal/onboarding -run 'TestReturnToRepository|TestWails' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit the service surface**

```powershell
git add internal/desktop/wails.go frontend/bindings
git commit -m "feat: expose onboarding repository return action" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 3: Add the failed-SSH edit flow to the React wizard

**Files:**
- Create: `frontend/src/onboarding/Onboarding.test.tsx`
- Modify: `frontend/src/onboarding/Onboarding.tsx`

- [ ] **Step 1: Write the failing frontend test**

Create `frontend/src/onboarding/Onboarding.test.tsx` with a hoisted API mock.
The test should use the generated `Step` enum and the exact SSH states below:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Step, type State } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/onboarding/models'
import Onboarding from './Onboarding'

const api = vi.hoisted(() => ({
  state: vi.fn(),
  setRepository: vi.fn(),
  verifySSH: vi.fn(),
  returnToRepository: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: vi.fn() },
  Events: { On: vi.fn(() => vi.fn()) },
}))

vi.mock('../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  CancelOnboarding: vi.fn(),
  CompleteOnboarding: vi.fn(),
  OnboardingState: () => api.state(),
  ReturnToRepository: () => api.returnToRepository(),
  SetRepository: (value: string) => api.setRepository(value),
  StartGitHubLogin: vi.fn(),
  VerifySSH: () => api.verifySSH(),
  WaitGitHubLogin: vi.fn(),
}))

function sshState(step: Step): State {
  return {
    step,
    repositoryUrl: 'git@github.com:acme/wrong.git',
    authMode: 'ssh',
    userCode: '',
    verificationUri: '',
    message: '',
    agents: [],
  }
}

describe('Onboarding SSH verification', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.state.mockResolvedValue(sshState(Step.Authentication))
    api.verifySSH.mockRejectedValue(new Error('repository access failed'))
    api.returnToRepository.mockResolvedValue(sshState(Step.Repository))
    api.setRepository.mockResolvedValue(undefined)
  })

  it('returns to a prefilled repository form after verification fails', async () => {
    const user = userEvent.setup()
    render(<Onboarding complete={vi.fn()} />)

    await user.click(await screen.findByRole('button', { name: 'Verify SSH access' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('repository access failed')

    await user.click(screen.getByRole('button', { name: 'Edit repository URL' }))

    const input = await screen.findByRole('textbox', { name: 'GitHub repository URL' })
    expect(input).toHaveValue('git@github.com:acme/wrong.git')
    await user.clear(input)
    await user.type(input, 'git@github.com:acme/correct.git')
    await user.click(screen.getByRole('button', { name: 'Continue' }))

    expect(api.setRepository).toHaveBeenCalledWith('git@github.com:acme/correct.git')
  })
})
```

- [ ] **Step 2: Run the frontend test and confirm failure**

Run:

```powershell
npm --prefix frontend test -- Onboarding.test.tsx
```

Expected: FAIL because no `Edit repository URL` action is rendered.

- [ ] **Step 3: Hydrate the repository input from backend state**

In `frontend/src/onboarding/Onboarding.tsx`, add a helper inside the component:

```tsx
const applyState = (next: WizardState) => {
  setState(next)
  if (next.step === Step.Repository) {
    setRepository(next.repositoryUrl)
  }
  if (next.step === Step.Agents) {
    setEnabled(Object.fromEntries(next.agents.map((agent) => [agent.name, agent.enabled])))
  }
}
```

Use `applyState(normalize(value))` for initial `OnboardingState`, onboarding
events, and successful async operations. This ensures a backend-owned
`Repository` step remains prefilled after page refresh or an emitted event.

- [ ] **Step 4: Implement the edit action**

Import `ReturnToRepository` from the generated Wails service and add:

```tsx
const editRepository = async () => {
  await run(async () => {
    applyState(normalize(await ReturnToRepository()))
  })
}
```

After the SSH verification button, render the secondary action only when the
current attempt has failed:

```tsx
{error && (
  <button
    className="secondary wide"
    disabled={busy}
    onClick={() => void editRepository()}
    type="button"
  >
    Edit repository URL
  </button>
)}
```

Because `run` clears `error` before invoking the transition, the stale SSH error
does not remain on the repository form.

- [ ] **Step 5: Run the focused frontend test**

Run:

```powershell
npm --prefix frontend test -- Onboarding.test.tsx
```

Expected: PASS.

- [ ] **Step 6: Run frontend typechecking and lint**

Run:

```powershell
npm --prefix frontend run typecheck
npm --prefix frontend run lint
```

Expected: both commands PASS.

- [ ] **Step 7: Commit the wizard behavior**

```powershell
git add frontend/src/onboarding/Onboarding.tsx frontend/src/onboarding/Onboarding.test.tsx
git commit -m "fix: allow editing repository after SSH failure" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 4: Validate and relaunch the isolated app

**Files:**
- Verify: all files changed by Tasks 1-3

- [ ] **Step 1: Run targeted backend and frontend tests**

```powershell
go test ./internal/onboarding ./internal/desktop -run 'TestReturnToRepository|TestWails' -count=1
npm --prefix frontend test -- Onboarding.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run repository checks**

```powershell
node scripts/dev.mjs check
```

Expected: `check: passed`.

- [ ] **Step 3: Run full source-bound verification**

```powershell
node scripts/dev.mjs verify
```

Expected: frontend build, repository checks, Go tests, lint tests, and frontend
tests pass, and the source snapshot remains stable.

- [ ] **Step 4: Relaunch with the isolated profile**

Stop the existing attached `synchub-desktop` and `synchub-app` sessions, then
launch Wails with:

```powershell
$profileRoot = 'C:\Users\qinqiangxu\.copilot\session-state\a6d31900-9fc4-4fc8-98c3-c63e8eb79bb0\files\synchub-test-profile'
$env:USERPROFILE = $profileRoot
$env:HOME = $profileRoot
$env:APPDATA = Join-Path $profileRoot 'AppData\Roaming'
$env:LOCALAPPDATA = Join-Path $profileRoot 'AppData\Local'
wails3 dev -config .\build\config.yml -port 9245
```

Expected: the frontend responds on port 9245, WebView2 reports successful
environment creation, and the onboarding window shows the corrected behavior
without reading the real user profile.
