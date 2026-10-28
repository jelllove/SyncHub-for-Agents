import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { InstallPlanPanel } from './InstallPlanPanel'

describe('InstallPlanPanel', () => {
  it('shows the trusted executable separately from its arguments', () => {
    render(
      <InstallPlanPanel
        plan={{
          id: 'plan-1',
          approved: false,
          operations: [{
            id: 'operation-1',
            adapter: 'copilot-plugin',
            source: 'wiqd@wiqd',
            kind: 'update',
            executable: 'copilot',
            args: ['plugin', 'update', 'wiqd@wiqd'],
            workingDir: '',
            error: '',
          }],
        }}
        busy={false}
        approve={vi.fn()}
        retry={vi.fn()}
      />,
    )

    expect(screen.getByText('Executable: copilot')).toBeInTheDocument()
    expect(screen.getByText('Arguments: ["plugin","update","wiqd@wiqd"]')).toBeInTheDocument()
  })

  it('shows retry action when operations contain failures', async () => {
    const retry = vi.fn().mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(
      <InstallPlanPanel
        plan={{
          id: 'plan-1',
          approved: false,
          operations: [{
            id: 'operation-1',
            adapter: 'copilot-plugin',
            source: 'wiqd@wiqd',
            kind: 'update',
            executable: 'copilot',
            args: ['plugin', 'update', 'wiqd@wiqd'],
            workingDir: '',
            error: 'access denied',
          }],
        }}
        busy={false}
        approve={vi.fn()}
        retry={retry}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Retry failed operations' }))
    expect(retry).toHaveBeenCalledWith('plan-1')
  })
})
