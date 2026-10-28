import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { Events } from '@wailsio/runtime'
import { UpdateStatus } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type { Status } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/updater/models'
import { useI18n } from './i18n'

type UpdateAction = 'check' | 'automatic' | 'restart' | 'release'

type UpdateController = {
  status: Status | null
  loading: boolean
  pending: UpdateAction | null
  error: string
  run: (action: UpdateAction, request: () => Promise<Status | void>) => Promise<void>
}

const UpdateContext = createContext<UpdateController | null>(null)

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export function UpdateProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n()
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState<UpdateAction | null>(null)
  const [error, setError] = useState('')
  const mounted = useRef(false)
  const statusRevision = useRef(0)
  const requestPending = useRef(false)

  useEffect(() => {
    mounted.current = true
    let active = true
    const revision = statusRevision.current
    const unsubscribe = Events.On('updater:status', (event) => {
      if (!active) return
      statusRevision.current += 1
      setStatus(event.data as Status)
      setError('')
      setLoading(false)
    })
    void UpdateStatus().then((latest) => {
      if (active && statusRevision.current === revision) {
        setStatus(latest)
        setError('')
      }
    }).catch((cause) => {
      if (active && statusRevision.current === revision) setError(errorMessage(cause))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => {
      active = false
      mounted.current = false
      unsubscribe()
    }
  }, [])

  const run: UpdateController['run'] = async (action, request) => {
    if (requestPending.current) return
    requestPending.current = true
    setPending(action)
    setError('')
    const revision = statusRevision.current
    try {
      const latest = await request()
      if (!mounted.current) return
      if (latest) {
        // Live events can advance a download before its initiating request returns.
        if (statusRevision.current === revision) {
          setStatus(latest)
        } else if (action === 'automatic') {
          setStatus((current) => current ? { ...current, automatic: latest.automatic } : latest)
        }
      }
    } catch (cause) {
      if (mounted.current) setError(errorMessage(cause))
    } finally {
      requestPending.current = false
      if (mounted.current) setPending(null)
    }
  }

  const visibleError = error || status?.error || (status?.phase === 'error' ? t('Please try checking for updates again.', '请重试检查更新。') : '')

  return (
    <UpdateContext.Provider value={{ status, loading, pending, error: visibleError, run }}>
      {children}
    </UpdateContext.Provider>
  )
}

export function useUpdates() {
  const updates = useContext(UpdateContext)
  if (!updates) throw new Error('useUpdates must be used within UpdateProvider')
  return updates
}
