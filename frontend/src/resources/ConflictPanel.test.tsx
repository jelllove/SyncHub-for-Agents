import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ConflictPanel } from './ConflictPanel'

const conflicts = [
  {
    id: 'conflict-1',
    revision: 'rev-1',
    resourceKey: 'claude/settings',
    path: 'agents/_portable/config/providers/claude/config/settings/settings.json',
    createdAt: '2026-08-19T06:00:00Z',
  },
  {
    id: 'conflict-2',
    revision: 'rev-2',
    resourceKey: 'copilot/settings',
    path: 'agents/_portable/config/providers/copilot/config/settings/settings.json',
    createdAt: '2026-08-19T06:10:00Z',
  },
]

describe('ConflictPanel', () => {
  it('requires complete selections before applying all', async () => {
    const applyBatch = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={null}
        busy={false}
        applyBatch={applyBatch}
        retryBatch={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    const applyButton = screen.getByRole('button', { name: 'Apply all and synchronize' })
    expect(applyButton).toBeDisabled()
    const localButtons = screen.getAllByRole('button', { name: 'Use local' })
    await user.click(localButtons[0])
    expect(applyButton).toBeDisabled()
    await user.click(localButtons[1])
    expect(applyButton).toBeEnabled()

    await user.click(applyButton)
    expect(applyBatch).toHaveBeenCalledTimes(1)
    expect(applyBatch).toHaveBeenCalledWith([
      { id: 'conflict-1', revision: 'rev-1', choice: 'local', content: '' },
      { id: 'conflict-2', revision: 'rev-2', choice: 'local', content: '' },
    ])
  })

  it('offers retry when the previous batch failed', async () => {
    const retryBatch = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={{ id: 'batch-1', status: 'failed', selected: 2, error: 'stale revision' }}
        busy={false}
        applyBatch={vi.fn().mockResolvedValue(undefined)}
        retryBatch={retryBatch}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Retry failed batch' }))
    expect(retryBatch).toHaveBeenCalledWith('batch-1')
  })

  it('applies local choice to all conflicts in one click', async () => {
    const applyBatch = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={null}
        busy={false}
        applyBatch={applyBatch}
        retryBatch={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Use local for all' }))
    const applyButton = screen.getByRole('button', { name: 'Apply all and synchronize' })
    expect(applyButton).toBeEnabled()
    await user.click(applyButton)

    expect(applyBatch).toHaveBeenCalledWith([
      { id: 'conflict-1', revision: 'rev-1', choice: 'local', content: '' },
      { id: 'conflict-2', revision: 'rev-2', choice: 'local', content: '' },
    ])
  })

  it('applies remote choice to all conflicts in one click', async () => {
    const applyBatch = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={null}
        busy={false}
        applyBatch={applyBatch}
        retryBatch={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Use remote for all' }))
    await user.click(screen.getByRole('button', { name: 'Apply all and synchronize' }))

    expect(applyBatch).toHaveBeenCalledWith([
      { id: 'conflict-1', revision: 'rev-1', choice: 'remote', content: '' },
      { id: 'conflict-2', revision: 'rev-2', choice: 'remote', content: '' },
    ])
  })

  it('clears all batch selections and disables apply', async () => {
    const user = userEvent.setup()
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={null}
        busy={false}
        applyBatch={vi.fn().mockResolvedValue(undefined)}
        retryBatch={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    const applyButton = screen.getByRole('button', { name: 'Apply all and synchronize' })
    await user.click(screen.getByRole('button', { name: 'Use local for all' }))
    expect(applyButton).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Clear all' }))
    expect(applyButton).toBeDisabled()
  })

  it('disables quick-action buttons while busy', () => {
    render(
      <ConflictPanel
        conflicts={conflicts}
        resolution={null}
        busy
        applyBatch={vi.fn().mockResolvedValue(undefined)}
        retryBatch={vi.fn().mockResolvedValue(undefined)}
      />,
    )

    expect(screen.getByRole('button', { name: 'Use local for all' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Use remote for all' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Clear all' })).toBeDisabled()
  })
})
