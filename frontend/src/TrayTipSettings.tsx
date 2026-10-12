import { useEffect, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import {
  SetTrayTipsEnabled,
  TrayTipsEnabled,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'

export function TrayTipSettings() {
  const [enabled, setEnabled] = useState<boolean>()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const mounted = useRef(false)

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void TrayTipsEnabled().then((value) => {
      if (active) setEnabled(value)
    }).catch((cause) => {
      if (active) setError(cause instanceof Error ? cause.message : String(cause))
    }).finally(() => {
      if (active) setLoading(false)
    })
    const unsubscribe = Events.On('tray-tip:preferences-changed', (event) => {
      if (!event.sender) setRevision((value) => value + 1)
    })
    return () => { active = false; unsubscribe() }
  }, [revision])

  const save = async (value: boolean) => {
    setSaving(true)
    setError('')
    try {
      await SetTrayTipsEnabled(value)
      if (mounted.current) setRevision((current) => current + 1)
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      if (mounted.current) setSaving(false)
    }
  }

  return (
    <fieldset>
      <legend>Activity tips</legend>
      <label className="toggle-row">
        <span>
          <strong>Show activity tips</strong>
          <small>Show small Windows tray popups while syncing. Hide after 5 seconds, or 10 seconds for errors. Changes save immediately.</small>
        </span>
        <input
          type="checkbox"
          checked={enabled ?? false}
          disabled={loading || saving || enabled === undefined || !!error}
          onChange={(event) => void save(event.target.checked)}
        />
      </label>
      <small>The tray icon and error status stay active even when popups are off.</small>
      {(loading || saving) && <small role="status">{saving ? 'Saving activity tip settings…' : 'Loading activity tip settings…'}</small>}
      {error && (
        <>
          <div className="inline-error" role="alert">{error}</div>
          <button className="secondary" onClick={() => setRevision((value) => value + 1)} type="button">
            Retry activity tip settings
          </button>
        </>
      )}
    </fieldset>
  )
}
