import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'
import { ResetPanel } from './ResetPanel'

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
