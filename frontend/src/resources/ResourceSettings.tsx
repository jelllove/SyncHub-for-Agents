import type { Agent } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { useI18n } from '../i18n'
import { statusName } from '../statusNames'

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
  const { t } = useI18n()
  return (
    <div className="resource-settings">
      {agents.map((agent) => (
        <details key={agent.name} open={agent.name === 'common'}>
          <summary>
            <span>
              <strong>{agent.name === 'common' ? t('Common resources', '通用资源') : agent.name}</strong>
              <small>{t(`${agent.resources?.length ?? 0} resource groups`, `${agent.resources?.length ?? 0} 个资源组`)}</small>
            </span>
          </summary>
          <div className="resource-list">
            {(agent.resources ?? []).map((resource) => (
              <label
                className={`resource-row ${resource.supported ? '' : 'unsupported'}`}
                key={`${resource.provider}:${resource.id}:${resource.category}`}
              >
                <span>
                  <strong>{statusName(resource.category, t)}</strong>
                  <small>{resource.source || t('No source on this platform', '此平台无来源')} → {resource.target || t('No restore target', '无恢复目标')}</small>
                  <small>
                    {t(`${resource.fileCount} files`, `${resource.fileCount} 个文件`)} · {formatBytes(resource.bytes)}
                    {resource.excludedFiles > 0 && t(` · ${resource.excludedFiles} excluded (${formatBytes(resource.excludedBytes)})`, ` · 已排除 ${resource.excludedFiles} 个（${formatBytes(resource.excludedBytes)}）`)}
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
