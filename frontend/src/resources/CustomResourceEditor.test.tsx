import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { CustomResourceEditor } from './CustomResourceEditor'
import { setLocale } from '../i18n'

const api = vi.hoisted(() => ({ preview: vi.fn() }))
vi.mock('../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  PreviewCustomResource: (candidate: unknown) => api.preview(candidate),
}))

afterEach(() => act(() => setLocale('en')))

it('keeps resource identifiers and category payloads stable while localizing labels and exposing preview errors', async () => {
  api.preview.mockRejectedValue(new Error('permission denied: C:\\synthetic\\instructions'))
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(<CustomResourceEditor platform="windows" resources={[]} onChange={onChange} />)
  await user.type(screen.getByRole('textbox', { name: 'Stable ID' }), 'my-instructions')
  await user.type(screen.getByRole('textbox', { name: 'Source on this computer' }), 'C:\\synthetic\\instructions')
  await user.type(screen.getByRole('textbox', { name: 'Restore target' }), 'C:\\synthetic\\restore')
  act(() => setLocale('zh-CN'))
  expect(screen.getByRole('textbox', { name: '稳定 ID' })).toHaveValue('my-instructions')
  expect(screen.getByRole('combobox', { name: '类别' })).toHaveValue('instructions')
  await user.selectOptions(screen.getByRole('combobox', { name: '类别' }), 'skills')
  await user.click(screen.getByRole('button', { name: '预览' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('permission denied: C:\\synthetic\\instructions')
  expect(screen.getByRole('button', { name: '添加资源' })).toBeDisabled()
  act(() => setLocale('en'))
  expect(screen.getByRole('alert')).toHaveTextContent('permission denied: C:\\synthetic\\instructions')
  expect(api.preview).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
    id: 'my-instructions', category: 'skills', strategy: 'file-tree',
    paths: { windows: 'C:\\synthetic\\instructions' }, targets: { windows: 'C:\\synthetic\\restore' },
  }))
  expect(onChange).not.toHaveBeenCalled()
})
