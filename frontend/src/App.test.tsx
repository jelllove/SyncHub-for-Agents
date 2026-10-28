import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  ResourcePreview,
  Snapshot,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import type { Status } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/updater/models'
import App from './App'
import { setLocale } from './i18n'

const api = vi.hoisted(() => ({
  needsOnboarding: vi.fn(),
  resourcePreview: vi.fn(),
  snapshot: vi.fn(),
  startAtLogin: vi.fn(),
  updateStatus: vi.fn(),
  checkForUpdates: vi.fn(),
  restartToUpdate: vi.fn(),
  openURL: vi.fn(),
  windows: vi.fn(),
  minimise: vi.fn(),
  toggleMaximise: vi.fn(),
  isMaximised: vi.fn(),
  closeWindow: vi.fn(),
  resetPreview: vi.fn(),
  resetLocalSetup: vi.fn(),
  saveSettings: vi.fn(),
  triggerSync: vi.fn(),
  resume: vi.fn(),
  acknowledgeSyncNotices: vi.fn(),
  events: new Map<string, (event: { data: unknown }) => void>(),
}))

vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: (url: string) => api.openURL(url),
  },
  System: {
    IsWindows: () => api.windows(),
  },
  Window: {
    Minimise: () => api.minimise(),
    ToggleMaximise: () => api.toggleMaximise(),
    IsMaximised: () => api.isMaximised(),
    Close: () => api.closeWindow(),
  },
  Events: {
    On: vi.fn((name: string, callback: (event: { data: unknown }) => void) => {
      api.events.set(name, callback)
      return () => api.events.delete(name)
    }),
  },
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  ApproveInstallPlan: vi.fn(),
  AcknowledgeSyncNotices: (fingerprint: string) => api.acknowledgeSyncNotices(fingerprint),
  NeedsOnboarding: () => api.needsOnboarding(),
  Pause: vi.fn(),
  PreviewCustomResource: vi.fn(),
  QueueConflictBatch: vi.fn(),
  RetryConflictBatch: vi.fn(),
  RetryInstallPlan: vi.fn(),
  ResourcePreview: () => api.resourcePreview(),
  Resume: () => api.resume(),
  SaveSettings: (input: unknown) => api.saveSettings(input),
  SetStartAtLogin: vi.fn(),
  Snapshot: () => api.snapshot(),
  StartAtLogin: () => api.startAtLogin(),
  TrayTipsEnabled: vi.fn().mockResolvedValue(true),
  SetTrayTipsEnabled: vi.fn().mockResolvedValue(undefined),
  TriggerSync: () => api.triggerSync(),
  UpdateStatus: () => api.updateStatus(),
  CheckForUpdates: () => api.checkForUpdates(),
  SetAutomaticUpdates: vi.fn(),
  RestartToUpdate: () => api.restartToUpdate(),
  ResetPreview: () => api.resetPreview(),
  ResetLocalSetup: (confirmation: string, path: string) => api.resetLocalSetup(confirmation, path),
  OnboardingState: vi.fn().mockResolvedValue({
    step: 'welcome', repositoryUrl: '', authMode: '', agents: [],
  }),
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

function diagnosticSnapshot(): Snapshot {
  const initial = configuredSnapshot()
  initial.progress.stage = 'complete'
  initial.progress.skipped = 2
  initial.progress.blockedFiles = 1
  initial.progress.needsAttention = true
  initial.state = 'error'
  initial.syncNotices = {
    version: 1, fingerprint: 'exact-set', finishedAt: '2026-10-03T10:00:00Z',
    skipped: 2, blocked: 1, detailsAvailable: true, reviewed: false,
    issues: [
      { kind: 'skipped', resourceKey: 'demo/source', path: 'cache.tmp', code: 'unapproved-link', message: 'Link needs approval', bytes: 12, reviewable: true },
      { kind: 'blocked', resourceKey: 'demo/config', path: 'secret.json', code: 'invalid-path', message: 'Unsafe destination', bytes: 20, reviewable: true },
    ],
  }
  return initial
}

