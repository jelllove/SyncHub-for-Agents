import { useEffect, useId, useRef, useState } from 'react'
import {
  ApplyLibrary,
  LibrarySnapshot,
  PreviewLibrary,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type {
  Entry,
  Plan,
  Request,
  Snapshot,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary/models'
import { SettingsDiagram } from './SettingsDiagram'
import { useI18n, type Translator } from './i18n'

type LibraryAction = 'import' | 'apply' | 'restore' | 'detach'
type LibraryRequest = Request & { action: LibraryAction }

function toolName(harness: string, t: Translator) {
  if (harness === 'library') return t('Shared library', '共享库')
  if (harness === 'claude') return 'Claude Code'
  if (harness === 'copilot') return 'GitHub Copilot'
  if (harness === 'common') return t('Common skills folder', '公共技能目录')
  return harness
}

function resourceLabel(entry: Entry, t: Translator) {
  return `${entry.name} · ${entry.kind === 'skills' ? t('Skill', '技能') : t('Global instructions', '全局指令')} · ${toolName(entry.harness, t)}`
}

function statusLabel(status: string, t: Translator) {
  switch (status) {
    case 'available': return t('Ready to use', '可使用')
    case 'managed': return t('Managed copy', '已管理的副本')
    case 'modified': return t('Changed locally — review before sharing', '本地已修改，请先预览')
    case 'recovery-required': return t('Recovery required', '需要恢复')
    default: return status
  }
}

function entryKey(entry: Entry) {
  return `${entry.kind}/${entry.name}/${entry.harness}`
}

function supported(entry: Entry) {
  return entry.kind === 'skills'
    || (entry.kind === 'instructions' && entry.name === 'global'
      && (entry.harness === 'library' || entry.harness === 'claude'))
}

function Inventory({ title, entries }: { title: string; entries: Entry[] }) {
  const id = useId()
  const { t } = useI18n()
  return (
    <section aria-labelledby={id}>
      <h3 id={id}>{title}</h3>
      {entries.length === 0 ? <p>{t('No resources found.', '未找到资源。')}</p> : (
        <div className="operation-list scroll-list">
          {entries.map((entry) => (
            <article key={entryKey(entry)}>
              <strong>{resourceLabel(entry, t)}</strong>
              <span>{entry.path}</span>
              <small className={entry.status === 'available' || entry.status === 'managed' ? undefined : 'warning'}>
                {statusLabel(entry.status, t)}
              </small>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}

function PreviewText({ label, text }: { label: string; text: string }) {
  const { t } = useI18n()
  const limit = 4000
  const [expanded, setExpanded] = useState(false)
  const remainingId = useId()
  return (
    <div>
      <strong>{label}</strong>
      <code>{text ? text.slice(0, limit) : t('(empty)', '（空）')}</code>
      {text.length > limit && (
        <>
          <button
            type="button"
            className="secondary"
            aria-expanded={expanded}
            aria-controls={remainingId}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded
              ? t(`Hide remaining ${label.toLowerCase()} content (${text.length - limit} characters)`, `收起剩余${label}内容（${text.length - limit} 个字符）`)
              : t(`Show remaining ${label.toLowerCase()} content (${text.length - limit} characters)`, `显示剩余${label}内容（${text.length - limit} 个字符）`)}
          </button>
          <div id={remainingId}>{expanded && <code>{text.slice(limit)}</code>}</div>
        </>
      )}
    </div>
  )
}

export function SharedLibraryPanel({ busy = false }: { busy?: boolean }) {
  const { t } = useI18n()
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [intent, setIntent] = useState<LibraryAction | null>(null)
  const [resource, setResource] = useState('')
  const [target, setTarget] = useState('claude')
  const [approval, setApproval] = useState<{ plan: Plan; request: LibraryRequest } | null>(null)
  const [working, setWorking] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState<LibraryAction | null>(null)
  const generation = useRef(0)
  const mounted = useRef(false)
  const previewId = useId()
  const resourceId = useId()
  const previewHeading = useRef<HTMLHeadingElement>(null)

  const invalidate = () => {
    generation.current += 1
    setApproval(null)
    setWorking('')
    setError('')
    setNotice(null)
    return generation.current
  }
  const current = (revision: number) => mounted.current && generation.current === revision
  const fail = (cause: unknown) => setError(cause instanceof Error ? cause.message : String(cause))

  const refresh = async (completed?: LibraryRequest) => {
    const revision = invalidate()
    setSnapshot(null)
    setWorking('scan')
    if (completed) setNotice(completed.action)
    try {
      const result = await LibrarySnapshot()
      if (!current(revision)) return
      setSnapshot(result)
      if (completed?.action === 'import') {
        setIntent('apply')
        setResource(`${completed.kind}/${completed.name}/library`)
      } else if (completed?.action === 'restore' || completed?.action === 'detach') {
        setIntent(null)
        setResource('')
      }
    } catch (cause) {
      if (current(revision)) fail(cause)
    } finally {
      if (current(revision)) setWorking('')
    }
  }

  useEffect(() => {
    mounted.current = true
    void refresh()
    return () => {
      mounted.current = false
      generation.current += 1
    }
  }, [])

  useEffect(() => {
    if (approval) previewHeading.current?.focus()
  }, [approval])

  const entries = (snapshot?.entries ?? []).filter(supported)
  const canonical = entries.filter((entry) => entry.harness === 'library' && entry.status === 'available')
  const blocked = (snapshot?.entries ?? []).filter((entry) => entry.status.startsWith('blocked:'))
  const candidates = entries.filter((entry) => ['claude', 'common', 'copilot'].includes(entry.harness)
    && ['available', 'managed', 'modified'].includes(entry.status))
  const managed = entries.filter((entry) => ['claude', 'common'].includes(entry.harness)
    && ['managed', 'modified', 'recovery-required'].includes(entry.status))
  const action = intent ?? (canonical.length > 0 ? 'apply' : 'import')
  const resources = action === 'import' ? candidates : action === 'apply' ? canonical : managed
  const selected = resources.find((entry) => entryKey(entry) === resource) ?? resources[0]
  const targets = selected
    ? action !== 'apply' ? [selected.harness] : selected.kind === 'skills' ? ['claude', 'common'] : ['claude']
    : []
  const harness = targets.includes(target) ? target : targets[0] ?? ''
  const request: LibraryRequest | null = selected ? { action, kind: selected.kind, name: selected.name, harness } : null
  const locked = busy || working === 'apply' || working === 'scan'
  const recovery = action === 'restore' || action === 'detach'
  const selectAction = (next: LibraryAction) => {
    invalidate()
    setIntent(next)
    setResource('')
  }
  const actionLabels = {
    import: { preview: t('Preview import', '预览导入'), confirm: t('Confirm import', '确认导入'), done: t('Resource added. Choose where to use it next.', '资源已加入共享库。接下来选择使用位置。') },
    apply: { preview: t('Preview apply', '预览使用'), confirm: t('Confirm apply', '确认使用'), done: t('Shared resource applied to the selected location.', '共享资源已应用到所选位置。') },
    restore: { preview: t('Preview restore', '预览恢复'), confirm: t('Confirm restore', '确认恢复'), done: t('Original files restored.', '已恢复原文件。') },
    detach: { preview: t('Preview detach', '预览停止管理'), confirm: t('Confirm detach', '确认停止管理'), done: t('Management stopped. Target files were kept.', '已停止管理，目标文件保持不变。') },
  }

  const preview = async () => {
    if (!request || locked || working) return
    const revision = invalidate()
    setWorking('preview')
    try {
      const plan = await PreviewLibrary(request)
      if (!current(revision)) return
      setApproval({ plan, request })
    } catch (cause) {
      if (current(revision)) fail(cause)
    } finally {
      if (current(revision)) setWorking('')
    }
  }

  const confirm = async () => {
    if (!approval || busy || working) return
    const approved = approval
    const revision = invalidate()
    setWorking('apply')
    try {
      await ApplyLibrary(approved.request, approved.plan.id)
      if (current(revision)) await refresh(approved.request)
    } catch (cause) {
      if (current(revision)) {
        fail(cause)
        setWorking('')
      }
    }
  }

  const management = managed.length > 0 && (
    <details className="library-management" open={recovery || undefined}>
      <summary>{t('Manage existing copies', '管理已共享的副本')}</summary>
      <p>{t('To keep edits from a managed output, add that copy to the shared library before stopping management. Safety checks still apply.', '如需保留已管理副本中的修改，先将该副本加入共享库，再停止管理。导入仍需通过安全检查。')}</p>
      <Inventory title={t('Managed outputs', '已管理的副本')} entries={managed} />
      <div className="inline-actions">
        <button className="secondary" type="button" disabled={locked} aria-pressed={action === 'restore'} onClick={() => selectAction('restore')}>{t('Restore original files', '恢复原文件')}</button>
        <button className="secondary" type="button" disabled={locked} aria-pressed={action === 'detach'} onClick={() => selectAction('detach')}>{t('Stop managing a copy', '停止管理副本')}</button>
      </div>
    </details>
  )

  return (
    <div className="settings-panel shared-library-panel">
      <section className="preview-refresh">
        <div className="settings-category-heading">
          <SettingsDiagram kind="library" compact />
          <div>
            <h2>{t('Shared library', '共享资源')}</h2>
            <small>{t('Maintain one source for skills or instructions, then choose where to use it.', '维护一份技能或指令，再选择哪些兼容工具使用。')}</small>
          </div>
        </div>
        <button className="secondary" type="button" disabled={locked} onClick={() => void refresh()}>
          {t('Refresh library', '重新扫描')}
        </button>
      </section>
      {working && <p role="status">{working === 'scan' ? t('Scanning library…', '正在扫描资源…') : working === 'preview' ? t('Preparing preview…', '正在准备预览…') : t('Applying approved changes…', '正在应用已确认的变更…')}</p>}
      {error && <div className="inline-error" role="alert">{error}<br />{t('Refresh the library or retry the preview; no force overwrite is available.', '请重新扫描或重试预览，不会强制覆盖文件。')}</div>}
      {notice && <p className="library-notice" role="status">{actionLabels[notice].done}</p>}
      {snapshot && (
        <>
          <div className="library-tasks">
            <button type="button" aria-pressed={action === 'import'} disabled={locked} onClick={() => selectAction('import')}>
              <strong>{t('Add existing resources', '加入已有资源')}</strong>
              <span aria-label={t(`${candidates.length} local resources to review`, `${candidates.length} 项本地资源可供查看`)}>{candidates.length}</span>
            </button>
            <button type="button" aria-pressed={action === 'apply'} disabled={locked} onClick={() => selectAction('apply')}>
              <strong>{t('Use shared resources', '使用共享资源')}</strong>
              <span aria-label={t(`${canonical.length} shared resources`, `共享库中有 ${canonical.length} 项资源`)}>{canonical.length}</span>
            </button>
          </div>
          {recovery && management}
          {!recovery && (
            <ol className="library-steps" aria-label={t('Sharing steps', '共享步骤')}>
              {[t('Add to the library', '加入共享库'), t('Choose where to use it', '选择使用位置'), t('Review and confirm', '预览并确认')].map((label, index) => (
                <li key={index} aria-current={(approval ? 2 : action === 'import' ? 0 : 1) === index ? 'step' : undefined}>
                  <span aria-hidden="true">{index + 1}</span>{label}
                </li>
              ))}
            </ol>
          )}
          {resources.length === 0 ? (
            <section className="library-empty">
              <h3>{action === 'import' ? t('No supported local resources found.', '尚未找到受支持的本地资源。') : t('Add a resource to your shared library first.', '先向共享库添加资源。')}</h3>
              <p>{action === 'import'
                ? t('Skills need a SKILL.md file in your Claude, Copilot, or common skills folder. Claude global instructions are supported too. Check blocked resources below, then refresh the library.', '技能需在 Claude、Copilot 或公共技能目录中包含 SKILL.md 文件；也支持 Claude 全局指令。请检查下方被阻止的资源，再重新扫描。')
                : t('Choose Add existing resources to review a local skill or Claude global instruction. Importing keeps the original files.', '选择“加入已有资源”查看本地技能或 Claude 全局指令。导入不会删除原文件。')}</p>
              {action !== 'import' && <button className="secondary" type="button" disabled={locked} onClick={() => selectAction('import')}>{t('Add existing resources', '加入已有资源')}</button>}
            </section>
          ) : (
            <fieldset disabled={locked} className="library-choice">
              <legend>{recovery ? t('Choose a managed copy', '选择已管理的副本') : action === 'import' ? t('Choose an existing resource', '选择本机已有资源') : t('Choose a shared resource', '选择共享资源')}</legend>
              <div className="library-resource-list">
                {resources.map((entry) => (
                  <label className="library-resource" key={entryKey(entry)}>
                    <input type="radio" name={resourceId} value={entryKey(entry)} checked={selected === entry}
                      aria-label={resourceLabel(entry, t)} aria-description={entry.path}
                      onChange={() => { invalidate(); setResource(entryKey(entry)) }} />
                    <span>
                      <strong>{entry.name}</strong>
                      <small>{entry.kind === 'skills' ? t('Skill', '技能') : t('Global instructions', '全局指令')} · {toolName(entry.harness, t)}</small>
                      {entry.status !== 'available' && <small className="warning">{statusLabel(entry.status, t)}</small>}
                    </span>
                  </label>
                ))}
              </div>
              <div className="library-next-action">
                {action === 'apply' && (
                  <label>
                    {t('Use in', '使用位置')}
                    <select value={harness} onChange={(event) => { invalidate(); setTarget(event.target.value) }}>
                      {targets.map((value) => <option key={value} value={value}>{toolName(value, t)}</option>)}
                    </select>
                  </label>
                )}
                <button className="primary" type="button" disabled={!request || !!working} onClick={() => void preview()}>
                  {actionLabels[action].preview}
                </button>
              </div>
              <p className="library-hint">
                {action === 'import'
                  ? t('Import keeps the original files. A same-name resource in the shared library will be compared in the preview, not silently merged.', '导入会保留原文件；共享库中的同名资源会在预览中比较，不会静默合并。')
                  : action === 'apply'
                    ? harness === 'common'
                      ? t('The common folder uses native .agents/skills discovery for Copilot and compatible tools. This is not an individual tool switch; no extra .copilot copy is created.', '公共目录使用 .agents/skills 原生发现机制，供 Copilot 等兼容工具读取。这不是逐个工具的开关，也不会额外生成 .copilot 副本。')
                      : t('Only the selected compatible location is changed. Global instructions are supported for Claude only.', '只变更所选的兼容位置；全局指令目前仅支持 Claude。')
                    : t('Detach keeps target files and stops managing them. Restore returns the pre-adoption backup only when the managed target is unmodified.', '停止管理会保留目标文件；恢复原文件仅在已管理目标未被另外修改时可用。')}
              </p>
            </fieldset>
          )}
          <details className="library-details">
            <summary>{t('Resource details and safety', '查看资源路径与安全说明')}</summary>
            <p>{t('Shared source folder: ', '共享源目录：')}<code>{snapshot.root}</code></p>
            {selected && <p>{t('Selected resource: ', '所选资源：')}<code>{selected.path}</code></p>}
            <p>{t('Scanning does not change files. This version shares skills and Claude global instructions—not plugin or configuration conversions.', '扫描不会改动文件。当前共享技能和 Claude 全局指令，不进行插件或配置格式转换。')}</p>
          </details>
          {!recovery && management}
          {blocked.length > 0 && <Inventory title={t('Blocked resources', '被阻止的资源')} entries={blocked} />}
        </>
      )}
      {approval && (
        <section className="attention-panel install-panel" aria-labelledby={previewId}>
          <span className="eyebrow">{t('APPROVAL REQUIRED', '需要你确认')}</span>
          <h3 id={previewId} ref={previewHeading} tabIndex={-1}>{t('Review the changes', '查看将发生的变更')}</h3>
          <p>{t(`${approval.plan.changes?.length ?? 0} changes · ${approval.plan.changes?.filter((change) => change.action === 'delete').length ?? 0} deletions`, `${approval.plan.changes?.length ?? 0} 项变更 · ${approval.plan.changes?.filter((change) => change.action === 'delete').length ?? 0} 项删除`)}</p>
          <p>{t('Review all changes below. Nothing is written until you confirm.', '请检查下方所有变更。确认前不会写入文件。')}</p>
          <div className="operation-list scroll-list">
            {(approval.plan.changes ?? []).map((change, index) => (
              <article key={`${change.path}-${index}`}>
                <strong>{change.action === 'delete' ? t('delete', '删除') : change.action === 'create' ? t('create', '新增') : change.action === 'replace' ? t('replace', '替换') : change.action === 'update' ? t('update', '更新') : change.action}</strong>
                <span>{change.path}</span>
                <PreviewText label={t('Before', '变更前')} text={change.before} />
                <PreviewText label={t('After', '变更后')} text={change.after} />
              </article>
            ))}
          </div>
          <button className="primary" type="button" disabled={busy || !!working} onClick={() => void confirm()}>
            {actionLabels[action].confirm}
          </button>
        </section>
      )}
    </div>
  )
}
