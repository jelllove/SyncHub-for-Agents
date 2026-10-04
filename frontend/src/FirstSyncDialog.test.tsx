import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import { FirstSyncDialog, type FirstSyncStrategy } from './FirstSyncDialog'

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
