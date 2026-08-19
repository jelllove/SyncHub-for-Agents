import { useMemo, useState } from 'react'
import type {
  ConflictResolutionStatus,
  ConflictSelection,
  ConflictSummary,
} from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

type SelectionChoice = 'local' | 'remote' | 'merged'

type ConflictSelections = Partial<Record<string, {
  choice: SelectionChoice
  content: string
}>>

function selectionClass(active: boolean) {
  return active ? 'secondary active' : 'secondary'
}

export function ConflictPanel({
  conflicts,
  resolution,
  busy,
  applyBatch,
  retryBatch,
}: {
  conflicts: ConflictSummary[]
  resolution: ConflictResolutionStatus | null
  busy: boolean
  applyBatch: (selections: ConflictSelection[]) => Promise<unknown>
  retryBatch: (id: string) => Promise<unknown>
}) {
  const [selections, setSelections] = useState<ConflictSelections>({})

  const applySelections = useMemo(
    () => conflicts.map((conflict) => {
      const selected = selections[conflict.id]
      return {
        id: conflict.id,
        revision: conflict.revision,
        choice: selected?.choice ?? '',
        content: selected?.choice === 'merged' ? selected.content : '',
      }
    }),
    [conflicts, selections],
  )
  const complete = conflicts.length > 0 && applySelections.every((selection) => {
    if (selection.choice === '') return false
    if (selection.choice === 'merged') return selection.content.trim().length > 0
    return true
  })

  const setChoice = (id: string, choice: SelectionChoice) => {
    setSelections((current) => ({
      ...current,
      [id]: {
        choice,
        content: choice === 'merged' ? (current[id]?.content ?? '') : '',
      },
    }))
  }

  const setMergedContent = (id: string, content: string) => {
    setSelections((current) => ({
      ...current,
      [id]: {
        choice: 'merged',
        content,
      },
    }))
  }

  return (
    <section className="attention-panel conflict-panel" aria-live="polite">
      <div className="attention-heading">
        <div>
          <span className="eyebrow">MANUAL REVIEW</span>
          <h2>Resolve synchronized conflicts</h2>
        </div>
        <span className="count-badge">{conflicts.length}</span>
      </div>
      <p>Choose a decision for every conflict, then apply them together in one transaction.</p>
      {resolution && (
        <div className={`resolution-status resolution-${resolution.status}`}>
          <strong>Batch status: {resolution.status}</strong>
          <span>{resolution.selected} selections</span>
          {resolution.error && <small className="warning">{resolution.error}</small>}
          {resolution.status === 'failed' && (
            <button
              className="secondary"
              disabled={busy}
              onClick={() => void retryBatch(resolution.id)}
            >
              Retry failed batch
            </button>
          )}
        </div>
      )}
      <div className="conflict-list">
        {conflicts.map((conflict) => {
          const selected = selections[conflict.id]
          const merged = selected?.choice === 'merged'
          return (
            <article key={conflict.id}>
              <strong>{conflict.resourceKey}</strong>
              <code>{conflict.path}</code>
              <small>Created {new Date(conflict.createdAt).toLocaleString()}</small>
              <div className="inline-actions">
                <button
                  className={selectionClass(selected?.choice === 'local')}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'local')}
                >
                  Use local
                </button>
                <button
                  className={selectionClass(selected?.choice === 'remote')}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'remote')}
                >
                  Use remote
                </button>
                <button
                  className={selected?.choice === 'merged' ? 'text-button active' : 'text-button'}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'merged')}
                >
                  Edit merged
                </button>
              </div>
              {merged && (
                <div className="merge-editor">
                  <label>
                    Reviewed merged content
                    <textarea
                      value={selected.content}
                      onChange={(event) => setMergedContent(conflict.id, event.target.value)}
                      placeholder="Paste or write the complete merged file"
                    />
                  </label>
                </div>
              )}
            </article>
          )
        })}
      </div>
      <button
        className="primary"
        disabled={busy || !complete}
        onClick={() => void applyBatch(applySelections)}
      >
        Apply all and synchronize
      </button>
    </section>
  )
}
