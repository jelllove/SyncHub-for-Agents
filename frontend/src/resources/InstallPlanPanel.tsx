import type { InstallPlan } from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

export function InstallPlanPanel({
  plan,
  busy,
  approve,
}: {
  plan: InstallPlan
  busy: boolean
  approve: (id: string) => Promise<unknown>
}) {
  const operations = plan.operations ?? []
  return (
    <section className="attention-panel install-panel" aria-live="polite">
      <div className="attention-heading">
        <div>
          <span className="eyebrow">APPROVAL REQUIRED</span>
          <h2>Restore agent integrations</h2>
        </div>
        <span className="count-badge">{operations.length}</span>
      </div>
      <p>Review the exact commands AgentConfigSync will run. No shell command strings are used.</p>
      <div className="operation-list scroll-list">
        {operations.map((operation) => (
          <article key={operation.id}>
            <strong>{operation.adapter} · {operation.kind}</strong>
            <span>{operation.source}</span>
            <code>Executable: {operation.executable}</code>
            <code>Arguments: {JSON.stringify(operation.args ?? [])}</code>
            {operation.workingDir && <small>Working directory: {operation.workingDir}</small>}
            {operation.error && <small className="warning">{operation.error}</small>}
          </article>
        ))}
      </div>
      <button
        className="primary"
        disabled={busy || plan.approved || operations.length === 0}
        onClick={() => void approve(plan.id)}
      >
        {plan.approved ? 'Approved; waiting for synchronization' : 'Approve and synchronize'}
      </button>
    </section>
  )
}
