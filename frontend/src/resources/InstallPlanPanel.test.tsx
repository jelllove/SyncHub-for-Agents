import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { InstallPlanPanel } from './InstallPlanPanel'
import { LanguageSwitcher } from '../LanguageSwitcher'
import { setLocale } from '../i18n'

describe('InstallPlanPanel', () => {
  afterEach(() => act(() => setLocale('en')))
  it('localizes approval and retry labels while preserving commands, IDs and backend errors', async () => {
    const approve = vi.fn().mockResolvedValue(undefined)
    const retry = vi.fn().mockResolvedValue(undefined)
    const plan = {
      id: 'plan-raw', approved: false, operations: [{
        id: 'operation-raw', adapter: 'copilot-plugin', source: 'my-plugin',
        kind: 'update', executable: 'copilot', args: ['plugin', 'update', 'my-plugin'],
        workingDir: 'C:\\synthetic', error: '',
      }],
    }
    const user = userEvent.setup()
    const { rerender } = render(<><LanguageSwitcher /><InstallPlanPanel plan={plan} busy={false} approve={approve} retry={retry} /></>)
    await user.selectOptions(screen.getByRole('combobox', { name: /Language|语言/ }), 'zh-CN')
    expect(screen.getByRole('heading', { name: '恢复智能体集成' })).toBeVisible()
    expect(screen.getByText('可执行程序：copilot')).toBeVisible()
    expect(approve).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: '批准并同步' }))
    expect(approve).toHaveBeenCalledWith('plan-raw')
    rerender(<><LanguageSwitcher /><InstallPlanPanel plan={{ ...plan, operations: [{ ...plan.operations[0], error: 'access denied: C:\\synthetic' }] }} busy={false} approve={approve} retry={retry} /></>)
    expect(screen.getByText('access denied: C:\\synthetic')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试失败的操作' }))
    expect(retry).toHaveBeenCalledWith('plan-raw')
    await user.selectOptions(screen.getByRole('combobox', { name: /Language|语言/ }), 'en')
    expect(screen.getByRole('button', { name: 'Retry failed operations' })).toBeVisible()
  })

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
