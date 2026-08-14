import { useState } from 'react'
import type {
  ConflictResolution,
  ConflictSummary,
} from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

export function ConflictPanel({
  conflicts,
  busy,
  resolve,
}: {
  conflicts: ConflictSummary[]
  busy: boolean
  resolve: (input: ConflictResolution) => Promise<unknown>
}) {
  const [editing, setEditing] = useState('')
  const [content, setContent] = useState('')

  return (
    <section className="attention-panel conflict-panel" aria-live="polite">
      <div className="attention-heading">
        <div>
          <span className="eyebrow">MANUAL REVIEW</span>
          <h2>Resolve synchronized conflicts</h2>
        </div>
        <span className="count-badge">{conflicts.length}</span>
      </div>
      <p>Only safe metadata is shown. Choose a complete version or provide reviewed merged content.</p>
      <div className="conflict-list">
        {conflicts.map((conflict) => (
          <article key={conflict.id}>
            <strong>{conflict.resourceKey}</strong>
            <code>{conflict.path}</code>
            <small>Created {new Date(conflict.createdAt).toLocaleString()}</small>
            <div className="inline-actions">
              <button className="secondary" disabled={busy} onClick={() => void resolve({ id: conflict.id, choice: 'local' })}>
                Use local
              </button>
              <button className="secondary" disabled={busy} onClick={() => void resolve({ id: conflict.id, choice: 'remote' })}>
                Use remote
              </button>
              <button
                className="text-button"
                disabled={busy}
                onClick={() => {
                  setEditing(editing === conflict.id ? '' : conflict.id)
                  setContent('')
                }}
              >
                Edit merged
              </button>
            </div>
            {editing === conflict.id && (
              <div className="merge-editor">
                <label>
                  Reviewed merged content
                  <textarea
                    value={content}
                    onChange={(event) => setContent(event.target.value)}
                    placeholder="Paste or write the complete merged file"
                  />
                </label>
                <button
                  className="primary"
                  disabled={busy || content.length === 0}
                  onClick={() => void resolve({ id: conflict.id, choice: 'merged', content })}
                >
                  Save merged version
                </button>
              </div>
            )}
          </article>
        ))}
      </div>
    </section>
  )
}
