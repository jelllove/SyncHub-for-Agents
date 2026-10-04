import { useEffect, useRef, useState } from 'react'
import type { AppSyncNotices } from './desktopState'

export type SyncNoticeFilter = 'all' | 'skipped' | 'blocked' | 'error'

export function SyncNoticesDialog({ notices, initialFilter, acknowledge, dismiss }: {
  notices: AppSyncNotices
  initialFilter: SyncNoticeFilter
  acknowledge: (fingerprint: string) => Promise<void>
  dismiss: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const [filter, setFilter] = useState<SyncNoticeFilter>(initialFilter)
  const [confirming, setConfirming] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const canReview = notices.detailsAvailable && !notices.reviewed
    && notices.issues.some((issue) => issue.reviewable)

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement
    element.showModal()
    closeButton.current?.focus()
    return () => {
      element.close()
      if (previousFocus instanceof HTMLElement && previousFocus.isConnected) previousFocus.focus()
    }
  }, [])

  const submit = async () => {
    setSubmitting(true)
    setError('')
    try {
      await acknowledge(notices.fingerprint)
      setConfirming(false)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <dialog
      ref={dialog}
      className="sync-notices-dialog"
      aria-labelledby="sync-notices-title"
      aria-describedby="sync-notices-description"
      onCancel={(event) => {
        event.preventDefault()
        if (!submitting) dismiss()
      }}
    >
      <div className="attention-heading">
        <div>
          <span className="eyebrow">LOG · LAST SUCCESSFUL CYCLE</span>
          <h2 id="sync-notices-title">Sync diagnostics log</h2>
        </div>
        <button ref={closeButton} className="secondary" type="button" disabled={submitting} onClick={dismiss}>Close log</button>
      </div>
      <div className="sync-notices-content">
      <p id="sync-notices-description">
        Skipped: {notices.skipped}. Blocked: {notices.blocked}.
        {' '}Exact original counts and per-resource records are retained, including duplicates.
        Blocked counts can differ from the number of records because the cycle counts protected paths.
        Opening or reviewing this log never starts synchronization.
      </p>
      {notices.finishedAt && !notices.finishedAt.startsWith('0001-') && (
        <p>Cycle finished: <time dateTime={notices.finishedAt}>{notices.finishedAt}</time></p>
      )}
      {!notices.detailsAvailable ? (
        <p role="status">Historical cycle details are unavailable. These counts were saved without issue records; preview results cannot reconstruct this cycle. Details will be recorded by a future successful cycle.</p>
      ) : (
        <>
          <label className="sync-notices-filter">
            Show issues
            <select
              value={filter}
              disabled={confirming || submitting}
              onChange={(event) => {
                const value = event.target.value
                if (value === 'all' || value === 'skipped' || value === 'blocked' || value === 'error') setFilter(value)
              }}
            >
              <option value="all">All issues</option>
              <option value="skipped">Skipped</option>
              <option value="blocked">Blocked</option>
              <option value="error">Operational issues</option>
            </select>
          </label>
          <ul className="sync-notices-list" aria-label="Cycle issue records">
            {notices.issues.filter((issue) => filter === 'all' || issue.kind === filter).map((issue, index) => (
              <li key={index}>
                <strong>{issue.resourceKey || '(unattributed resource)'}</strong>
                <code>{issue.path || '(resource-level issue; no relative file path)'}</code>
                <span>{issue.kind} · <code>{issue.code}</code></span>
                <p>{issue.message}</p>
                {!issue.reviewable && <small>Operational issue; acknowledgement does not clear it.</small>}
              </li>
            ))}
          </ul>
          {notices.issues.filter((issue) => filter === 'all' || issue.kind === filter).length === 0 && (
            <p>No issue records in this filter.</p>
          )}
        </>
      )}
      </div>
      <footer className="sync-notices-footer">
      {notices.reviewed && <p role="status">Safety notices reviewed</p>}
      {canReview && (
        <>
          <p>Reviewing hides these reminders only. Files stay excluded; sync errors, conflicts and installation approvals remain visible.</p>
          {confirming && <p>Confirm review of these recorded exclusions?</p>}
          <div className="sync-notices-actions">
            {confirming ? (
              <>
                <button className="secondary" type="button" disabled={submitting} onClick={() => setConfirming(false)}>Cancel acknowledgement</button>
                <button className="primary" type="button" disabled={submitting} onClick={() => void submit()}>{submitting ? 'Saving acknowledgement...' : 'Confirm acknowledgement'}</button>
              </>
            ) : (
              <button className="primary" type="button" onClick={() => { setFilter('all'); setConfirming(true) }}>Review safety notices</button>
            )}
          </div>
        </>
      )}
      {error && <div className="inline-error" role="alert">{error}</div>}
      </footer>
    </dialog>
  )
}
