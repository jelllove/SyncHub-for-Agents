import type { Progress } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'

export function ResultSummary({ progress }: { progress: Progress }) {
  if (progress.stage !== 'complete' && !progress.needsAttention) return null
  const values = [
    ['Restored', progress.restored],
    ['Reinstalled', progress.reinstalled],
    ['Skipped', progress.skipped],
    ['Blocked', progress.blockedFiles],
    ['Conflicts', progress.conflicts],
  ] as const
  return (
    <section className={`result-summary ${progress.needsAttention ? 'needs-attention' : ''}`} aria-live="polite">
      {values.map(([label, value]) => (
        <span key={label}><strong>{value}</strong>{label}</span>
      ))}
    </section>
  )
}
