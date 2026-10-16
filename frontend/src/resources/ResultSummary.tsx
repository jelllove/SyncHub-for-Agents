import type { Progress } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import type { AppSyncNotices } from '../desktopState'
import { useI18n } from '../i18n'

export function ResultSummary({ progress, notices, openLog }: {
  progress: Progress
  notices: AppSyncNotices
  openLog: (filter: 'skipped' | 'blocked') => void
}) {
  const { t } = useI18n()
  const labels: Record<string, string> = { Restored: '已恢复', Reinstalled: '已重新安装', Skipped: '已跳过', Blocked: '已拦截', Conflicts: '冲突' }
  if (progress.stage !== 'complete' && !progress.needsAttention && !notices.skipped && !notices.blocked) return null
  const values = [
    ['Restored', progress.restored],
    ['Reinstalled', progress.reinstalled],
    ['Skipped', notices.skipped],
    ['Blocked', notices.blocked],
    ['Conflicts', progress.conflicts],
  ] as const
  return (
    <section className={`result-summary ${progress.needsAttention ? 'needs-attention' : ''}`} aria-label={t('Last cycle results', '上次周期结果')} aria-live="polite">
      {values.map(([label, value]) => (
        label === 'Skipped' || label === 'Blocked' ? (
          <button
            key={label}
            type="button"
            aria-label={t(`${label}: ${value}. Open sync diagnostics log`, `${labels[label]}：${value}。打开同步诊断日志`)}
            onClick={() => openLog(label === 'Skipped' ? 'skipped' : 'blocked')}
          ><strong>{value}</strong>{t(label, labels[label])}<small>{t('LOG', '日志')}</small></button>
        ) : <span key={label}><strong>{value}</strong>{t(label, labels[label])}</span>
      ))}
      {notices.reviewed && <small className="sync-notices-reviewed">{t('Reviewed safety notices · records retained', '已审阅安全提示 · 记录已保留')}</small>}
    </section>
  )
}
