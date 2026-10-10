import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TrayTip } from './TrayTip'

const events = vi.hoisted(() => ({
  on: vi.fn(),
  emit: vi.fn(),
  unsubscribe: vi.fn(),
  handlers: new Map<string, (event: { data: unknown; sender?: string }) => void>(),
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: (event: { data: unknown }) => void) => {
      events.handlers.set(name, handler)
      events.on(name)
      return events.unsubscribe
    },
    Emit: (name: string) => events.emit(name),
  },
}))

function send(kind: string, revision = 1, title = kind) {
  act(() => {
    events.handlers.get('tray-tip:status')?.({
      data: { kind, revision, title, message: 'Synthetic synchronization status.' },
    })
  })
}

describe('TrayTip', () => {
  beforeEach(() => {
    events.handlers.clear()
    events.on.mockReset()
    events.emit.mockReset().mockResolvedValue(undefined)
    events.unsubscribe.mockReset()
  })

  it.each([
    ['pulling', 'Pulling'], ['pushing', 'Pushing'], ['scanning', 'Scanning'],
    ['comparing', 'Comparing'], ['applying', 'Applying'],
    ['done', 'Sync complete'], ['warning', 'Needs attention'], ['error', 'Sync error'],
  ])('shows a distinct status icon and text for %s', (kind, title) => {
    render(<TrayTip />)
    send(kind, 1, title)
    expect(screen.getByRole('heading', { name: title })).toBeVisible()
    expect(screen.getByRole('img', { name: `${title} status` })).toBeInTheDocument()
    expect(screen.getByText('Synthetic synchronization status.')).toBeVisible()
    expect(screen.getByRole(kind === 'error' || kind === 'warning' ? 'alert' : 'status')).toBeInTheDocument()
  })

  it('subscribes before readiness, opens the app only on request and cleans up listeners', async () => {
    const view = render(<TrayTip />)
    expect(events.on).toHaveBeenCalledWith('tray-tip:status')
    expect(events.emit).toHaveBeenCalledWith('tray-tip:ready')
    expect(events.on.mock.invocationCallOrder[0]).toBeLessThan(events.emit.mock.invocationCallOrder[0])
    send('error', 1, 'Sync error')
    await userEvent.click(screen.getByRole('button', { name: 'Open SyncHub for Agents' }))
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss activity tip' }))
    expect(events.emit).toHaveBeenCalledWith('tray-tip:open')
    expect(events.emit).toHaveBeenCalledWith('tray-tip:dismiss')
    view.unmount()
    expect(events.unsubscribe).toHaveBeenCalledOnce()
  })

  it('ignores stale progress without hiding the current error', () => {
    render(<TrayTip />)
    send('error', 9, 'Sync error')
    send('pulling', 8, 'Pulling')
    expect(screen.getByRole('heading', { name: 'Sync error' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Pulling' })).not.toBeInTheDocument()
  })

  it('reports malformed status and action failures rather than presenting success', async () => {
    render(<TrayTip />)
    send('unsupported-state')
    expect(screen.getByRole('alert')).toHaveTextContent('Activity status could not be read')
    send('error', 2, 'Sync error')
    events.emit.mockRejectedValueOnce(new Error('Desktop connection is unavailable'))
    await userEvent.click(screen.getByRole('button', { name: 'Open SyncHub for Agents' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Desktop connection is unavailable')
  })
})