function deferredSnapshot() {
  let resolve!: (snapshot: Snapshot) => void
  const promise = new Promise<Snapshot>((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

function updateStatus(overrides: Partial<Status> = {}): Status {
  return {
    currentVersion: 'v1.0.0',
    latestVersion: '',
    phase: 'idle',
    automatic: true,
    supported: true,
    releaseURL: '',
    error: '',
    lastChecked: '',
    ...overrides,
  }
}

function readyUpdate(overrides: Partial<Status> = {}) {
  return updateStatus({
    latestVersion: 'v1.1.0',
    phase: 'ready',
    releaseURL: 'https://github.com/jelllove/SyncHub-for-Agents/releases/tag/v1.1.0',
    ...overrides,
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

function publishUpdate(status: Status) {
  act(() => api.events.get('updater:status')?.({ data: status }))
}

describe('App settings preview', () => {
  afterEach(() => act(() => setLocale('en')))
  beforeEach(() => {
    api.needsOnboarding.mockReset()
    api.resourcePreview.mockReset()
    api.snapshot.mockReset()
    api.startAtLogin.mockReset()
    api.updateStatus.mockReset()
    api.checkForUpdates.mockReset().mockResolvedValue(updateStatus({ phase: 'upToDate' }))
    api.restartToUpdate.mockReset().mockResolvedValue(undefined)
    api.openURL.mockReset().mockResolvedValue(undefined)
    api.windows.mockReset().mockReturnValue(true)
    api.minimise.mockReset().mockResolvedValue(undefined)
    api.toggleMaximise.mockReset().mockResolvedValue(undefined)
    api.isMaximised.mockReset().mockResolvedValue(false)
    api.closeWindow.mockReset().mockResolvedValue(undefined)
    api.saveSettings.mockReset().mockResolvedValue(undefined)
    api.triggerSync.mockReset().mockResolvedValue(undefined)
    api.resume.mockReset().mockResolvedValue(undefined)
    api.acknowledgeSyncNotices.mockReset()
    api.events.clear()
    api.needsOnboarding.mockResolvedValue(false)
    api.snapshot.mockResolvedValue(configuredSnapshot())
    api.startAtLogin.mockResolvedValue(false)
    api.updateStatus.mockResolvedValue(updateStatus())
  })

  it('header update installs a ready release beside minimize without opening settings', async () => {
    api.updateStatus.mockResolvedValue(readyUpdate())
    render(<App />)

    const update = await screen.findByRole('button', { name: 'Update' })
    const titleBar = screen.getByRole('toolbar', { name: 'Window title bar' })
    expect(within(titleBar).getByRole('button', { name: 'Update' })).toBe(update)
    expect(within(titleBar).getAllByRole('button').map((button) => button.textContent || button.getAttribute('aria-label'))).toEqual([
      'Update', 'Minimize window', 'Maximize window', 'Close window',
    ])
    expect(within(screen.getByRole('banner')).queryByRole('button', { name: 'Update' })).not.toBeInTheDocument()
    expect(update).toHaveAttribute('title', expect.stringMatching(/v1\.1\.0.*restart/i))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(api.restartToUpdate).not.toHaveBeenCalled()
    await userEvent.click(update)

    expect(api.restartToUpdate).toHaveBeenCalledTimes(1)
    expect(api.checkForUpdates).not.toHaveBeenCalled()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('localizes the title bar and update action without resetting the published settings language', async () => {
    api.updateStatus.mockResolvedValue(readyUpdate())
    render(<App />)
    await screen.findByRole('button', { name: 'Update' })
    act(() => setLocale('zh-CN'))

    const titleBar = screen.getByRole('toolbar', { name: '窗口标题栏' })
    expect(within(titleBar).getByRole('button', { name: '更新' })).toBeEnabled()
    expect(within(titleBar).getByRole('button', { name: '最小化窗口' })).toBeEnabled()
    expect(within(titleBar).getByRole('button', { name: '最大化窗口' })).toBeEnabled()
    expect(within(titleBar).getByRole('button', { name: '关闭窗口' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '打开设置' })).toBeEnabled()
    expect(api.updateStatus).toHaveBeenCalledTimes(1)
    expect(api.restartToUpdate).not.toHaveBeenCalled()
  })

  it.each(['idle', 'checking', 'upToDate', 'error'])('header update stays hidden without an available release (%s)', async (phase) => {
    api.updateStatus.mockResolvedValue(updateStatus({ phase }))
    render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })

    expect(within(screen.getByRole('toolbar', { name: 'Window title bar' })).queryByRole('button', { name: /Update|Downloading|Restarting/i })).not.toBeInTheDocument()
  })

  it('header update follows download events while settings are closed', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })
    expect(api.events.has('updater:status')).toBe(true)
    publishUpdate(readyUpdate({ phase: 'downloading' }))

    expect(screen.getByRole('button', { name: 'Downloading…' })).toBeDisabled()
    publishUpdate(readyUpdate())
    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
    expect(api.restartToUpdate).not.toHaveBeenCalled()
    publishUpdate(updateStatus({ phase: 'upToDate' }))
    expect(screen.queryByRole('button', { name: 'Update' })).not.toBeInTheDocument()
  })

  it('header update persists after checking in settings and closing them', async () => {
    const initial = configuredSnapshot()
    initial.preview.generatedAt = '2026-10-28T05:00:00Z'
    api.snapshot.mockResolvedValue(initial)
    api.checkForUpdates.mockResolvedValue(readyUpdate())
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Open settings' }))
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
    await user.click(await screen.findByRole('button', { name: 'Check now' }))
    expect(screen.getByRole('button', { name: 'Restart to update' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Close settings' }))

    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
    expect(api.updateStatus).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Update' }))
    expect(api.restartToUpdate).toHaveBeenCalledTimes(1)
  })

  it.each(['available', 'ready'])('header update opens the supplied manual download on unsupported platforms (%s)', async (phase) => {
    const latest = readyUpdate({ phase, supported: false })
    api.updateStatus.mockResolvedValue(latest)
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: 'Download update' }))

    expect(api.openURL).toHaveBeenCalledWith(latest.releaseURL)
    expect(api.restartToUpdate).not.toHaveBeenCalled()
  })

  it.each([
    { currentVersion: 'dev' },
    { supported: false, releaseURL: '' },
  ])('header update does not offer an unusable action (%j)', async (overrides) => {
    api.updateStatus.mockResolvedValue(readyUpdate(overrides))
    render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })

    expect(within(screen.getByRole('toolbar', { name: 'Window title bar' })).queryByRole('button', { name: /Update/i })).not.toBeInTheDocument()
  })

  it('header update waits until active synchronization finishes', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), state: 'updating' })
    api.updateStatus.mockResolvedValue(readyUpdate())
    render(<App />)
    const update = await screen.findByRole('button', { name: 'Update' })

    expect(update).toBeDisabled()
    expect(update).toHaveAttribute('title', expect.stringMatching(/synchronization/i))
    await userEvent.click(update)
    expect(api.restartToUpdate).not.toHaveBeenCalled()
    act(() => api.events.get('desktop:snapshot')?.({ data: configuredSnapshot() }))
    expect(update).toBeEnabled()
  })

  it('header update waits for settings changes to finish', async () => {
    const initial = configuredSnapshot()
    initial.preview.generatedAt = '2026-10-28T05:00:00Z'
    api.snapshot.mockResolvedValue(initial)
    api.updateStatus.mockResolvedValue(readyUpdate())
    const save = deferred<void>()
    api.saveSettings.mockReturnValue(save.promise)
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Open settings' }))
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    expect(screen.getByRole('button', { name: 'Update' })).toBeDisabled()
    expect(api.restartToUpdate).not.toHaveBeenCalled()
    await act(async () => save.resolve(undefined))
    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
  })

  it('header update shares pending actions with settings and prevents duplicate restarts', async () => {
    const initial = configuredSnapshot()
    initial.preview.generatedAt = '2026-10-28T05:00:00Z'
    api.snapshot.mockResolvedValue(initial)
    api.updateStatus.mockResolvedValue(readyUpdate())
    const restart = deferred<void>()
    api.restartToUpdate.mockReturnValue(restart.promise)
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Open settings' }))
    await user.click(screen.getByRole('tab', { name: 'Desktop' }))
    await user.click(screen.getByRole('button', { name: 'Restart to update' }))

    expect(screen.getAllByRole('button', { name: 'Restarting…' })).toHaveLength(2)
    for (const button of screen.getAllByRole('button', { name: 'Restarting…' })) {
      expect(button).toBeDisabled()
    }
    await user.click(screen.getByRole('button', { name: 'Close settings' }))
    await user.click(screen.getByRole('button', { name: 'Restarting…' }))
    expect(api.restartToUpdate).toHaveBeenCalledTimes(1)
    await act(async () => restart.resolve(undefined))
  })

  it('header update shows restart failures and allows retry without opening settings', async () => {
    api.updateStatus.mockResolvedValue(readyUpdate())
    api.restartToUpdate.mockRejectedValueOnce(new Error('Synchronization is still running'))
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: 'Update' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Synchronization is still running')
    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
    await userEvent.click(screen.getByRole('button', { name: 'Update' }))
    expect(api.restartToUpdate).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('header update exposes backend errors with a settings recovery action', async () => {
    const initial = configuredSnapshot()
    initial.preview.generatedAt = '2026-10-28T05:00:00Z'
    api.snapshot.mockResolvedValue(initial)
    render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })
    publishUpdate(readyUpdate({ phase: 'error', error: 'Update checksum did not match' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Update checksum did not match')
    await userEvent.click(screen.getByRole('button', { name: 'Review updates' }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Desktop' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('alert')).toHaveTextContent('Update checksum did not match')
  })

  it('header update ignores a stale initial status response after a ready event', async () => {
    const initial = deferred<Status>()
    api.updateStatus.mockReturnValue(initial.promise)
    const app = render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })
    publishUpdate(readyUpdate())
    await act(async () => initial.resolve(updateStatus()))

    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
    app.unmount()
    expect(api.events.has('updater:status')).toBe(false)
  })

  it('preserves the native title bar on other platforms and keeps Update in the app header', async () => {
    api.windows.mockReturnValue(false)
    api.updateStatus.mockResolvedValue(readyUpdate())
    render(<App />)

    expect(await screen.findByRole('button', { name: 'Update' })).toBe(within(screen.getByRole('banner')).getByRole('button', { name: 'Update' }))
    expect(screen.queryByRole('toolbar', { name: 'Window title bar' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Minimize window' })).not.toBeInTheDocument()
    expect(api.isMaximised).not.toHaveBeenCalled()
  })

  it('keeps window controls available while the application is loading', async () => {
    const initial = deferredSnapshot()
    api.snapshot.mockReturnValue(initial.promise)
    render(<App />)

    expect(screen.getByRole('button', { name: 'Minimize window' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Close window' })).toBeEnabled()
    expect(screen.getByText('Opening SyncHub for Agents…')).toBeInTheDocument()
    await act(async () => initial.resolve(configuredSnapshot()))
  })

  it('routes title bar minimize, maximize and close through the current Wails window', async () => {
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Minimize window' }))
    expect(api.minimise).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Maximize window' }))
    expect(api.toggleMaximise).toHaveBeenCalledTimes(1)
    act(() => api.events.get('common:WindowMaximise')?.({ data: undefined }))
    await user.click(screen.getByRole('button', { name: 'Restore window' }))
    expect(api.toggleMaximise).toHaveBeenCalledTimes(2)
    act(() => api.events.get('common:WindowUnMaximise')?.({ data: undefined }))
    expect(screen.getByRole('button', { name: 'Maximize window' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Close window' }))
    expect(api.closeWindow).toHaveBeenCalledTimes(1)
  })

  it('ignores a stale maximized-state response after a native window event', async () => {
    const initial = deferred<boolean>()
    api.isMaximised.mockReturnValue(initial.promise)
    const app = render(<App />)
    await screen.findByRole('button', { name: 'Open settings' })
    act(() => api.events.get('common:WindowMaximise')?.({ data: undefined }))
    await act(async () => initial.resolve(false))

    expect(screen.getByRole('button', { name: 'Restore window' })).toBeEnabled()
    app.unmount()
    expect(api.events.has('common:WindowMaximise')).toBe(false)
    expect(api.events.has('common:WindowUnMaximise')).toBe(false)
  })

  it('shows window-control failures and permits a retry', async () => {
    api.minimise.mockRejectedValueOnce(new Error('Window unavailable'))
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: 'Minimize window' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Window unavailable')
    await userEvent.click(screen.getByRole('button', { name: 'Minimize window' }))
    expect(api.minimise).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('prevents duplicate window-control actions while a request is pending', async () => {
    const minimize = deferred<void>()
    api.minimise.mockReturnValue(minimize.promise)
    render(<App />)
    await userEvent.click(await screen.findByRole('button', { name: 'Minimize window' }))
    expect(screen.getByRole('button', { name: 'Minimize window' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Maximize window' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Minimize window' }))
    expect(api.minimise).toHaveBeenCalledTimes(1)
    await act(async () => minimize.resolve(undefined))
    expect(screen.getByRole('button', { name: 'Minimize window' })).toBeEnabled()
  })

  it('switches first-run setup between English and Chinese without restarting onboarding', async () => {
    api.needsOnboarding.mockResolvedValue(true)
    const user = userEvent.setup()
    render(<App />)
    expect(await screen.findByRole('button', { name: 'Get started' })).toBeVisible()
    const language = screen.getByRole('combobox', { name: /Language|语言/ })
    await user.selectOptions(language, 'zh-CN')
    expect(screen.getByRole('button', { name: '开始使用' })).toBeVisible()
    expect(screen.getByText('凭据仅保存在操作系统密钥环中。')).toBeVisible()
    await user.selectOptions(language, 'en')
    expect(screen.getByRole('button', { name: 'Get started' })).toBeVisible()
    expect(api.needsOnboarding).toHaveBeenCalledTimes(1)
  })

  it('offers language switching only inside settings after setup and preserves the choice when closed', async () => {
    api.resourcePreview.mockResolvedValue(configuredSnapshot().preview)
    const user = userEvent.setup()
    render(<App />)
    expect(await screen.findByRole('heading', { name: 'Ready' })).toBeVisible()
    act(() => api.events.get('desktop:progress')?.({ data: {
      ...configuredSnapshot().progress, stage: 'complete', revision: 1,
    } }))
    expect(screen.queryByRole('combobox', { name: /Language|语言/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Open settings' }))
    const settings = screen.getByRole('dialog', { name: 'Sync settings' })
    expect(screen.getAllByRole('combobox', { name: /Language|语言/ })).toHaveLength(1)
    await user.selectOptions(within(settings).getByRole('combobox', { name: /Language|语言/ }), 'zh-CN')
    expect(localStorage.getItem('synchub.ui.language')).toBe('zh-CN')
    await user.click(screen.getByRole('button', { name: '关闭设置' }))
    expect(screen.queryByRole('combobox', { name: /Language|语言/ })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '就绪' })).toBeVisible()
    expect(screen.getByRole('button', { name: '立即同步' })).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent('同步完成；无需更改')
    expect(screen.getByText('每 10 分钟')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '打开设置' }))
    const language = within(screen.getByRole('dialog', { name: '同步设置' })).getByRole('combobox', { name: /Language|语言/ })
    expect(language).toHaveValue('zh-CN')
    await user.selectOptions(language, 'en')
    await user.click(screen.getByRole('button', { name: 'Close settings' }))
    expect(screen.getByRole('button', { name: 'Sync now' })).toBeVisible()
    expect(api.snapshot).toHaveBeenCalledTimes(1)
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('translates a retained local acknowledgement error without translating service diagnostics', async () => {
    const initial = diagnosticSnapshot()
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockResolvedValue({ ...initial, syncNotices: { ...initial.syncNotices, fingerprint: 'new-set' } })
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Review notices' }))
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Sync notice set changed while saving')
    act(() => setLocale('zh-CN'))
    expect(screen.getByRole('alert')).toHaveTextContent('保存时同步提示集已更改；请关闭并重新打开日志以审阅当前记录。')
    expect(screen.getByText('Unsafe destination')).toBeVisible()
    expect(api.acknowledgeSyncNotices).toHaveBeenCalledTimes(1)
  })

  it('does not prompt for skips when the backend reports only normal exclusions', async () => {
    const initial = configuredSnapshot()
    initial.progress = { ...initial.progress, stage: 'complete', skipped: 250 }
    initial.syncNotices = {
      version: 1, fingerprint: 'normal', finishedAt: '', skipped: 0, blocked: 0,
      detailsAvailable: true, reviewed: false, issues: [],
    }
    api.snapshot.mockResolvedValue(initial)
    render(<App />)
    expect(await screen.findByRole('heading', { name: 'Ready' })).toBeVisible()
    expect(screen.queryByText('Skipped')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review notices' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Open sync diagnostics log/ })).not.toBeInTheDocument()
    expect(api.acknowledgeSyncNotices).not.toHaveBeenCalled()
  })

  it('opens actionable diagnostics by mouse and keyboard without syncing or scanning', async () => {
    const initial = diagnosticSnapshot()
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockResolvedValue({
      ...initial, state: 'idle', progress: { ...initial.progress, needsAttention: false },
      syncNotices: { ...initial.syncNotices, reviewed: true },
    })
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Issues: 2. Open sync diagnostics log' }))
    expect(screen.getByText('cache.tmp')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Close log' }))
    const blocked = screen.getByRole('button', { name: 'Issues: 2. Open sync diagnostics log' })
    blocked.focus()
    await user.keyboard('{Enter}')
    expect(screen.getByText('secret.json')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(await screen.findByText('Safety notices reviewed')).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Ready' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Close log' }))
    expect(blocked).toHaveFocus()
    expect(screen.getByRole('button', { name: 'Issues: 2. Open sync diagnostics log' })).toBeVisible()
    expect(api.triggerSync).not.toHaveBeenCalled()
    expect(api.resourcePreview).not.toHaveBeenCalled()
  })

  it('offers a prominent review action and clears attention after confirming recorded exclusions', async () => {
    const initial = diagnosticSnapshot()
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockResolvedValue({
      ...initial, state: 'done', progress: { ...initial.progress, needsAttention: false },
      syncNotices: { ...initial.syncNotices, reviewed: true },
    })
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Review notices' }))
    expect(screen.getByText('cache.tmp')).toBeVisible()
    expect(screen.getByText('secret.json')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    await user.click(screen.getByRole('button', { name: 'Close log' }))
    expect(screen.getByRole('heading', { name: 'Sync complete' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Needs attention' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review notices' })).not.toBeInTheDocument()
    expect(screen.getByText('Safety notices reviewed. Protection rules remain active.')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Issues: 2. Open sync diagnostics log' })).toBeVisible()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('uses an outlined settings icon and still opens settings', async () => {
    const user = userEvent.setup()
    render(<App />)
    const button = await screen.findByRole('button', { name: 'Open settings' })
    const icon = button.querySelector('svg')
    expect(icon).toHaveAttribute('fill', 'none')
    expect(icon).toHaveAttribute('stroke', 'currentColor')
    expect(icon).toHaveAttribute('stroke-linecap', 'round')
    expect(icon).toHaveAttribute('stroke-linejoin', 'round')
    await user.click(button)
    expect(await screen.findByRole('heading', { name: 'Sync settings' })).toBeVisible()
  })

  it('pins the displayed issue set across snapshots and visibly rejects stale acknowledgement', async () => {
    const initial = diagnosticSnapshot()
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockRejectedValue(new Error('Sync notice set changed; reopen the log'))
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Issues: 2. Open sync diagnostics log' }))
    act(() => api.events.get('desktop:snapshot')?.({
      data: {
        ...initial, syncNotices: {
          ...initial.syncNotices, fingerprint: 'new-set', skipped: 3,
          issues: [{ ...initial.syncNotices!.issues![0], path: 'new.tmp' }],
        },
      },
    }))
    expect(screen.getByText('cache.tmp')).toBeVisible()
    expect(screen.queryByText('new.tmp')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(api.acknowledgeSyncNotices).toHaveBeenCalledExactlyOnceWith('exact-set')
    expect(await screen.findByRole('alert')).toHaveTextContent('Sync notice set changed')
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Close log' }))
    const skipped = screen.getByRole('button', { name: 'Issues: 1. Open sync diagnostics log' })
    skipped.focus()
    await user.keyboard(' ')
    expect(screen.getByText('new.tmp')).toBeVisible()
    expect(api.triggerSync).not.toHaveBeenCalled()
    expect(api.resourcePreview).not.toHaveBeenCalled()
  })

  it('retains active error and approval panels when the API acknowledges only safety notices', async () => {
    const initial = diagnosticSnapshot()
    initial.lastError = 'Synthetic sync failure'
    initial.pendingInstallPlan = { id: 'synthetic', approved: false, operations: [] }
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockResolvedValue({
      ...initial, syncNotices: { ...initial.syncNotices, reviewed: true },
    })
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Issues: 2. Open sync diagnostics log' }))
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(await screen.findByText('Safety notices reviewed')).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    expect(screen.getByText('Synthetic sync failure')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Approve and synchronize' })).toBeVisible()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('does not let an old acknowledgement response or event replace a newer cycle with attention gates', async () => {
    const initial = { ...diagnosticSnapshot(), revision: 1 }
    const acknowledged = {
      ...initial, revision: 2, state: 'idle',
      progress: { ...initial.progress, needsAttention: false },
      syncNotices: { ...initial.syncNotices!, reviewed: true },
    }
    const newer = {
      ...initial, revision: 3, lastError: 'New cycle failed',
      conflicts: [{ id: 'new-conflict', revision: 'bundle-v1', resourceKey: 'demo/config', path: 'settings.json', createdAt: '2026-10-03T11:00:00Z' }],
      pendingInstallPlan: { id: 'new-install', approved: false, operations: [] },
      syncNotices: {
        ...initial.syncNotices!, fingerprint: 'new-cycle', skipped: 3,
        issues: [{ ...initial.syncNotices!.issues![0], path: 'new.tmp' }],
      },
    }
    const response = deferredSnapshot()
    api.snapshot.mockResolvedValue(initial)
    api.acknowledgeSyncNotices.mockReturnValue(response.promise)
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Issues: 2. Open sync diagnostics log' }))
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    act(() => api.events.get('desktop:snapshot')?.({ data: newer }))
    await act(async () => { response.resolve(acknowledged); await response.promise })
    act(() => api.events.get('desktop:snapshot')?.({ data: acknowledged }))
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    expect(screen.getByText('New cycle failed')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Issues: 1. Open sync diagnostics log' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Approve and synchronize' })).toBeVisible()
    expect(screen.getByRole('heading', { name: 'Resolve synchronized conflicts' })).toBeVisible()
    expect(screen.getByText('cache.tmp')).toBeVisible()
    expect(screen.queryByText('new.tmp')).not.toBeInTheDocument()
    expect(screen.getByText('Safety notices reviewed')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Close log' }))
    await user.click(screen.getByRole('button', { name: 'Issues: 1. Open sync diagnostics log' }))
    expect(screen.getByText('new.tmp')).toBeVisible()
    expect(screen.queryByText('Safety notices reviewed')).not.toBeInTheDocument()
    expect(api.acknowledgeSyncNotices).toHaveBeenCalledExactlyOnceWith('exact-set')
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('uses the same freshness guard for delayed refresh responses and snapshot events', async () => {
    const response = deferredSnapshot()
    api.snapshot.mockReturnValue(response.promise)
    render(<App />)
    await waitFor(() => expect(api.snapshot).toHaveBeenCalledOnce())
    const newer = { ...diagnosticSnapshot(), revision: 3, lastError: 'Newest error' }
    act(() => api.events.get('desktop:snapshot')?.({ data: newer }))
    await act(async () => { response.resolve({ ...configuredSnapshot(), revision: 1 }); await response.promise })
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    expect(screen.getByText('Newest error')).toBeVisible()
  })

  it('orders progress and full snapshots in one stream instead of regressing either channel', async () => {
    const initial = { ...diagnosticSnapshot(), revision: 1 }
    api.snapshot.mockResolvedValue(initial)
    render(<App />)
    await screen.findByRole('heading', { name: 'Needs attention' })
    const progress = {
      ...initial.progress, revision: 3, stage: 'applying',
      label: 'Applying the newer cycle', percentage: 70,
    }
    act(() => api.events.get('desktop:progress')?.({ data: progress }))
    act(() => api.events.get('desktop:snapshot')?.({ data: { ...initial, revision: 2, state: 'idle' } }))
    expect(screen.getByRole('heading', { name: 'Updating' })).toBeVisible()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '70')
    const failed = { ...initial, revision: 4, lastError: 'Latest operation failed' }
    act(() => api.events.get('desktop:snapshot')?.({ data: failed }))
    act(() => api.events.get('desktop:progress')?.({ data: { ...progress, revision: 3 } }))
    act(() => api.events.get('desktop:progress')?.({
      data: { ...initial.progress, revision: 2, stage: 'complete', needsAttention: false },
    }))
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    expect(screen.getByText('Latest operation failed')).toBeVisible()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.queryByText('Synchronization complete; no changes needed')).not.toBeInTheDocument()
  })

  it('requests a newer baseline when progress arrives before the initial snapshot resolves', async () => {
    const response = deferredSnapshot()
    const newer = { ...diagnosticSnapshot(), revision: 3, lastError: 'Latest initial cycle error' }
    api.snapshot.mockReturnValueOnce(response.promise).mockResolvedValue(newer)
    render(<App />)
    await waitFor(() => expect(api.snapshot).toHaveBeenCalledOnce())
    act(() => api.events.get('desktop:progress')?.({
      data: { ...newer.progress, revision: 2, stage: 'applying', label: 'Newer progress' },
    }))
    expect(await screen.findByText('Latest initial cycle error')).toBeVisible()
    await act(async () => { response.resolve({ ...configuredSnapshot(), revision: 1 }); await response.promise })
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    expect(screen.getByText('Latest initial cycle error')).toBeVisible()
    expect(api.snapshot).toHaveBeenCalledTimes(2)
  })

  it('automatically opens the first-sync dialog without choosing an overwrite direction', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true })
    render(<App />)
    const dialog = await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    expect(within(dialog).getAllByRole('radio')).toHaveLength(3)
    for (const radio of within(dialog).getAllByRole('radio')) expect(radio).not.toBeChecked()
    expect(within(dialog).getByRole('button', { name: 'Start first sync' })).toBeDisabled()
    expect(api.saveSettings).not.toHaveBeenCalled()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('defers without syncing and reopens the dialog from dashboard and settings sync actions', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true })
    api.resourcePreview.mockResolvedValue({
      generatedAt: '2026-10-03T00:00:00Z', resources: [], files: 0, bytes: 0,
      excludedFiles: 0, excludedBytes: 0, issues: [],
    })
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    await user.click(screen.getByRole('button', { name: 'Choose later' }))
    await user.click(screen.getByRole('button', { name: 'Sync now' }))
    expect(screen.getByRole('dialog', { name: 'Choose first sync strategy' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Choose later' }))
    await user.click(screen.getByRole('button', { name: 'Open settings' }))
    await user.click(screen.getByRole('button', { name: 'Run sync now' }))
    expect(screen.getByRole('dialog', { name: 'Choose first sync strategy' })).toBeVisible()
    expect(screen.queryByRole('dialog', { name: 'Sync settings' })).not.toBeInTheDocument()
    expect(api.saveSettings).not.toHaveBeenCalled()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it.each([
    ['Use cloud', 'use-cloud'],
    ['Merge cloud + local', 'merge-cloud-local'],
    ['Use local', 'use-local'],
  ])('confirms %s and relies on settings save to schedule one sync', async (label, strategy) => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true })
    const user = userEvent.setup()
    render(<App />)
    const dialog = await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    await user.click(within(dialog).getByRole('radio', { name: new RegExp(label.replace('+', '\\+')) }))
    expect(api.saveSettings).not.toHaveBeenCalled()
    api.snapshot.mockResolvedValue(configuredSnapshot())
    await user.click(within(dialog).getByRole('button', { name: 'Start first sync' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Choose first sync strategy' })).not.toBeInTheDocument())
    expect(api.saveSettings).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      firstSyncStrategy: strategy, repositoryUrl: 'git@github.com:owner/repo.git',
    }))
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('keeps save errors in the dialog and allows retry without losing the choice', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true })
    api.saveSettings.mockRejectedValueOnce(new Error('Cannot save configuration'))
    const user = userEvent.setup()
    render(<App />)
    const dialog = await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    await user.click(within(dialog).getByRole('radio', { name: /Use local/ }))
    await user.click(within(dialog).getByRole('button', { name: 'Start first sync' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Cannot save configuration')
    expect(within(dialog).getByRole('radio', { name: /Use local/ })).toBeChecked()
    expect(api.triggerSync).not.toHaveBeenCalled()
    api.snapshot.mockResolvedValue(configuredSnapshot())
    await user.click(within(dialog).getByRole('button', { name: 'Start first sync' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Choose first sync strategy' })).not.toBeInTheDocument())
    expect(api.saveSettings).toHaveBeenCalledTimes(2)
  })

  it('resumes paused synchronization only after first-sync confirmation', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true, state: 'paused' })
    const user = userEvent.setup()
    render(<App />)
    const dialog = await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    await user.click(within(dialog).getByRole('radio', { name: /Merge cloud/ }))
    expect(api.resume).not.toHaveBeenCalled()
    api.snapshot.mockResolvedValue(configuredSnapshot())
    await user.click(within(dialog).getByRole('button', { name: 'Start first sync' }))
    expect(api.resume).toHaveBeenCalledOnce()
    expect(api.resume.mock.invocationCallOrder[0]).toBeLessThan(api.saveSettings.mock.invocationCallOrder[0])
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('does not save a strategy when resuming paused synchronization fails', async () => {
    api.snapshot.mockResolvedValue({ ...configuredSnapshot(), firstSyncRequired: true, state: 'paused' })
    api.resume.mockRejectedValueOnce(new Error('Cannot resume synchronization'))
    const user = userEvent.setup()
    render(<App />)
    const dialog = await screen.findByRole('dialog', { name: 'Choose first sync strategy' })
    await user.click(within(dialog).getByRole('radio', { name: /Use cloud/ }))
    await user.click(within(dialog).getByRole('button', { name: 'Start first sync' }))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Cannot resume synchronization')
    expect(api.saveSettings).not.toHaveBeenCalled()
    expect(api.triggerSync).not.toHaveBeenCalled()
  })

  it('keeps normal sync behavior once the first strategy is already selected', async () => {
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Sync now' }))
    expect(api.triggerSync).toHaveBeenCalledOnce()
    expect(api.saveSettings).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog', { name: 'Choose first sync strategy' })).not.toBeInTheDocument()
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

  it('returns from settings to the first-run welcome screen after confirmed reset', async () => {
    api.resourcePreview.mockResolvedValue({
      generatedAt: '2026-10-02T00:00:00Z', resources: [], files: 0, bytes: 0,
      excludedFiles: 0, excludedBytes: 0, issues: [],
    })
    api.resetPreview.mockResolvedValue({ repoPath: 'C:\\synthetic\\repo' })
    api.resetLocalSetup.mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Open settings' }))
    await screen.findByText(/Last refreshed/)
    await user.click(screen.getByRole('tab', { name: 'Advanced' }))
    await user.click(screen.getByRole('button', { name: 'Reset and start over' }))
    await screen.findByText('C:\\synthetic\\repo')
    await user.type(screen.getByRole('textbox', { name: 'Type RESET to confirm' }), 'RESET')
    await user.click(screen.getByRole('button', { name: 'Delete local setup' }))
    expect(await screen.findByRole('button', { name: 'Get started' })).toBeVisible()
    expect(screen.queryByRole('dialog', { name: 'Sync settings' })).not.toBeInTheDocument()
  })
})
