import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  ResourcePreview,
  Snapshot,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import App from './App'

const api = vi.hoisted(() => ({
  needsOnboarding: vi.fn(),
  resourcePreview: vi.fn(),
  snapshot: vi.fn(),
  startAtLogin: vi.fn(),
  updateStatus: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: vi.fn(),
  },
  Events: {
    On: vi.fn(() => vi.fn()),
  },
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  ApproveInstallPlan: vi.fn(),
  NeedsOnboarding: () => api.needsOnboarding(),
  Pause: vi.fn(),
  PreviewCustomResource: vi.fn(),
  QueueConflictBatch: vi.fn(),
  RetryConflictBatch: vi.fn(),
  RetryInstallPlan: vi.fn(),
  ResourcePreview: () => api.resourcePreview(),
  Resume: vi.fn(),
  SaveSettings: vi.fn(),
  SetStartAtLogin: vi.fn(),
  Snapshot: () => api.snapshot(),
  StartAtLogin: () => api.startAtLogin(),
  TriggerSync: vi.fn(),
  UpdateStatus: () => api.updateStatus(),
  CheckForUpdates: vi.fn(),
  SetAutomaticUpdates: vi.fn(),
  RestartToUpdate: vi.fn(),
}))

function configuredSnapshot(): Snapshot {
  return {
    configured: true,
    state: 'idle',
    repositoryUrl: 'git@github.com:owner/repo.git',
    platform: 'windows',
    intervalMinutes: 10,
    trashGraceDays: 30,
    agents: [],
    lastSync: '0001-01-01T00:00:00Z',
    nextSync: '0001-01-01T00:00:00Z',
    pendingActions: 0,
    blockedFiles: 0,
    lastError: '',
    repoPath: 'C:/Users/test/.synchub/repo',
    firstSyncRequired: false,
    syncDiagnostic: null,
    progress: {
      stage: '',
      label: '',
      percentage: 0,
      completedActions: 0,
      totalActions: 0,
      blockedFiles: 0,
      pushed: false,
      restored: 0,
      reinstalled: 0,
      skipped: 0,
      conflicts: 0,
      pendingInstalls: 0,
      needsAttention: false,
    },
    preview: {
      generatedAt: '0001-01-01T00:00:00Z',
      resources: null,
      files: 0,
      bytes: 0,
      excludedFiles: 0,
      excludedBytes: 0,
      issues: null,
    },
    customResources: null,
    pendingInstallPlan: null,
    conflicts: null,
  }
}

function deferredPreview() {
  let resolve!: (preview: ResourcePreview) => void
  const promise = new Promise<ResourcePreview>((resolvePromise) => {
    resolve = resolvePromise
  }) as Promise<ResourcePreview> & { cancel: ReturnType<typeof vi.fn> }
  promise.cancel = vi.fn()
  return { promise, resolve }
}

describe('App settings preview', () => {
  beforeEach(() => {
    api.needsOnboarding.mockReset()
    api.resourcePreview.mockReset()
    api.snapshot.mockReset()
    api.startAtLogin.mockReset()
    api.updateStatus.mockReset()
    api.needsOnboarding.mockResolvedValue(false)
    api.snapshot.mockResolvedValue(configuredSnapshot())
    api.startAtLogin.mockResolvedValue(false)
    api.updateStatus.mockResolvedValue({
      currentVersion: 'v1.0.0',
      latestVersion: '',
      phase: 'idle',
      automatic: true,
      supported: true,
      releaseURL: '',
      error: '',
      lastChecked: '',
    })
  })

  it('opens settings before preview completion and cancels the request on close', async () => {
    const preview = deferredPreview()
    api.resourcePreview.mockReturnValue(preview.promise)
    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Open settings' }))

    expect(screen.getByRole('dialog', { name: 'Sync settings' })).toBeInTheDocument()
    expect(screen.getByText('No preview generated yet')).toBeInTheDocument()
    expect(api.resourcePreview).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: 'Close settings' }))
    expect(preview.promise.cancel).toHaveBeenCalledTimes(1)
  })

  it('renders sync fix panel for git rebase dirty worktree', async () => {
    api.resourcePreview.mockResolvedValue({
      generatedAt: '0001-01-01T00:00:00Z',
      resources: null,
      files: 0,
      bytes: 0,
      excludedFiles: 0,
      excludedBytes: 0,
      issues: null,
    })
    api.snapshot.mockResolvedValue({
      ...configuredSnapshot(),
      syncDiagnostic: {
        code: 'git-rebase-dirty-worktree',
        summary: 'Local repository has unstaged changes',
        repoPath: 'C:/Users/test/.synchub/repo',
        steps: [{ title: 'Stash then pull', command: 'git stash ...' }],
      },
    })
    render(<App />)
    expect(await screen.findByText('Fix sync issue')).toBeInTheDocument()
    expect(screen.getByText('C:/Users/test/.synchub/repo')).toBeInTheDocument()
  })
})
