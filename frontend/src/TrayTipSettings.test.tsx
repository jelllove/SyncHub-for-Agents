import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TrayTipSettings } from './TrayTipSettings'

const api = vi.hoisted(() => ({
  get: vi.fn(),
  set: vi.fn(),
  unsubscribe: vi.fn(),
  changed: undefined as undefined | ((event: { sender?: string }) => void),
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  TrayTipsEnabled: () => api.get(),
  SetTrayTipsEnabled: (enabled: boolean) => api.set(enabled),
}))
vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (_name: string, handler: (event: { sender?: string }) => void) => {
      api.changed = handler
      return api.unsubscribe
    },
  },
}))

describe('TrayTipSettings', () => {
  beforeEach(() => {
    api.get.mockReset().mockResolvedValue(true)
    api.set.mockReset().mockResolvedValue(undefined)
    api.unsubscribe.mockReset()
    api.changed = undefined
  })

  it('loads saved opt-out and reenables it independently of sync settings', async () => {
    api.get.mockResolvedValueOnce(false).mockResolvedValue(true)
    const view = render(<TrayTipSettings />)
    const checkbox = await screen.findByRole('checkbox', { name: /Show activity tips/ })
    await waitFor(() => expect(checkbox).toBeEnabled())
    expect(checkbox).not.toBeChecked()
    await userEvent.click(checkbox)
    expect(api.set).toHaveBeenCalledWith(true)
    await waitFor(() => expect(checkbox).toBeChecked())
    view.unmount()
    expect(api.unsubscribe).toHaveBeenCalled()
  })

  it('reports read failures and offers retry instead of treating them as disabled', async () => {
    api.get.mockRejectedValueOnce(new Error('Settings file is unreadable')).mockResolvedValue(false)
    render(<TrayTipSettings />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Settings file is unreadable')
    expect(screen.getByRole('checkbox', { name: /Show activity tips/ })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Retry activity tip settings' }))
    await waitFor(() => expect(screen.getByRole('checkbox', { name: /Show activity tips/ })).toBeEnabled())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps the old preference when saving fails and offers retry', async () => {
    api.set.mockRejectedValueOnce(new Error('Could not write preferences'))
    render(<TrayTipSettings />)
    const checkbox = screen.getByRole('checkbox', { name: /Show activity tips/ })
    await waitFor(() => expect(checkbox).toBeEnabled())
    await userEvent.click(checkbox)
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not write preferences')
    expect(checkbox).toBeChecked()
    expect(screen.getByRole('button', { name: 'Retry activity tip settings' })).toBeEnabled()
  })

  it('refreshes when the popup disables tips and ignores sender-controlled events', async () => {
    render(<TrayTipSettings />)
    const checkbox = screen.getByRole('checkbox', { name: /Show activity tips/ })
    await waitFor(() => expect(checkbox).toBeEnabled())
    act(() => api.changed?.({ sender: 'other-window' }))
    expect(api.get).toHaveBeenCalledTimes(1)
    api.get.mockResolvedValue(false)
    act(() => api.changed?.({}))
    await waitFor(() => expect(checkbox).not.toBeChecked())
  })
})
