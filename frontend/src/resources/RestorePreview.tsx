import type { ResourcePreview } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { formatBytes } from './ResourceSettings'
import { useI18n } from '../i18n'
import { statusName } from '../statusNames'

export function RestorePreview({ preview }: { preview: ResourcePreview }) {
  const { t } = useI18n()
  const resources = preview.resources ?? []
  const issues = preview.issues ?? []
  return (
    <section className="resource-preview" aria-label={t('Synchronization preview', '同步预览')}>
      <div className="preview-summary">
        <span><strong>{preview.files}</strong> {t('safe files', '个安全文件')}</span>
        <span><strong>{formatBytes(preview.bytes)}</strong> {t('portable data', '可移植数据')}</span>
        <span><strong>{preview.excludedFiles}</strong> {t('excluded or blocked', '项已排除或拦截')}</span>
      </div>
      <details>
        <summary>{t('Review source and restore paths', '审查来源和恢复路径')}</summary>
        <div className="preview-paths">
          {resources.map((resource) => (
            <div key={`${resource.provider}:${resource.id}:${resource.category}`}>
              <strong>{resource.provider} · {statusName(resource.category, t)}</strong>
              <code>{resource.source || t('Unavailable', '不可用')}</code>
              <span aria-hidden="true">↓</span>
              <code>{resource.target || t('No restore target', '无恢复目标')}</code>
            </div>
          ))}
        </div>
      </details>
      {issues.length > 0 && (
        <details className="preview-issues">
          <summary>{t(`${issues.length} safety notice${issues.length === 1 ? '' : 's'}`, `${issues.length} 条安全提示`)}</summary>
          <div className="scroll-list">
            {issues.map((issue, index) => (
              <div key={`${issue.resourceKey}:${issue.path}:${index}`}>
                <strong>{issue.code}</strong>
                <span>{issue.path || issue.resourceKey}</span>
                <small>{issue.message}</small>
              </div>
            ))}
          </div>
        </details>
      )}
    </section>
  )
}
