import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ResetPanel } from './ResetPanel'
import { setLocale } from './i18n'

afterEach(() => act(() => setLocale('en')))

const api = vi.hoisted(() => ({ preview: vi.fn(), reset: vi.fn() }))
vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  ResetPreview: () => api.preview(),
  ResetLocalSetup: (confirmation: string, repo: string) => api.reset(confirmation, repo),
}))

beforeEach(() => {
  vi.resetAllMocks()
  api.preview.mockResolvedValue({ repoPath: 'C:\\synthetic\\.synchub\\repo' })
  api.reset.mockResolvedValue(undefined)
})

it('requires exact confirmation and shows the actual clone path', async () => {
  const user = userEvent.setup()
  const complete = vi.fn()
  render(<ResetPanel busy={false} complete={complete} />)
  await user.click(screen.getByRole('button', { name: 'Reset and start over' }))
  expect(await screen.findByText('C:\\synthetic\\.synchub\\repo')).toBeVisible()
  const confirm = screen.getByRole('button', { name: 'Delete local setup' })
  expect(confirm).toBeDisabled()
  await user.type(screen.getByRole('textbox', { name: 'Type RESET to confirm' }), 'RESET')
  await user.click(confirm)
  expect(api.reset).toHaveBeenCalledWith('RESET', 'C:\\synthetic\\.synchub\\repo')
  expect(complete).toHaveBeenCalledOnce()
})

it('retains the exact RESET safety token and reviewed path in Chinese', async () => {
  const user = userEvent.setup()
  render(<ResetPanel busy={false} complete={vi.fn()} />)
  act(() => setLocale('zh-CN'))
  await user.click(screen.getByRole('button', { name: '重置并重新开始' }))
  expect(await screen.findByText('C:\\synthetic\\.synchub\\repo')).toBeVisible()
  expect(screen.getByText(/此操作无法撤销/)).toBeVisible()
  expect(screen.getByRole('button', { name: '删除本地设置' })).toBeDisabled()
  const input = screen.getByRole('textbox', { name: '输入 RESET 以确认' })
  await user.type(input, '重置')
  expect(screen.getByRole('button', { name: '删除本地设置' })).toBeDisabled()
  await user.clear(input)
  await user.type(input, 'RESET')
  act(() => setLocale('en'))
  expect(api.preview).toHaveBeenCalledTimes(1)
  expect(api.reset).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Delete local setup' }))
  expect(api.reset).toHaveBeenCalledExactlyOnceWith('RESET', 'C:\\synthetic\\.synchub\\repo')
})

it('keeps errors visible without claiming reset succeeded', async () => {
  api.reset.mockRejectedValue(new Error('unsafe repository path'))
  const user = userEvent.setup()
  const complete = vi.fn()
  render(<ResetPanel busy={false} complete={complete} />)
  await user.click(screen.getByRole('button', { name: 'Reset and start over' }))
  await screen.findByText('C:\\synthetic\\.synchub\\repo')
  await user.type(screen.getByRole('textbox', { name: 'Type RESET to confirm' }), 'RESET')
  await user.click(screen.getByRole('button', { name: 'Delete local setup' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('unsafe repository path')
  expect(complete).not.toHaveBeenCalled()
})
