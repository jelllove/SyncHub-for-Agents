import { useEffect, useRef, useState } from 'react'
import type { AppSyncNotices } from './desktopState'
import { useI18n } from './i18n'
import { statusName } from './statusNames'
import { LocalizedError, localizedErrorMessage } from './localizedError'

export type SyncNoticeFilter = 'all' | 'skipped' | 'blocked' | 'error'

export function SyncNoticesDialog({ notices, initialFilter, acknowledge, dismiss }: {
  notices: AppSyncNotices
  initialFilter: SyncNoticeFilter
  acknowledge: (fingerprint: string) => Promise<void>
  dismiss: () => void
}) {
  const { locale, t } = useI18n()
  const dialog = useRef<HTMLDialogElement>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const [filter, setFilter] = useState<SyncNoticeFilter>(initialFilter)
  const [confirming, setConfirming] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | LocalizedError>('')
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
      setError(cause instanceof LocalizedError ? cause : cause instanceof Error ? cause.message : String(cause))
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
          <span className="eyebrow">{t('LOG · LAST SUCCESSFUL CYCLE', '日志 · 上次成功的同步周期')}</span>
          <h2 id="sync-notices-title">{t('Sync diagnostics log', '同步诊断日志')}</h2>
        </div>
        <button ref={closeButton} className="secondary" type="button" disabled={submitting} onClick={dismiss}>{t('Close log', '关闭日志')}</button>
      </div>
      <div className="sync-notices-content">
      <p id="sync-notices-description">
        {t('Only issues that may need your attention are shown. Normal policy exclusions are handled automatically and retained in local diagnostic history. Opening or reviewing this log never starts synchronization.', '这里只显示可能需要处理的问题。正常规则排除会自动处理，并保留在本地诊断历史中。打开或审阅此日志不会启动同步。')}
      </p>
      {notices.finishedAt && !notices.finishedAt.startsWith('0001-') && (
        <p>{t('Cycle finished: ', '周期完成时间：')}<time dateTime={notices.finishedAt}>{Number.isNaN(Date.parse(notices.finishedAt)) ? notices.finishedAt : new Date(notices.finishedAt).toLocaleString(locale)}</time></p>
      )}
      {!notices.detailsAvailable ? (
        <p role="status">{t('Historical cycle details are unavailable. These counts were saved without issue records; preview results cannot reconstruct this cycle. Details will be recorded by a future successful cycle.', '历史周期详情不可用。这些计数保存时未附带问题记录；预览结果无法重建此周期。未来成功的同步周期将记录详情。')}</p>
      ) : (
        <>
          <label className="sync-notices-filter">
            {t('Show issues', '显示问题')}
            <select
              value={filter}
              disabled={confirming || submitting}
              onChange={(event) => {
                const value = event.target.value
                if (value === 'all' || value === 'skipped' || value === 'blocked' || value === 'error') setFilter(value)
              }}
            >
              <option value="all">{t('All issues', '所有问题')}</option>
              <option value="skipped">{t('Collection issues', '资源读取问题')}</option>
              <option value="blocked">{t('Blocked', '已拦截')}</option>
              <option value="error">{t('Operational issues', '操作问题')}</option>
            </select>
          </label>
          <ul className="sync-notices-list" aria-label={t('Cycle issue records', '周期问题记录')}>
            {notices.issues.filter((issue) => filter === 'all' || issue.kind === filter).map((issue, index) => (
              <li key={index}>
                <strong>{issue.resourceKey || t('(unattributed resource)', '（未归属资源）')}</strong>
                <code>{issue.path || t('(resource-level issue; no relative file path)', '（资源级问题；无相对文件路径）')}</code>
                <span>{issue.kind === 'skipped' ? t('Collection issue', '资源读取问题') : statusName(issue.kind, t)} · <code>{issue.code}</code></span>
                <p>{issue.message}</p>
                {!issue.reviewable && <small>{t('Operational issue; acknowledgement does not clear it.', '操作问题；确认提示不会清除此问题。')}</small>}
              </li>
            ))}
          </ul>
          {notices.issues.filter((issue) => filter === 'all' || issue.kind === filter).length === 0 && (
            <p>{t('No issue records in this filter.', '此筛选条件下没有问题记录。')}</p>
          )}
        </>
      )}
      </div>
      <footer className="sync-notices-footer">
      {notices.reviewed && <p role="status">{t('Safety notices reviewed', '已审阅安全提示')}</p>}
      {canReview && (
        <>
          <p>{t('Reviewing hides these reminders only. Files stay excluded; sync errors, conflicts and installation approvals remain visible.', '审阅仅隐藏这些提醒。文件仍会被排除；同步错误、冲突和安装批准请求仍然可见。')}</p>
          {confirming && <p>{t('Confirm review of these recorded exclusions?', '确认已审阅这些排除记录？')}</p>}
          <div className="sync-notices-actions">
            {confirming ? (
              <>
                <button className="secondary" type="button" disabled={submitting} onClick={() => setConfirming(false)}>{t('Cancel acknowledgement', '取消确认')}</button>
                <button className="primary" type="button" disabled={submitting} onClick={() => void submit()}>{submitting ? t('Saving acknowledgement...', '正在保存确认…') : t('Confirm acknowledgement', '确认已审阅')}</button>
              </>
            ) : (
              <button className="primary" type="button" onClick={() => { setFilter('all'); setConfirming(true) }}>{t('Review safety notices', '审阅安全提示')}</button>
            )}
          </div>
        </>
      )}
      {error && <div className="inline-error" role="alert">{localizedErrorMessage(error, t)}</div>}
      </footer>
    </dialog>
  )
}
