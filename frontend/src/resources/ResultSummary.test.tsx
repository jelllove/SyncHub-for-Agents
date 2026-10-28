import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ResultSummary } from './ResultSummary'
import type { Progress } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { setLocale } from '../i18n'

const progress: Progress = {
  stage: 'complete', label: '', percentage: 100, completedActions: 0, totalActions: 0,
  blockedFiles: 0, pushed: false, restored: 0, reinstalled: 0, skipped: 0,
  conflicts: 0, pendingInstalls: 0, needsAttention: false,
}

const notices = {
  version: 1, fingerprint: 'issues', finishedAt: '', skipped: 0, blocked: 0,
  detailsAvailable: true, reviewed: false, issues: [],
}

describe('ResultSummary', () => {
  afterEach(() => act(() => setLocale('en')))

  it('does not show skipped or blocked cards for a normal completed cycle', () => {
    render(<ResultSummary progress={{ ...progress, skipped: 250 }} notices={notices} openLog={vi.fn()} />)
    expect(screen.queryByText('Skipped')).not.toBeInTheDocument()
    expect(screen.queryByText('Blocked')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('opens actionable operational issues and retains the conflict count', async () => {
    const openLog = vi.fn()
    render(<ResultSummary progress={{ ...progress, conflicts: 1, needsAttention: true }}
      notices={{ ...notices, issues: [{ kind: 'error', code: 'restore-failed', resourceKey: 'demo', path: 'settings.json', message: 'Access denied', bytes: 0, reviewable: false }] }}
      openLog={openLog} />)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Issues: 1. Open sync diagnostics log' }))
    expect(openLog).toHaveBeenCalledExactlyOnceWith('all')
    expect(screen.getByText('Conflicts')).toBeVisible()
    expect(screen.queryByText('Skipped')).not.toBeInTheDocument()
    act(() => setLocale('zh-CN'))
    expect(screen.getByRole('button', { name: '问题：1。打开同步诊断日志' })).toBeVisible()
    expect(screen.queryByText('已跳过')).not.toBeInTheDocument()
  })

  it('keeps legacy unavailable details accessible without inventing counts', async () => {
    const openLog = vi.fn()
    render(<ResultSummary progress={progress} notices={{ ...notices, skipped: 50, detailsAvailable: false }} openLog={openLog} />)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Open historical sync diagnostics log' }))
    expect(openLog).toHaveBeenCalledExactlyOnceWith('all')
    expect(screen.queryByText('50')).not.toBeInTheDocument()
    expect(screen.queryByText('Skipped')).not.toBeInTheDocument()
  })
})
