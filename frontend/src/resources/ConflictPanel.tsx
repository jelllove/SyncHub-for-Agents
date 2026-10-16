import { useMemo, useState } from 'react'
import { useI18n } from '../i18n'
import { statusName } from '../statusNames'
import type {
  ConflictResolutionStatus,
  ConflictSelection,
  ConflictSummary,
} from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'

type SelectionChoice = 'local' | 'remote' | 'merged'
type BulkSelectionChoice = Exclude<SelectionChoice, 'merged'>

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
  const { locale, t } = useI18n()
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

  const setAllChoice = (choice: BulkSelectionChoice) => {
    setSelections(
      Object.fromEntries(conflicts.map((conflict) => [conflict.id, { choice, content: '' }])),
    )
  }

  const clearAllChoices = () => {
    setSelections({})
  }

  return (
    <section className="attention-panel conflict-panel" aria-live="polite">
      <div className="attention-heading">
        <div>
          <span className="eyebrow">{t('MANUAL REVIEW', '人工审查')}</span>
          <h2>{t('Resolve synchronized conflicts', '解决同步冲突')}</h2>
        </div>
        <span className="count-badge">{conflicts.length}</span>
      </div>
      <p>{t('Choose a decision for every conflict, then apply them together in one transaction.', '为每个冲突选择处理方式，然后在同一事务中统一应用。')}</p>
      <div className="inline-actions">
        <button
          className="secondary"
          disabled={busy || conflicts.length === 0}
          onClick={() => setAllChoice('local')}
          type="button"
        >
          {t('Use local for all', '全部使用本地')}
        </button>
        <button
          className="secondary"
          disabled={busy || conflicts.length === 0}
          onClick={() => setAllChoice('remote')}
          type="button"
        >
          {t('Use remote for all', '全部使用远程')}
        </button>
        <button
          className="text-button"
          disabled={busy || conflicts.length === 0}
          onClick={clearAllChoices}
          type="button"
        >
          {t('Clear all', '清除全部')}
        </button>
      </div>
      {resolution && (
        <div className={`resolution-status resolution-${resolution.status}`}>
          <strong>{t('Batch status: ', '批次状态：')}{statusName(resolution.status, t)}</strong>
          <span>{t(`${resolution.selected} selections`, `${resolution.selected} 项选择`)}</span>
          {resolution.error && <small className="warning">{resolution.error}</small>}
          {resolution.status === 'failed' && (
            <button
              className="secondary"
              disabled={busy}
              onClick={() => void retryBatch(resolution.id)}
            >
              {t('Retry failed batch', '重试失败批次')}
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
              <small>{t('Created ', '创建于 ')}{new Date(conflict.createdAt).toLocaleString(locale)}</small>
              <div className="inline-actions">
                <button
                  className={selectionClass(selected?.choice === 'local')}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'local')}
                  type="button"
                >
                  {t('Use local', '使用本地')}
                </button>
                <button
                  className={selectionClass(selected?.choice === 'remote')}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'remote')}
                  type="button"
                >
                  {t('Use remote', '使用远程')}
                </button>
                <button
                  className={selected?.choice === 'merged' ? 'text-button active' : 'text-button'}
                  disabled={busy}
                  onClick={() => setChoice(conflict.id, 'merged')}
                  type="button"
                >
                  {t('Edit merged', '编辑合并内容')}
                </button>
              </div>
              {merged && (
                <div className="merge-editor">
                  <label>
                    {t('Reviewed merged content', '已审查的合并内容')}
                    <textarea
                      value={selected.content}
                      onChange={(event) => setMergedContent(conflict.id, event.target.value)}
                      placeholder={t('Paste or write the complete merged file', '粘贴或编写完整的合并文件')}
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
        type="button"
      >
        {t('Apply all and synchronize', '全部应用并同步')}
      </button>
    </section>
  )
}
