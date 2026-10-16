import { useEffect, useRef, useState } from 'react'
import { useI18n } from './i18n'
import { LocalizedError, localizedErrorMessage } from './localizedError'

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
  const { t } = useI18n()
  const strategyText: Record<FirstSyncStrategy, [string, string]> = {
    'use-cloud': ['使用云端', '两端存在差异时优先使用云端数据。本地文件可能被替换或删除。'],
    'merge-cloud-local': ['合并云端和本地', '合并两端内容。任何冲突都必须先经审查和解决，才能应用。'],
    'use-local': ['使用本地', '两端存在差异时优先使用此电脑的数据。云端文件可能被替换或删除。'],
  }
  const dialog = useRef<HTMLDialogElement>(null)
  const laterButton = useRef<HTMLButtonElement>(null)
  const [selected, setSelected] = useState<FirstSyncStrategy>()
  const [error, setError] = useState<string | LocalizedError>('')
  const [missingStrategy, setMissingStrategy] = useState(false)

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
      setMissingStrategy(true)
      return
    }
    setError('')
    setMissingStrategy(false)
    try {
      await confirm(selected)
    } catch (cause) {
      setError(cause instanceof LocalizedError ? cause : cause instanceof Error ? cause.message : String(cause))
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
      <span className="eyebrow">{t('FIRST SYNCHRONIZATION', '首次同步')}</span>
      <h2 id="first-sync-title">{t('Choose first sync strategy', '选择首次同步策略')}</h2>
      <p id="first-sync-description">{t('Before syncing, choose how to handle differences between this computer and your cloud repository. Nothing starts until you confirm.', '同步前，请选择如何处理此电脑与云端仓库之间的差异。确认之前不会开始任何操作。')}</p>
      <code className="first-sync-repository">{repositoryUrl}</code>
      <form onSubmit={(event) => void submit(event)}>
        <fieldset disabled={busy} className="first-sync-options">
          <legend>{t('How should the first sync handle your data?', '首次同步应如何处理您的数据？')}</legend>
          {strategies.map((strategy) => (
            <label className={`first-sync-option${selected === strategy.value ? ' selected' : ''}`} key={strategy.value}>
              <input
                type="radio"
                name="first-sync-strategy"
                checked={selected === strategy.value}
                onChange={() => setSelected(strategy.value)}
              />
              <span>
                <strong>{t(strategy.label, strategyText[strategy.value][0])}</strong>
                <small>{t(strategy.description, strategyText[strategy.value][1])}</small>
              </span>
            </label>
          ))}
        </fieldset>
        {paused && <p className="first-sync-paused">{t('Synchronization is paused. Starting your first sync will also resume synchronization.', '同步已暂停。开始首次同步也将恢复同步。')}</p>}
        {missingStrategy && <div className="inline-error" role="alert">{t('Choose a strategy before starting your first sync.', '开始首次同步前请选择策略。')}</div>}
        {error && <div className="inline-error" role="alert">{localizedErrorMessage(error, t)}</div>}
        <div className="first-sync-actions">
          <button ref={laterButton} className="secondary" type="button" disabled={busy} onClick={dismiss}>{t('Choose later', '稍后选择')}</button>
          <button className="primary" type="submit" disabled={busy || !selected}>{busy ? t('Starting first sync...', '正在开始首次同步…') : t('Start first sync', '开始首次同步')}</button>
        </div>
      </form>
    </dialog>
  )
}
