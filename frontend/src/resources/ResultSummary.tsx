import type { Progress } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import type { AppSyncNotices } from '../desktopState'

export function ResultSummary({ progress, notices, openLog }: {
  progress: Progress
  notices: AppSyncNotices
  openLog: (filter: 'skipped' | 'blocked') => void
}) {
  if (progress.stage !== 'complete' && !progress.needsAttention && !notices.skipped && !notices.blocked) return null
  const values = [
    ['Restored', progress.restored],
    ['Reinstalled', progress.reinstalled],
    ['Skipped', notices.skipped],
    ['Blocked', notices.blocked],
    ['Conflicts', progress.conflicts],
  ] as const
  return (
    <section className={`result-summary ${progress.needsAttention ? 'needs-attention' : ''}`} aria-label="Last cycle results" aria-live="polite">
      {values.map(([label, value]) => (
        label === 'Skipped' || label === 'Blocked' ? (
          <button
            key={label}
            type="button"
            aria-label={`${label}: ${value}. Open sync diagnostics log`}
            onClick={() => openLog(label === 'Skipped' ? 'skipped' : 'blocked')}
          ><strong>{value}</strong>{label}<small>LOG</small></button>
        ) : <span key={label}><strong>{value}</strong>{label}</span>
      ))}
      {notices.reviewed && <small className="sync-notices-reviewed">Reviewed safety notices · records retained</small>}
    </section>
  )
}
