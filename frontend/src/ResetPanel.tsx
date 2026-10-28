import { useState } from 'react'
import {
  ResetLocalSetup,
  ResetPreview,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'

export function ResetPanel({ busy, complete, workingChanged }: {
  busy: boolean
  complete: () => void
  workingChanged?: (working: boolean) => void
}) {
  const [repoPath, setRepoPath] = useState<string>()
  const [confirmation, setConfirmation] = useState('')
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')

  const review = async () => {
    setWorking(true)
    workingChanged?.(true)
    setError('')
    try {
      const preview = await ResetPreview()
      setConfirmation('')
      setRepoPath(preview.repoPath)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setWorking(false)
      workingChanged?.(false)
    }
  }

  const reset = async () => {
    if (repoPath === undefined) {
      setError('Review the local repository path before resetting.')
      return
    }
    setWorking(true)
    workingChanged?.(true)
    setError('')
    try {
      await ResetLocalSetup(confirmation, repoPath)
      complete()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setWorking(false)
      workingChanged?.(false)
    }
  }

  return (
    <section className="reset-panel">
      <h3>Reset local setup</h3>
      <p>Remove SyncHub settings, sync history, pending conflicts and installs, saved OAuth login, and the local clone. This cannot be undone.</p>
      <p>Your remote repository, agent files, and SSH keys/configuration will not be changed.</p>
      {repoPath === undefined ? (
        <button className="secondary" disabled={busy || working} onClick={() => void review()} type="button">
          Reset and start over
        </button>
      ) : (
        <>
          <p>Local clone to delete:</p>
          <code>{repoPath}</code>
          <label>
            Type RESET to confirm
            <input value={confirmation} disabled={busy || working} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
          </label>
          <div className="inline-actions">
            <button className="secondary" disabled={working} onClick={() => { setRepoPath(undefined); setError('') }} type="button">
              Cancel reset
            </button>
            <button className="primary" disabled={busy || working || confirmation !== 'RESET'} onClick={() => void reset()} type="button">
              {working ? 'Resetting…' : 'Delete local setup'}
            </button>
          </div>
        </>
      )}
      {error && <div className="inline-error" role="alert">{error}</div>}
    </section>
  )
}
