import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { FirstSyncDialog, type FirstSyncStrategy } from './FirstSyncDialog'
import { setLocale } from './i18n'

afterEach(() => act(() => setLocale('en')))

it('preserves explicit strategy selection and backend errors across language changes', async () => {
  const confirm = vi.fn().mockRejectedValue(new Error('synthetic repository failure'))
  const user = userEvent.setup()
  render(<FirstSyncDialog busy={false} paused={true} repositoryUrl="git@github.com:synthetic/repo.git" confirm={confirm} dismiss={vi.fn()} />)
  act(() => setLocale('zh-CN'))
  expect(screen.getByRole('dialog', { name: '选择首次同步策略' })).toBeVisible()
  expect(screen.getByRole('button', { name: '开始首次同步' })).toBeDisabled()
  expect(screen.getByText(/云端文件可能被替换或删除/)).toBeVisible()
  await user.click(screen.getByRole('radio', { name: /使用本地/ }))
  act(() => setLocale('en'))
  expect(screen.getByRole('radio', { name: /Use local/ })).toBeChecked()
  expect(confirm).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Start first sync' }))
  expect(confirm).toHaveBeenCalledExactlyOnceWith('use-local')
  expect(await screen.findByRole('alert')).toHaveTextContent('synthetic repository failure')
  act(() => setLocale('zh-CN'))
  expect(screen.getByRole('alert')).toHaveTextContent('synthetic repository failure')
})

it('focuses the safe dismiss action and restores the opener after cancellation', async () => {
  function Fixture() {
    const [open, setOpen] = useState(false)
    return (
      <>
        <button onClick={() => setOpen(true)}>Open first sync</button>
        {open && <FirstSyncDialog busy={false} paused={false} repositoryUrl="synthetic" confirm={vi.fn()} dismiss={() => setOpen(false)} />}
      </>
    )
  }
  const user = userEvent.setup()
  render(<Fixture />)
  const opener = screen.getByRole('button', { name: 'Open first sync' })
  await user.click(opener)
  expect(screen.getByRole('button', { name: 'Choose later' })).toHaveFocus()
  fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(opener).toHaveFocus()
})

it('prevents cancel and repeated confirmation while saving', async () => {
  let reject!: (cause: Error) => void
  const saved = new Promise<void>((_, rejectPromise) => { reject = rejectPromise })
  const confirm = vi.fn((_strategy: FirstSyncStrategy) => saved)
  const dismiss = vi.fn()
  function Fixture() {
    const [busy, setBusy] = useState(false)
    return <FirstSyncDialog
      busy={busy}
      paused={false}
      repositoryUrl="synthetic"
      dismiss={dismiss}
      confirm={async (strategy) => {
        setBusy(true)
        try {
          await confirm(strategy)
        } finally {
          setBusy(false)
        }
      }}
    />
  }
  const user = userEvent.setup()
  render(<Fixture />)
  await user.click(screen.getByRole('radio', { name: /Use local/ }))
  await user.dblClick(screen.getByRole('button', { name: 'Start first sync' }))
  expect(confirm).toHaveBeenCalledOnce()
  expect(screen.getByRole('button', { name: 'Choose later' })).toBeDisabled()
  fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))
  expect(dismiss).not.toHaveBeenCalled()
  reject(new Error('synthetic save failure'))
  expect(await screen.findByRole('alert')).toHaveTextContent('synthetic save failure')
  expect(screen.getByRole('button', { name: 'Start first sync' })).toBeEnabled()
})
