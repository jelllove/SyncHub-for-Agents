import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SyncNoticesDialog } from './SyncNoticesDialog'
import { setLocale } from './i18n'

const notices = {
  version: 1, fingerprint: 'exact-set', finishedAt: '2026-10-03T10:00:00Z',
  skipped: 2, blocked: 1, detailsAvailable: true, reviewed: false,
  issues: [
    { kind: 'skipped', resourceKey: 'demo/source', path: 'cache.tmp', code: 'unapproved-link', message: 'Link needs approval', bytes: 12, reviewable: true },
    { kind: 'skipped', resourceKey: 'demo/source', path: 'cache.tmp', code: 'unapproved-link', message: 'Link needs approval', bytes: 12, reviewable: true },
    { kind: 'blocked', resourceKey: 'demo/config', path: 'secret.json', code: 'invalid-path', message: 'Unsafe destination', bytes: 20, reviewable: true },
    { kind: 'error', resourceKey: 'demo/config', path: 'settings.json', code: 'restore-failed', message: 'Write failed', bytes: 0, reviewable: false },
  ],
}

describe('SyncNoticesDialog', () => {
  afterEach(() => act(() => setLocale('en')))

  it('retains filtered diagnostics and explicit acknowledgement when translated', async () => {
    const acknowledge = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<SyncNoticesDialog notices={notices} initialFilter="blocked" acknowledge={acknowledge} dismiss={vi.fn()} />)
    act(() => setLocale('zh-CN'))
    expect(screen.getByRole('dialog', { name: '同步诊断日志' })).toBeVisible()
    expect(screen.getByRole('combobox', { name: '显示问题' })).toHaveValue('blocked')
    expect(screen.getByText('invalid-path')).toBeVisible()
    expect(screen.getByText('Unsafe destination')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '审阅安全提示' }))
    expect(acknowledge).not.toHaveBeenCalled()
    act(() => setLocale('en'))
    expect(screen.getByRole('button', { name: 'Confirm acknowledgement' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(acknowledge).toHaveBeenCalledExactlyOnceWith('exact-set')
  })

  it('retains duplicates and exposes exact resource, path, reason and code with filters', async () => {
    const user = userEvent.setup()
    render(<SyncNoticesDialog notices={notices} initialFilter="skipped" acknowledge={vi.fn()} dismiss={vi.fn()} />)
    const dialog = screen.getByRole('dialog', { name: 'Sync diagnostics log' })
    expect(within(dialog).getAllByText('cache.tmp')).toHaveLength(2)
    expect(within(dialog).getAllByText('demo/source')).toHaveLength(2)
    expect(within(dialog).getAllByText('unapproved-link')).toHaveLength(2)
    expect(within(dialog).getAllByText('Link needs approval')).toHaveLength(2)
    expect(within(dialog).queryByText('secret.json')).not.toBeInTheDocument()
    await user.selectOptions(within(dialog).getByRole('combobox', { name: 'Show issues' }), 'blocked')
    expect(within(dialog).getByText('secret.json')).toBeVisible()
    expect(within(dialog).getByText('invalid-path')).toBeVisible()
    await user.selectOptions(within(dialog).getByRole('combobox', { name: 'Show issues' }), 'all')
    expect(within(dialog).getByText('restore-failed')).toBeVisible()
    expect(within(dialog).getByText(/Operational issue; acknowledgement does not clear it/)).toBeVisible()
  })

  it('shows all notices for explicit confirmation and sends only the displayed fingerprint', async () => {
    const user = userEvent.setup()
    const acknowledge = vi.fn().mockResolvedValue(undefined)
    render(<SyncNoticesDialog notices={notices} initialFilter="blocked" acknowledge={acknowledge} dismiss={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    expect(screen.getAllByText('cache.tmp')).toHaveLength(2)
    expect(screen.getByText('secret.json')).toBeVisible()
    expect(acknowledge).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(acknowledge).toHaveBeenCalledExactlyOnceWith('exact-set')
    expect(screen.getAllByText('cache.tmp')).toHaveLength(2)
  })

  it('keeps stale/persistence errors visible and retains records for retry', async () => {
    const user = userEvent.setup()
    const acknowledge = vi.fn().mockRejectedValue(new Error('Notice set changed; reopen log'))
    render(<SyncNoticesDialog notices={notices} initialFilter="all" acknowledge={acknowledge} dismiss={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Review safety notices' }))
    await user.click(screen.getByRole('button', { name: 'Confirm acknowledgement' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Notice set changed; reopen log')
    expect(screen.getAllByText('cache.tmp')).toHaveLength(2)
    expect(screen.getByRole('button', { name: 'Confirm acknowledgement' })).toBeEnabled()
  })

  it('reports unavailable legacy details without inventing preview entries or allowing acknowledgement', () => {
    render(<SyncNoticesDialog notices={{ ...notices, detailsAvailable: false, issues: [] }} initialFilter="blocked" acknowledge={vi.fn()} dismiss={vi.fn()} />)
    expect(screen.getByText(/Historical cycle details are unavailable/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Review safety notices' })).not.toBeInTheDocument()
  })

  it('shows reviewed state while retaining the log and operational failures', () => {
    render(<SyncNoticesDialog notices={{ ...notices, reviewed: true }} initialFilter="all" acknowledge={vi.fn()} dismiss={vi.fn()} />)
    expect(screen.getByText('Safety notices reviewed')).toBeVisible()
    expect(screen.getByText('Write failed')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Review safety notices' })).not.toBeInTheDocument()
  })

  it('handles Escape independently and restores focus to the invoking counter', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const dismiss = vi.fn()
    const { unmount } = render(<SyncNoticesDialog notices={notices} initialFilter="all" acknowledge={vi.fn()} dismiss={dismiss} />)
    expect(screen.getByRole('button', { name: 'Close log' })).toHaveFocus()
    fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))
    expect(dismiss).toHaveBeenCalledOnce()
    unmount()
    await waitFor(() => expect(trigger).toHaveFocus())
    trigger.remove()
  })
})
