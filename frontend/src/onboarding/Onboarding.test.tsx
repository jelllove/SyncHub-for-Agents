import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  Step,
  type State,
} from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/onboarding/models'
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

  it('shows the supported SSH alias URL form', async () => {
    api.state.mockResolvedValue(sshState(Step.Repository))

    render(<Onboarding complete={vi.fn()} />)

    expect(await screen.findByText('git@github-work:you/sync.git')).toBeVisible()
  })
})
