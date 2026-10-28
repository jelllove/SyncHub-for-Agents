import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Snapshot } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { SettingsPanel } from './SettingsPanel'
import { normalizeSnapshot } from './desktopState'

const startAtLogin = vi.fn()
const updates = vi.hoisted(() => ({
  status: vi.fn(),
  check: vi.fn(),
  automatic: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: vi.fn() },
  Events: { On: vi.fn(() => vi.fn()) },
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  PreviewCustomResource: vi.fn(),
  StartAtLogin: () => startAtLogin(),
  UpdateStatus: () => updates.status(),
  CheckForUpdates: () => updates.check(),
  SetAutomaticUpdates: (enabled: boolean) => updates.automatic(enabled),
  RestartToUpdate: vi.fn(),
}))

function updateStatus(automatic = true) {
  return {
    currentVersion: 'v1.0.0',
    latestVersion: '',
    phase: 'idle',
    automatic,
    supported: true,
    releaseURL: '',
    error: '',
    lastChecked: '',
  }
}

function snapshot(generatedAt: string, files: number): Snapshot {
  return {
    configured: true,
    state: 'idle',
    repositoryUrl: 'git@github.com:owner/repo.git',
    platform: 'windows',
    intervalMinutes: 10,
    trashGraceDays: 30,
    agents: [{
      name: 'claude',
      enabled: true,
      exclude: null,
      resources: null,
    }],
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
      generatedAt,
      resources: null,
      files,
      bytes: files * 10,
      excludedFiles: 0,
      excludedBytes: 0,
      issues: null,
    },
    customResources: null,
    pendingInstallPlan: {
      id: 'plan-1',
      approved: false,
      operations: null,
    },
    conflicts: null,
  }
}

describe('SettingsPanel', () => {
  beforeEach(() => {
    startAtLogin.mockReset()
    startAtLogin.mockResolvedValue(false)
    updates.status.mockReset().mockResolvedValue(updateStatus())
    updates.check.mockReset().mockResolvedValue(updateStatus(false))
    updates.automatic.mockReset().mockImplementation((enabled: boolean) => Promise.resolve(updateStatus(enabled)))
  })

  it('opens immediately with a missing preview and refreshes on demand', async () => {
    const refreshPreview = vi.fn().mockResolvedValue(undefined)
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot(snapshot('0001-01-01T00:00:00Z', 0))}
        busy={false}
        previewLoading={false}
        close={vi.fn()}
        refreshPreview={refreshPreview}
        runSyncNow={vi.fn().mockResolvedValue(undefined)}
        save={vi.fn().mockResolvedValue(true)}
      />,
    )

    expect(screen.getByRole('dialog', { name: 'Sync settings' })).toBeInTheDocument()
    expect(screen.getByText('No preview generated yet')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }))
    expect(refreshPreview).toHaveBeenCalledTimes(1)
  })

  it('shows the persisted preview timestamp and file count', () => {
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot(snapshot('2026-08-18T09:00:00Z', 7))}
        busy={false}
        previewLoading={false}
        close={vi.fn()}
        refreshPreview={vi.fn().mockResolvedValue(undefined)}
        runSyncNow={vi.fn().mockResolvedValue(undefined)}
        save={vi.fn().mockResolvedValue(true)}
      />,
    )

    expect(screen.getByText(/Last refreshed/)).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByText('safe files')).toBeInTheDocument()
  })

  it('shows run sync now action in settings and triggers callback', async () => {
    const runSyncNow = vi.fn().mockResolvedValue(undefined)
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot(snapshot('2026-08-18T09:00:00Z', 7))}
        busy={false}
        previewLoading={false}
        close={vi.fn()}
        refreshPreview={vi.fn().mockResolvedValue(undefined)}
        runSyncNow={runSyncNow}
        save={vi.fn().mockResolvedValue(true)}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Run sync now' }))
    expect(runSyncNow).toHaveBeenCalledTimes(1)
  })

  it('keeps update actions outside sync-form validation and saves sync settings unchanged', async () => {
    const save = vi.fn().mockResolvedValue(true)
    const close = vi.fn()
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot(snapshot('2026-08-18T09:00:00Z', 7))}
        busy={false}
        previewLoading={false}
        close={close}
        refreshPreview={vi.fn().mockResolvedValue(undefined)}
        runSyncNow={vi.fn().mockResolvedValue(undefined)}
        save={save}
      />,
    )
    await screen.findByText('v1.0.0')
    const user = userEvent.setup()
    const repository = screen.getByRole('textbox', { name: /Private Git repository/ })
    await user.clear(repository)
    await user.click(screen.getByRole('checkbox', { name: 'Automatic updates' }))
    await user.click(screen.getByRole('button', { name: 'Check now' }))

    expect(updates.automatic).toHaveBeenCalledWith(false)
    expect(updates.check).toHaveBeenCalledTimes(1)
    expect(save).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
    const panel = screen.getByRole('group', { name: 'Application updates' })
    expect(panel.closest('form')).toBeNull()
    for (const button of within(panel).getAllByRole('button')) {
      expect(button).toHaveAttribute('type', 'button')
    }

    await user.type(repository, 'git@github.com:owner/changed.git')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        repositoryUrl: 'git@github.com:owner/changed.git',
        intervalMinutes: 10,
        trashGraceDays: 30,
      }),
      false,
      false,
    )
    expect(save.mock.calls[0][0]).not.toHaveProperty('automatic')
    expect(close).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['updating', false, true],
    ['idle', true, true],
    ['idle', false, false],
  ])('guards update restart for sync state %s and settings busy=%s', async (state, busy, disabled) => {
    updates.status.mockResolvedValue({
      ...updateStatus(),
      latestVersion: 'v1.1.0',
      phase: 'ready',
    })
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot({ ...snapshot('2026-08-18T09:00:00Z', 7), state })}
        busy={busy}
        previewLoading={false}
        close={vi.fn()}
        refreshPreview={vi.fn().mockResolvedValue(undefined)}
        runSyncNow={vi.fn().mockResolvedValue(undefined)}
        save={vi.fn().mockResolvedValue(true)}
      />,
    )

    const restart = await screen.findByRole('button', { name: 'Restart to update' })
    if (disabled) expect(restart).toBeDisabled()
    else expect(restart).toBeEnabled()
  })
})
