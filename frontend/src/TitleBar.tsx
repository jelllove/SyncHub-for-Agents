import { useEffect, useRef, useState } from 'react'
import { Events, Window } from '@wailsio/runtime'
import { BrandMark } from './BrandMark'
import { UpdateButton } from './UpdateButton'
import { useI18n } from './i18n'

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export function TitleBar({ syncBusy }: { syncBusy: boolean }) {
  const { t } = useI18n()
  const [maximized, setMaximized] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const mounted = useRef(false)
  const requestPending = useRef(false)

  useEffect(() => {
    mounted.current = true
    let active = true
    let changed = false
    const maximize = Events.On('common:WindowMaximise', () => {
      changed = true
      setMaximized(true)
    })
    const restore = Events.On('common:WindowUnMaximise', () => {
      changed = true
      setMaximized(false)
    })
    void Window.IsMaximised().then((value) => {
      if (active && !changed) setMaximized(value)
    }).catch((cause) => {
      if (active) setError(errorMessage(cause))
    })
    return () => {
      active = false
      mounted.current = false
      maximize()
      restore()
    }
  }, [])

  const run = async (action: () => Promise<void>) => {
    if (requestPending.current) return
    requestPending.current = true
    setPending(true)
    setError('')
    try {
      await action()
    } catch (cause) {
      if (mounted.current) setError(errorMessage(cause))
    } finally {
      requestPending.current = false
      if (mounted.current) setPending(false)
    }
  }

  return (
    <>
      <div aria-label={t('Window title bar', '窗口标题栏')} className="titlebar" role="toolbar">
        <div className="titlebar-caption">
          <BrandMark label="SyncHub for Agents" />
          <span>SyncHub for Agents</span>
        </div>
        <div className="titlebar-actions">
          <UpdateButton syncBusy={syncBusy} />
          <div className="window-controls">
            <button
              aria-label={t('Minimize window', '最小化窗口')}
              className="window-minimize"
              disabled={pending}
              onClick={() => void run(() => Window.Minimise())}
              title={t('Minimize', '最小化')}
              type="button"
            >
              <svg aria-hidden="true" viewBox="0 0 12 12"><path d="M1 6h10" /></svg>
            </button>
            <button
              aria-label={maximized ? t('Restore window', '还原窗口') : t('Maximize window', '最大化窗口')}
              className="window-maximize"
              disabled={pending}
              onClick={() => void run(() => Window.ToggleMaximise())}
              title={maximized ? t('Restore', '还原') : t('Maximize', '最大化')}
              type="button"
            >
              <svg aria-hidden="true" viewBox="0 0 12 12">
                {maximized ? <path d="M3.5 3.5v-2h7v7h-2m-7-5h7v7h-7z" /> : <path d="M1.5 1.5h9v9h-9z" />}
              </svg>
            </button>
            <button
              aria-label={t('Close window', '关闭窗口')}
              className="window-close"
              disabled={pending}
              onClick={() => void run(() => Window.Close())}
              title={t('Close to tray', '关闭到托盘')}
              type="button"
            >
              <svg aria-hidden="true" viewBox="0 0 12 12"><path d="m1.5 1.5 9 9m-9 0 9-9" /></svg>
            </button>
          </div>
        </div>
      </div>
      {error && <div className="inline-error titlebar-error" role="alert">{error}</div>}
    </>
  )
}
