import type { Progress } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import type { AppSyncNotices } from '../desktopState'
import { useI18n } from '../i18n'

export function ResultSummary({ progress, notices, openLog }: {
  progress: Progress
  notices: AppSyncNotices
  openLog: (filter: 'all') => void
}) {
  const { t } = useI18n()
  const labels: Record<string, string> = { Restored: '已恢复', Reinstalled: '已重新安装', Conflicts: '冲突' }
  const issueCount = notices.issues.length
  const hasIssues = issueCount > 0 || notices.skipped > 0 || notices.blocked > 0
  if (progress.stage !== 'complete' && !progress.needsAttention && !hasIssues) return null
  const values = [
    ['Restored', progress.restored],
    ['Reinstalled', progress.reinstalled],
    ['Conflicts', progress.conflicts],
  ] as const
  return (
    <section className={`result-summary ${progress.needsAttention ? 'needs-attention' : ''}`} aria-label={t('Last cycle results', '上次周期结果')} aria-live="polite">
      {values.map(([label, value]) => <span key={label}><strong>{value}</strong>{t(label, labels[label])}</span>)}
      {hasIssues && (
        <button
          type="button"
          aria-label={notices.detailsAvailable
            ? t(`Issues: ${issueCount}. Open sync diagnostics log`, `问题：${issueCount}。打开同步诊断日志`)
            : t('Open historical sync diagnostics log', '打开历史同步诊断日志')}
          onClick={() => openLog('all')}
        ><strong>{notices.detailsAvailable ? issueCount : '—'}</strong>{t('Issues', '待处理问题')}<small>{t('LOG', '日志')}</small></button>
      )}
      {notices.reviewed && <small className="sync-notices-reviewed">{t('Reviewed safety notices · records retained', '已审阅安全提示 · 记录已保留')}</small>}
    </section>
  )
}
