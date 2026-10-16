import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Snapshot } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { SettingsPanel } from './SettingsPanel'
import { normalizeSnapshot } from './desktopState'
import { setLocale } from './i18n'

const startAtLogin = vi.fn()
const librarySnapshot = vi.fn()
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
  LibrarySnapshot: () => librarySnapshot(),
  TrayTipsEnabled: vi.fn().mockResolvedValue(true),
  SetTrayTipsEnabled: vi.fn().mockResolvedValue(undefined),
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
  afterEach(() => act(() => setLocale('en')))
  beforeEach(() => {
    startAtLogin.mockReset()
    startAtLogin.mockResolvedValue(false)
    librarySnapshot.mockReset().mockResolvedValue({ root: '/test/.agents/library', entries: [] })
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
    await userEvent.click(screen.getByRole('tab', { name: 'Resources' }))
    expect(screen.getByText('No preview generated yet')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }))
    expect(refreshPreview).toHaveBeenCalledTimes(1)
  })

  it('shows the persisted preview timestamp and file count', async () => {
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

    await userEvent.click(screen.getByRole('tab', { name: 'Resources' }))
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
    await user.click(screen.getByRole('tab', { name: 'Repository' }))
    const repository = screen.getByRole('textbox', { name: /Private Git repository/ })
    await user.clear(repository)
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
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

    await user.click(screen.getByRole('tab', { name: 'Repository' }))
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

    await userEvent.click(screen.getByRole('tab', { name: 'Desktop' }))
    const restart = await screen.findByRole('button', { name: 'Restart to update' })
    if (disabled) expect(restart).toBeDisabled()
    else expect(restart).toBeEnabled()
  })

  function renderSettings(save = vi.fn().mockResolvedValue(true)) {
    render(
      <SettingsPanel
        snapshot={normalizeSnapshot(snapshot('2026-08-18T09:00:00Z', 7))}
        busy={false}
        previewLoading={false}
        close={vi.fn()}
        refreshPreview={vi.fn().mockResolvedValue(undefined)}
        runSyncNow={vi.fn().mockResolvedValue(undefined)}
        save={save}
      />,
    )
    return save
  }

  it('explains each category with a labeled engineering diagram', async () => {
    renderSettings()
    const user = userEvent.setup()
    for (const tab of ['Repository', 'Sync', 'Resources', 'Library', 'Desktop', 'Advanced']) {
      await user.click(screen.getByRole('tab', { name: tab }))
      expect(within(screen.getByRole('tabpanel', { name: tab })).getByRole('img')).toBeVisible()
    }
  })

  it('switches language without resetting drafts or refetching startup settings', async () => {
    const save = renderSettings()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Repository' }))
    const repository = screen.getByRole('textbox', { name: /Private Git repository/ })
    await user.clear(repository)
    await user.type(repository, 'git@github.com:owner/bilingual.git')
    await user.selectOptions(screen.getByRole('combobox', { name: 'Language / 语言' }), 'zh-CN')
    expect(screen.getByRole('tab', { name: '同步仓库' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('textbox', { name: /私有 Git 仓库/ })).toHaveValue('git@github.com:owner/bilingual.git')
    await user.click(screen.getByRole('button', { name: '保存设置' }))
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ repositoryUrl: 'git@github.com:owner/bilingual.git' }), false, false)
    expect(startAtLogin).toHaveBeenCalledTimes(1)
  })

  it('groups controls into six accessible categories without resetting drafts', async () => {
    startAtLogin.mockResolvedValue(true)
    const save = renderSettings()
    const user = userEvent.setup()
    expect(screen.getByRole('tablist', { name: 'Settings categories' })).toBeVisible()
    expect(screen.getAllByRole('tab')).toHaveLength(6)
    expect(screen.getByRole('tab', { name: 'Sync' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByRole('textbox', { name: /Private Git repository/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Repository' }))
    const repository = screen.getByRole('textbox', { name: /Private Git repository/ })
    await user.clear(repository)
    await user.type(repository, 'git@github.com:owner/new.git')
    await user.click(screen.getByRole('tab', { name: 'Resources' }))
    await user.click(screen.getByRole('checkbox', { name: /claude.*safety exclusions/i }))
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
    const startup = screen.getByRole('checkbox', { name: /Start with Windows/ })
    await waitFor(() => expect(startup).toBeChecked())
    await user.click(startup)
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({ repositoryUrl: 'git@github.com:owner/new.git', agents: { claude: false } }),
      false,
      true,
    )
    expect(updates.status).toHaveBeenCalledTimes(1)
  })

  it('supports arrow, Home and End navigation with roving keyboard focus', async () => {
    renderSettings()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'Resources' })).toHaveFocus()
    expect(screen.getByRole('tabpanel', { name: 'Resources' })).toBeVisible()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'Library' })).toHaveFocus()
    expect(screen.getByRole('tabpanel', { name: 'Library' })).toBeVisible()
    await user.keyboard('{End}')
    expect(screen.getByRole('tab', { name: 'Advanced' })).toHaveFocus()
    expect(screen.getByRole('button', { name: 'Reset and start over' })).toBeVisible()
    await user.keyboard('{Home}')
    expect(screen.getByRole('tab', { name: 'Repository' })).toHaveFocus()
    await user.keyboard('{ArrowLeft}')
    expect(screen.getByRole('tab', { name: 'Advanced' })).toHaveFocus()
    expect(screen.getByRole('tab', { name: 'Advanced' })).toHaveAttribute('tabindex', '0')
    expect(screen.getByRole('tab', { name: 'Repository' })).toHaveAttribute('tabindex', '-1')
  })

  it('uses a vertical category sidebar with up and down keyboard navigation', async () => {
    renderSettings()
    const user = userEvent.setup()
    expect(screen.getByRole('tablist')).toHaveAttribute('aria-orientation', 'vertical')
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    await user.keyboard('{ArrowDown}')
    expect(screen.getByRole('tab', { name: 'Resources' })).toHaveFocus()
    await user.keyboard('{ArrowUp}')
    expect(screen.getByRole('tab', { name: 'Sync' })).toHaveFocus()
  })

  it('does not present Save settings as the way to confirm a library operation', async () => {
    const save = renderSettings()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Library' }))
    expect(screen.queryByRole('button', { name: 'Save settings' })).not.toBeInTheDocument()
    expect(screen.getByText('Library changes use their own preview and confirmation.')).toBeVisible()
    const form = screen.getByRole('tabpanel', { name: 'Library' }).closest('form')
    if (!form) throw new Error('Missing settings form')
    fireEvent.submit(form)
    expect(save).not.toHaveBeenCalled()
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeVisible()
    expect(save).not.toHaveBeenCalled()
  })

  it('scans the library only while its tab is mounted and rescans on return', async () => {
    const save = renderSettings()
    expect(librarySnapshot).not.toHaveBeenCalled()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Library' }))
    await screen.findByRole('button', { name: 'Refresh library' })
    expect(librarySnapshot).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    expect(screen.queryByRole('heading', { name: 'Shared library' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Library' }))
    await waitFor(() => expect(librarySnapshot).toHaveBeenCalledTimes(2))
    await user.click(screen.getByRole('button', { name: 'Refresh library' }))
    expect(save).not.toHaveBeenCalled()
  })

  it('reveals and focuses invalid settings from another tab rather than silently blocking save', async () => {
    const save = renderSettings()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Repository' }))
    await user.clear(screen.getByRole('textbox', { name: /Private Git repository/ }))
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))
    expect(save).not.toHaveBeenCalled()
    expect(screen.getByRole('tab', { name: 'Repository' })).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(screen.getByRole('textbox', { name: /Private Git repository/ })).toHaveFocus())
  })

  it('surfaces startup errors and allows retry without losing other settings', async () => {
    startAtLogin.mockRejectedValueOnce(new Error('Registry access denied'))
    renderSettings()
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Registry access denied')
    expect(screen.getByRole('checkbox', { name: /Start with Windows/ })).toBeDisabled()
    startAtLogin.mockResolvedValue(true)
    await user.click(screen.getByRole('button', { name: 'Retry startup status' }))
    await waitFor(() => expect(screen.getByRole('checkbox', { name: /Start with Windows/ })).toBeChecked())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Sync' }))
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
    expect(screen.getByRole('checkbox', { name: /Start with Windows/ })).toBeChecked()
  })
})
