import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Status } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/updater/models'
import { UpdatePanel } from './UpdatePanel'

const api = vi.hoisted(() => ({
  status: vi.fn(),
  check: vi.fn(),
  automatic: vi.fn(),
  restart: vi.fn(),
  openURL: vi.fn(),
  on: vi.fn(),
  unsubscribe: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: (...args: unknown[]) => api.openURL(...args) },
  Events: { On: (...args: unknown[]) => api.on(...args) },
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  UpdateStatus: () => api.status(),
  CheckForUpdates: () => api.check(),
  SetAutomaticUpdates: (enabled: boolean) => api.automatic(enabled),
  RestartToUpdate: () => api.restart(),
}))

function status(overrides: Partial<Status> = {}): Status {
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

function ready(overrides: Partial<Status> = {}) {
  return status({
    latestVersion: 'v1.1.0',
    phase: 'ready',
    releaseURL: 'https://github.com/qinqingxu/synchub-for-agents/releases/tag/v1.1.0',
    ...overrides,
  })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function publish(next: Status) {
  const calls = api.on.mock.calls
  const listener = calls[calls.length - 1][1] as (event: { data: Status }) => void
  act(() => listener({ data: next }))
}

describe('UpdatePanel', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    api.status.mockResolvedValue(status())
    api.check.mockResolvedValue(status({ phase: 'upToDate' }))
    api.automatic.mockImplementation((enabled: boolean) => Promise.resolve(status({ automatic: enabled })))
    api.restart.mockResolvedValue(undefined)
    api.openURL.mockResolvedValue(undefined)
    api.on.mockReturnValue(api.unsubscribe)
  })

  it('loads the current version and defaults automatic updates to enabled', async () => {
    const initial = deferred<Status>()
    api.status.mockReturnValue(initial.promise)
    render(<UpdatePanel syncBusy={false} />)

    expect(screen.getByRole('checkbox', { name: 'Automatic updates' })).toBeChecked()
    expect(screen.getByRole('checkbox')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled()
    await act(async () => initial.resolve(status()))

    expect(screen.getByText('v1.0.0')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeEnabled()
    expect(screen.getByText(/Saved immediately on this device/)).toBeInTheDocument()
    expect(screen.getByText(/Quit from the tray/)).toHaveTextContent('No forced background restarts.')
    expect(api.check).not.toHaveBeenCalled()
    expect(api.on).toHaveBeenCalledWith('updater:status', expect.any(Function))
  })

  it.each([
    ['checking', 'Checking for updates…', true],
    ['downloading', 'Downloading and verifying update…', true],
    ['upToDate', "You're up to date.", false],
    ['available', 'Version v1.1.0 is available.', false],
  ])('shows %s status and protects in-progress checks', async (phase, message, busy) => {
    api.status.mockResolvedValue(status({ phase, latestVersion: 'v1.1.0' }))
    render(<UpdatePanel syncBusy={false} />)

    expect(await screen.findByText(message)).toBeInTheDocument()
    const button = screen.getByRole('button', { name: 'Check now' })
    if (busy) expect(button).toBeDisabled()
    else expect(button).toBeEnabled()
    expect(screen.queryByRole('button', { name: 'Restart to update' })).not.toBeInTheDocument()
  })

  it('checks on demand and displays the last checked time', async () => {
    api.check.mockResolvedValue(status({ phase: 'upToDate', lastChecked: '2026-09-14T09:00:00Z' }))
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('button', { name: 'Check now' }))

    expect(api.check).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('status')).toHaveTextContent("You're up to date.")
    expect(screen.getByText(/Last checked/)).toBeInTheDocument()
  })

  it.each(['not-a-date', '0001-01-01T00:00:00Z'])('ignores invalid or missing timestamps (%s)', async (lastChecked) => {
    api.status.mockResolvedValue(status({ lastChecked }))
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')

    expect(screen.queryByText(/Last checked/)).not.toBeInTheDocument()
  })

  it('offers an explicit restart after a ready event and prevents duplicate requests', async () => {
    const restart = deferred<void>()
    api.restart.mockReturnValue(restart.promise)
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    publish(ready())

    expect(screen.getByRole('status')).toHaveTextContent('Version v1.1.0 is ready to install.')
    expect(api.restart).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Restart to update' }))
    expect(api.restart).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Restarting…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled()
    await act(async () => restart.resolve(undefined))
  })

  it.each(['available', 'ready'])('offers only manual installation on unsupported platforms (%s)', async (phase) => {
    const latest = ready({ phase, supported: false })
    api.status.mockResolvedValue(latest)
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')

    expect(screen.getByRole('status')).toHaveTextContent('Version v1.1.0 is available.')
    expect(screen.getByText(/Automatic installation is not supported/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Restart/ })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Download release' }))
    expect(api.openURL).toHaveBeenCalledWith(latest.releaseURL)
    expect(api.restart).not.toHaveBeenCalled()
  })

  it('does not construct a download URL when the backend supplies none', async () => {
    api.status.mockResolvedValue(ready({ supported: false, releaseURL: '' }))
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')

    expect(screen.queryByRole('button', { name: /release/ })).not.toBeInTheDocument()
    expect(api.openURL).not.toHaveBeenCalled()
  })

  it('disables updates for developer builds', async () => {
    api.status.mockResolvedValue(ready({ currentVersion: 'dev' }))
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('dev')

    expect(screen.getByText(/Updates are disabled for developer builds/)).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: /Restart|release/ })).not.toBeInTheDocument()
  })

  it('saves the automatic preference immediately in both directions', async () => {
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('checkbox', { name: 'Automatic updates' }))

    expect(api.automatic).toHaveBeenLastCalledWith(false)
    expect(screen.getByRole('checkbox')).not.toBeChecked()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(api.automatic).toHaveBeenLastCalledWith(true)
    expect(screen.getByRole('checkbox')).toBeChecked()
  })

  it('keeps the saved preference on failure and clears the error after retry', async () => {
    api.automatic.mockRejectedValueOnce(new Error('Could not save update preference'))
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('checkbox'))

    expect(screen.getByRole('alert')).toHaveTextContent('Could not save update preference')
    expect(screen.getByRole('checkbox')).toBeChecked()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(screen.getByRole('checkbox')).not.toBeChecked()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('recovers from an initial status failure using Check now', async () => {
    api.status.mockRejectedValue(new Error('Update service unavailable'))
    render(<UpdatePanel syncBusy={false} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Update service unavailable')
    expect(screen.getByText('Unavailable')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Check now' }))
    expect(screen.getByText('v1.0.0')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows check failures and clears them on a successful retry', async () => {
    api.check.mockRejectedValueOnce('Network unavailable')
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('button', { name: 'Check now' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Network unavailable')
    await userEvent.click(screen.getByRole('button', { name: 'Check now' }))
    expect(screen.getByRole('status')).toHaveTextContent("You're up to date.")
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows backend errors and clears them when live status recovers', async () => {
    api.status.mockResolvedValue(status({ phase: 'error', error: 'Update checksum did not match' }))
    render(<UpdatePanel syncBusy={false} />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Update checksum did not match')

    publish(status({ phase: 'checking' }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    publish(status({ phase: 'upToDate' }))
    expect(screen.getByRole('status')).toHaveTextContent("You're up to date.")
  })

  it('supplies a recovery message when an error status has no detail', async () => {
    api.status.mockResolvedValue(status({ phase: 'error' }))
    render(<UpdatePanel syncBusy={false} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Please try checking for updates again.')
  })

  it('shows restart failures and permits a successful retry', async () => {
    api.status.mockResolvedValue(ready())
    api.restart.mockRejectedValueOnce(new Error('Synchronization is still running'))
    render(<UpdatePanel syncBusy={false} />)
    await userEvent.click(await screen.findByRole('button', { name: 'Restart to update' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Synchronization is still running')
    await userEvent.click(screen.getByRole('button', { name: 'Restart to update' }))
    expect(api.restart).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('disables restart while sync is busy and enables it when sync finishes', async () => {
    api.status.mockResolvedValue(ready())
    const panel = render(<UpdatePanel syncBusy />)
    const restart = await screen.findByRole('button', { name: 'Restart to update' })

    expect(restart).toBeDisabled()
    expect(screen.getByText(/Wait for synchronization/)).toBeInTheDocument()
    await userEvent.click(restart)
    expect(api.restart).not.toHaveBeenCalled()
    panel.rerender(<UpdatePanel syncBusy={false} />)
    expect(restart).toBeEnabled()
    expect(screen.queryByText(/Wait for synchronization/)).not.toBeInTheDocument()
  })

  it('exposes browser errors and opens only the backend release URL on retry', async () => {
    api.status.mockResolvedValue(ready())
    api.openURL.mockRejectedValueOnce(new Error('Could not open browser'))
    render(<UpdatePanel syncBusy={false} />)
    await userEvent.click(await screen.findByRole('button', { name: 'View release' }))

    expect(screen.getByRole('alert')).toHaveTextContent('Could not open browser')
    await userEvent.click(screen.getByRole('button', { name: 'View release' }))
    expect(api.openURL).toHaveBeenLastCalledWith(ready().releaseURL)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('does not overwrite newer events with the initial status response', async () => {
    const initial = deferred<Status>()
    api.status.mockReturnValue(initial.promise)
    render(<UpdatePanel syncBusy={false} />)
    publish(ready())
    await act(async () => initial.resolve(status()))

    expect(screen.getByRole('button', { name: 'Restart to update' })).toBeInTheDocument()
  })

  it('does not roll back a ready event when the check request returns older status', async () => {
    const check = deferred<Status>()
    api.check.mockReturnValue(check.promise)
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('button', { name: 'Check now' }))
    publish(ready())
    await act(async () => check.resolve(status({ phase: 'checking' })))

    expect(screen.getByRole('button', { name: 'Restart to update' })).toBeEnabled()
    expect(screen.getByRole('status')).toHaveTextContent('Version v1.1.0 is ready to install.')
  })

  it('retains live download progress when the preference save returns', async () => {
    const automatic = deferred<Status>()
    api.automatic.mockReturnValue(automatic.promise)
    render(<UpdatePanel syncBusy={false} />)
    await screen.findByText('v1.0.0')
    await userEvent.click(screen.getByRole('checkbox'))
    expect(screen.getByRole('checkbox')).toBeDisabled()
    publish(ready())
    await act(async () => automatic.resolve(status({ automatic: false })))

    expect(screen.getByRole('checkbox')).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Restart to update' })).toBeEnabled()
  })

  it('unsubscribes on unmount and ignores a late initial response after reopening', async () => {
    const initial = deferred<Status>()
    api.status.mockReturnValueOnce(initial.promise).mockResolvedValue(ready())
    const first = render(<UpdatePanel syncBusy={false} />)
    first.unmount()
    expect(api.unsubscribe).toHaveBeenCalledTimes(1)

    const second = render(<UpdatePanel syncBusy={false} />)
    await screen.findByRole('button', { name: 'Restart to update' })
    await act(async () => initial.resolve(status({ phase: 'error', error: 'Stale error' })))

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(api.on).toHaveBeenCalledTimes(2)
    second.unmount()
    await waitFor(() => expect(api.unsubscribe).toHaveBeenCalledTimes(2))
  })
})
