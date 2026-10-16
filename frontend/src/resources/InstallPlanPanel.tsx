import type { InstallPlan } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { useI18n } from '../i18n'
import { statusName } from '../statusNames'

export function InstallPlanPanel({
  plan,
  busy,
  approve,
  retry,
}: {
  plan: InstallPlan
  busy: boolean
  approve: (id: string) => Promise<unknown>
  retry: (id: string) => Promise<unknown>
}) {
  const { t } = useI18n()
  const operations = plan.operations ?? []
  const hasFailures = operations.some((operation) => Boolean(operation.error))
  return (
    <section className="attention-panel install-panel" aria-live="polite">
      <div className="attention-heading">
        <div>
          <span className="eyebrow">{t('APPROVAL REQUIRED', '需要批准')}</span>
          <h2>{t('Restore agent integrations', '恢复智能体集成')}</h2>
        </div>
        <span className="count-badge">{operations.length}</span>
      </div>
      <p>{t('Review the exact commands SyncHub for Agents will run. No shell command strings are used.', '请审查 SyncHub for Agents 将执行的确切命令。不会使用 shell 命令字符串。')}</p>
      <div className="operation-list scroll-list">
        {operations.map((operation) => (
          <article key={operation.id}>
            <strong>{operation.adapter} · {statusName(operation.kind, t)}</strong>
            <span>{operation.source}</span>
            <code>{t('Executable: ', '可执行程序：')}{operation.executable}</code>
            <code>{t('Arguments: ', '参数：')}{JSON.stringify(operation.args ?? [])}</code>
            {operation.workingDir && <small>{t('Working directory: ', '工作目录：')}{operation.workingDir}</small>}
            {operation.error && <small className="warning">{operation.error}</small>}
          </article>
        ))}
      </div>
      <button
        className="primary"
        disabled={busy || operations.length === 0 || (!hasFailures && plan.approved)}
        onClick={() => void (hasFailures ? retry(plan.id) : approve(plan.id))}
      >
        {hasFailures
          ? t('Retry failed operations', '重试失败的操作')
          : (plan.approved ? t('Approved; waiting for synchronization', '已批准；等待同步') : t('Approve and synchronize', '批准并同步'))}
      </button>
    </section>
  )
}
