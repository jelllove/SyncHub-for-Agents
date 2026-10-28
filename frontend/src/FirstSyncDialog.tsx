import { useEffect, useRef, useState } from 'react'

export type FirstSyncStrategy = 'use-cloud' | 'merge-cloud-local' | 'use-local'

const strategies: { value: FirstSyncStrategy; label: string; description: string }[] = [
  {
    value: 'use-cloud',
    label: 'Use cloud',
    description: 'Prefer cloud data when the two sides differ. Local files may be replaced or removed.',
  },
  {
    value: 'merge-cloud-local',
    label: 'Merge cloud + local',
    description: 'Combine both sides. Review and resolve any conflicting changes before they can be applied.',
  },
  {
    value: 'use-local',
    label: 'Use local',
    description: 'Prefer this computer when the two sides differ. Cloud files may be replaced or removed.',
  },
]

export function FirstSyncDialog({ busy, paused, repositoryUrl, confirm, dismiss }: {
  busy: boolean
  paused: boolean
  repositoryUrl: string
  confirm: (strategy: FirstSyncStrategy) => Promise<void>
  dismiss: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const laterButton = useRef<HTMLButtonElement>(null)
  const [selected, setSelected] = useState<FirstSyncStrategy>()
  const [error, setError] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement
    element.showModal()
    laterButton.current?.focus()
    return () => {
      element.close()
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus()
    }
  }, [])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!selected) {
      setError('Choose a strategy before starting your first sync.')
      return
    }
    setError('')
    try {
      await confirm(selected)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  return (
    <dialog
      ref={dialog}
      className="first-sync-dialog"
      aria-labelledby="first-sync-title"
      aria-describedby="first-sync-description"
      onCancel={(event) => {
        event.preventDefault()
        if (!busy) dismiss()
      }}
    >
      <span className="eyebrow">FIRST SYNCHRONIZATION</span>
      <h2 id="first-sync-title">Choose first sync strategy</h2>
      <p id="first-sync-description">Before syncing, choose how to handle differences between this computer and your cloud repository. Nothing starts until you confirm.</p>
      <code className="first-sync-repository">{repositoryUrl}</code>
      <form onSubmit={(event) => void submit(event)}>
        <fieldset disabled={busy} className="first-sync-options">
          <legend>How should the first sync handle your data?</legend>
          {strategies.map((strategy) => (
            <label className={`first-sync-option${selected === strategy.value ? ' selected' : ''}`} key={strategy.value}>
              <input
                type="radio"
                name="first-sync-strategy"
                checked={selected === strategy.value}
                onChange={() => setSelected(strategy.value)}
              />
              <span>
                <strong>{strategy.label}</strong>
                <small>{strategy.description}</small>
              </span>
            </label>
          ))}
        </fieldset>
        {paused && <p className="first-sync-paused">Synchronization is paused. Starting your first sync will also resume synchronization.</p>}
        {error && <div className="inline-error" role="alert">{error}</div>}
        <div className="first-sync-actions">
          <button ref={laterButton} className="secondary" type="button" disabled={busy} onClick={dismiss}>Choose later</button>
          <button className="primary" type="submit" disabled={busy || !selected}>{busy ? 'Starting first sync...' : 'Start first sync'}</button>
        </div>
      </form>
    </dialog>
  )
}
