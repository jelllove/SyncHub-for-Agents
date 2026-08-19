import type { ResourcePreview } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { formatBytes } from './ResourceSettings'

export function RestorePreview({ preview }: { preview: ResourcePreview }) {
  const resources = preview.resources ?? []
  const issues = preview.issues ?? []
  return (
    <section className="resource-preview" aria-label="Synchronization preview">
      <div className="preview-summary">
        <span><strong>{preview.files}</strong> safe files</span>
        <span><strong>{formatBytes(preview.bytes)}</strong> portable data</span>
        <span><strong>{preview.excludedFiles}</strong> excluded or blocked</span>
      </div>
      <details>
        <summary>Review source and restore paths</summary>
        <div className="preview-paths">
          {resources.map((resource) => (
            <div key={`${resource.provider}:${resource.id}:${resource.category}`}>
              <strong>{resource.provider} · {resource.category}</strong>
              <code>{resource.source || 'Unavailable'}</code>
              <span aria-hidden="true">↓</span>
              <code>{resource.target || 'No restore target'}</code>
            </div>
          ))}
        </div>
      </details>
      {issues.length > 0 && (
        <details className="preview-issues">
          <summary>{issues.length} safety notice{issues.length === 1 ? '' : 's'}</summary>
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
