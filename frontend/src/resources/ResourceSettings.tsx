import type { Agent } from '../../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

export type CategorySettings = Record<string, Record<string, boolean>>

export function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`
}

function updateCategory(
  current: CategorySettings,
  agent: string,
  category: string,
  enabled: boolean,
) {
  return {
    ...current,
    [agent]: {
      ...(current[agent] ?? {}),
      [category]: enabled,
    },
  }
}

export function ResourceSettings({
  agents,
  categories,
  onChange,
}: {
  agents: Agent[]
  categories: CategorySettings
  onChange: (next: CategorySettings) => void
}) {
  return (
    <div className="resource-settings">
      {agents.map((agent) => (
        <details key={agent.name} open={agent.name === 'common'}>
          <summary>
            <span>
              <strong>{agent.name === 'common' ? 'Common resources' : agent.name}</strong>
              <small>{agent.resources?.length ?? 0} resource groups</small>
            </span>
          </summary>
          <div className="resource-list">
            {(agent.resources ?? []).map((resource) => (
              <label
                className={`resource-row ${resource.supported ? '' : 'unsupported'}`}
                key={`${resource.provider}:${resource.id}:${resource.category}`}
              >
                <span>
                  <strong>{resource.category}</strong>
                  <small>{resource.source || 'No source on this platform'} → {resource.target || 'No restore target'}</small>
                  <small>
                    {resource.fileCount} files · {formatBytes(resource.bytes)}
                    {resource.excludedFiles > 0 && ` · ${resource.excludedFiles} excluded (${formatBytes(resource.excludedBytes)})`}
                  </small>
                  {resource.reason && <small className="warning">{resource.reason}</small>}
                </span>
                <input
                  type="checkbox"
                  disabled={!resource.supported}
                  checked={categories[agent.name]?.[resource.category] ?? resource.enabled}
                  onChange={(event) => onChange(updateCategory(
                    categories,
                    agent.name,
                    resource.category,
                    event.target.checked,
                  ))}
                />
              </label>
            ))}
          </div>
        </details>
      ))}
    </div>
  )
}
