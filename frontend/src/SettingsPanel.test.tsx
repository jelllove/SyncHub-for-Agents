import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Snapshot } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { SettingsPanel } from './SettingsPanel'
import { normalizeSnapshot } from './desktopState'

const startAtLogin = vi.fn()

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  PreviewCustomResource: vi.fn(),
  StartAtLogin: () => startAtLogin(),
}))

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
        save={vi.fn().mockResolvedValue(true)}
      />,
    )

    expect(screen.getByText(/Last refreshed/)).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByText('safe files')).toBeInTheDocument()
  })
})
