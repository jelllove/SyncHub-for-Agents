import { useMemo, useState } from 'react'
import { PreviewCustomResource } from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type {
  CustomResourceInput,
  ResourcePreview,
} from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import { formatBytes } from './ResourceSettings'
import { useI18n } from '../i18n'
import { statusName } from '../statusNames'

type Category = 'sessions' | 'config' | 'instructions' | 'skills' | 'plugins'
type Strategy = 'file-tree' | 'text-tree' | 'structured-merge' | 'source-tree'

export interface CustomResourceDraft {
  id: string
  category: Category
  source: string
  target: string
  include: string
  exclude: string
  strategy: Strategy
}

const initialDraft: CustomResourceDraft = {
  id: '',
  category: 'instructions',
  source: '',
  target: '',
  include: '**',
  exclude: '',
  strategy: 'text-tree',
}

export function toCustomResource(draft: CustomResourceDraft, goos: string): CustomResourceInput {
  return {
    id: draft.id.trim(),
    category: draft.category,
    paths: { [goos]: draft.source.trim() },
    targets: { [goos]: draft.target.trim() },
    include: draft.include.split('\n').map((value) => value.trim()).filter(Boolean),
    exclude: draft.exclude.split('\n').map((value) => value.trim()).filter(Boolean),
    strategy: draft.strategy,
  }
}

function strategies(category: Category): Strategy[] {
  if (category === 'sessions') return ['file-tree']
  if (category === 'plugins') return ['source-tree']
  return ['file-tree', 'text-tree', 'structured-merge', 'source-tree']
}

export function CustomResourceEditor({
  platform,
  resources,
  onChange,
}: {
  platform: string
  resources: CustomResourceInput[]
  onChange: (resources: CustomResourceInput[]) => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(initialDraft)
  const [preview, setPreview] = useState<ResourcePreview>()
  const [previewedCandidate, setPreviewedCandidate] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const candidate = useMemo(() => toCustomResource(draft, platform), [draft, platform])
  const duplicate = resources.some((resource) => resource.id === candidate.id)
  const valid = /^[A-Za-z0-9._-]+$/.test(candidate.id)
    && candidate.paths?.[platform]
    && candidate.targets?.[platform]
    && (candidate.include?.length ?? 0) > 0

  const update = (next: Partial<CustomResourceDraft>) => {
    setDraft((current) => ({ ...current, ...next }))
    setPreview(undefined)
    setPreviewedCandidate('')
    setError('')
  }

  const previewCandidate = async () => {
    const fingerprint = JSON.stringify(candidate)
    setBusy(true)
    setError('')
    try {
      setPreview(await PreviewCustomResource(candidate))
      setPreviewedCandidate(fingerprint)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setBusy(false)
    }
  }

  const add = () => {
    onChange([...resources, candidate])
    setDraft(initialDraft)
    setPreview(undefined)
    setPreviewedCandidate('')
  }

  return (
    <fieldset className="custom-resources">
      <legend>{t('Custom resources', '自定义资源')}</legend>
      {resources.length > 0 && (
        <div className="custom-resource-list">
          {resources.map((resource) => (
            <div key={resource.id}>
              <span>
                <strong>{resource.id}</strong>
                <small>{statusName(resource.category, t)} · {statusName(resource.strategy, t)}</small>
              </span>
              <button
                className="text-button danger"
                type="button"
                onClick={() => onChange(resources.filter((item) => item.id !== resource.id))}
              >
                {t('Remove', '移除')}
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="custom-editor">
        <div className="field-grid">
          <label>
            {t('Stable ID', '稳定 ID')}
            <input value={draft.id} onChange={(event) => update({ id: event.target.value })} placeholder="my-instructions" />
          </label>
          <label>
            {t('Category', '类别')}
            <select
              value={draft.category}
              onChange={(event) => {
                const category = event.target.value as Category
                update({ category, strategy: strategies(category)[0] })
              }}
            >
              {(['sessions', 'config', 'instructions', 'skills', 'plugins'] as Category[]).map((category) => (
                <option key={category} value={category}>{statusName(category, t)}</option>
              ))}
            </select>
          </label>
        </div>
        <label>
          {t('Source on this computer', '此电脑上的来源')}
          <input
            value={draft.source}
            onChange={(event) => update({ source: event.target.value })}
            placeholder={'C:\\Users\\you\\.agent\\instructions'}
          />
        </label>
        <label>
          {t('Restore target', '恢复目标')}
          <input
            value={draft.target}
            onChange={(event) => update({ target: event.target.value })}
            placeholder={'C:\\Users\\you\\.agent\\instructions'}
          />
        </label>
        <div className="field-grid">
          <label>
            {t('Include globs (one per line)', '包含的通配模式（每行一项）')}
            <textarea value={draft.include} onChange={(event) => update({ include: event.target.value })} />
          </label>
          <label>
            {t('Exclude globs (one per line)', '排除的通配模式（每行一项）')}
            <textarea value={draft.exclude} onChange={(event) => update({ exclude: event.target.value })} />
          </label>
        </div>
        <label>
          {t('Synchronization strategy', '同步策略')}
          <select value={draft.strategy} onChange={(event) => update({ strategy: event.target.value as Strategy })}>
            {strategies(draft.category).map((strategy) => <option key={strategy} value={strategy}>{statusName(strategy, t)}</option>)}
          </select>
        </label>
        {error && <div className="inline-error" role="alert">{error}</div>}
        {preview && (
          <div className="candidate-preview" aria-live="polite">
            <strong>{t(`${preview.files} safe files`, `${preview.files} 个安全文件`)} · {formatBytes(preview.bytes)}</strong>
            <span>
              {t(`${preview.excludedFiles} excluded`, `${preview.excludedFiles} 项已排除`)} · {t(`${(preview.issues ?? []).filter((issue) => issue.code.includes('large')).length} oversized`, `${(preview.issues ?? []).filter((issue) => issue.code.includes('large')).length} 项过大`)}
              {' · '}{t(`${(preview.issues ?? []).filter((issue) => issue.code.includes('secret') || issue.code.includes('scan')).length} blocked`, `${(preview.issues ?? []).filter((issue) => issue.code.includes('secret') || issue.code.includes('scan')).length} 项已拦截`)}
            </span>
          </div>
        )}
        <div className="inline-actions">
          <button type="button" className="secondary" disabled={!valid || duplicate || busy} onClick={() => void previewCandidate()}>
            {busy ? t('Checking…', '正在检查…') : t('Preview', '预览')}
          </button>
          <button
            type="button"
            className="primary"
            disabled={!preview || previewedCandidate !== JSON.stringify(candidate) || duplicate}
            onClick={add}
          >
            {t('Add resource', '添加资源')}
          </button>
        </div>
        {duplicate && <small className="warning">{t('A custom resource with this ID already exists.', '已存在使用此 ID 的自定义资源。')}</small>}
      </div>
    </fieldset>
  )
}
