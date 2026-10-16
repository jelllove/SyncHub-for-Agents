import { useEffect, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import './trayTip.css'
import { useI18n } from './i18n'
import { activityMessage } from './activityMessages'

const kinds = ['ready', 'syncing', 'pulling', 'scanning', 'comparing', 'applying', 'pushing', 'done', 'warning', 'error', 'paused'] as const
type TipKind = typeof kinds[number]
type Tip = { kind: TipKind; title: string; message: string; revision: number }

function validTip(value: unknown): value is Tip {
  return typeof value === 'object' && value !== null &&
    'kind' in value && kinds.some((kind) => kind === value.kind) &&
    'title' in value && typeof value.title === 'string' && value.title.length > 0 &&
    'message' in value && typeof value.message === 'string' &&
    'revision' in value && typeof value.revision === 'number' &&
    Number.isSafeInteger(value.revision) && value.revision >= 0
}

function StatusIcon({ kind, title }: { kind: TipKind; title: string }) {
  const { t } = useI18n()
  let glyph
  switch (kind) {
    case 'pulling': glyph = <path d="M12 4v13m-5-5 5 5 5-5M5 20h14" />; break
    case 'pushing': glyph = <path d="M12 20V7m-5 5 5-5 5 5M5 4h14" />; break
    case 'scanning': glyph = <><circle cx="10" cy="10" r="6" /><path d="m15 15 5 5" /></>; break
    case 'comparing': glyph = <path d="M4 7h16m-4-4 4 4-4 4M20 17H4m4-4-4 4 4 4" />; break
    case 'applying': glyph = <path d="m3 7 2 2 3-4m3 2h10M3 15l2 2 3-4m3 2h10" />; break
    case 'done': glyph = <path d="m5 12 4 4L19 6" />; break
    case 'warning': glyph = <><path d="m12 3 10 18H2L12 3Zm0 6v5" /><circle cx="12" cy="17" r=".5" /></>; break
    case 'error': glyph = <><circle cx="12" cy="12" r="9" /><path d="m8 8 8 8m0-8-8 8" /></>; break
    case 'paused': glyph = <path d="M8 5v14M16 5v14" />; break
    default: glyph = <path d="M20 8a9 9 0 0 0-15-3L3 8m0-5v5h5M4 16a9 9 0 0 0 15 3l2-3m0 5v-5h-5" />
  }
  return <svg role="img" aria-label={t(`${title} status`, `${title}状态`)} viewBox="0 0 24 24">{glyph}</svg>
}

export function TrayTip() {
  const { t } = useI18n()
  const [tip, setTip] = useState<Tip>()
  const [error, setError] = useState('')
  const [invalidStatus, setInvalidStatus] = useState(false)
  const revision = useRef(-1)
  const mounted = useRef(false)

  const emit = async (name: string) => {
    try {
      await Events.Emit(name)
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  useEffect(() => {
    mounted.current = true
    const unsubscribe = Events.On('tray-tip:status', (event) => {
      if (event.sender) return
      if (!validTip(event.data)) {
        setInvalidStatus(true)
        return
      }
      if (event.data.revision < revision.current) return
      revision.current = event.data.revision
      setTip(event.data)
      setInvalidStatus(false)
      setError('')
    })
    const unsubscribeError = Events.On('tray-tip:preference-error', (event) => {
      if (!event.sender && typeof event.data === 'string') setError(event.data)
    })
    void emit('tray-tip:ready')
    return () => {
      mounted.current = false
      unsubscribe()
      unsubscribeError()
    }
  }, [])

  const title = tip ? activityMessage(tip.title, t) : t('Waiting for sync status…', '正在等待同步状态…')
  const visibleError = error || (invalidStatus ? t('Activity status could not be read. Open SyncHub for Agents for details.', '无法读取活动状态。请打开 SyncHub for Agents 查看详情。') : '')
  const message = visibleError || (tip ? activityMessage(tip.message, t) : '') || t('Connecting to the desktop activity feed.', '正在连接桌面活动信息。')

  return (
    <section className="tray-tip-card" data-kind={tip?.kind ?? 'ready'} aria-label={t('SyncHub activity', 'SyncHub 活动')}>
      <div className="tray-tip-heading">
        <span className="tray-tip-brand">SYNCHUB FOR AGENTS</span>
        <button aria-label={t('Dismiss activity tip', '关闭活动提示')} onClick={() => void emit('tray-tip:dismiss')} type="button">×</button>
      </div>
      <div className="tray-tip-content" role={visibleError || tip?.kind === 'error' || tip?.kind === 'warning' ? 'alert' : 'status'} aria-atomic="true">
        {tip && <span className="tray-tip-icon"><StatusIcon kind={tip.kind} title={title} /></span>}
        <div>
          <h1>{title}</h1>
          <p title={message}>{message}</p>
        </div>
      </div>
      <div className="tray-tip-actions">
        <button className="tray-tip-disable" onClick={() => void emit('tray-tip:disable')} type="button" title={t('Turn tips back on in Settings > Desktop', '可在设置 > 桌面中重新开启提示')}>
          {t("Don't show again", '不再显示')}
        </button>
        <button className="tray-tip-open" aria-label={t('Open SyncHub for Agents', '打开 SyncHub for Agents')} onClick={() => void emit('tray-tip:open')} type="button">
          {t('Open app', '打开应用')} <span aria-hidden="true">→</span>
        </button>
      </div>
    </section>
  )
}
